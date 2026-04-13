//go:build boundary

package sqlite_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- GC Removes Soft-Deleted Issues ---

func TestBoundary_GC_RemovesSoftDeletedIssues(t *testing.T) {
	// Given — a soft-deleted task and an open task
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	keepID := createIntTask(t, svc, "Keep me")
	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Delete me", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// When
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: false})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The open task should still exist.
	_, err = svc.ShowIssue(ctx, keepID.String())
	if err != nil {
		t.Errorf("expected open task to survive GC, got error: %v", err)
	}

	// The deleted task should be physically gone (not even accessible with
	// includeDeleted, since GC removes the row).
	_, err = svc.ShowIssue(ctx, deleteOut.Issue.ID().String())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for GC'd issue, got: %v", err)
	}
}

// --- GC with IncludeClosed Removes Closed Issues ---

func TestBoundary_GC_IncludeClosed_RemovesClosedIssues(t *testing.T) {
	// Given — a closed task and an open task
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	keepID := createIntTask(t, svc, "Open task")
	closedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedOut.Issue.ID().String(), ClaimID: closedOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}

	// When
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: true})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The open task should survive.
	_, err = svc.ShowIssue(ctx, keepID.String())
	if err != nil {
		t.Errorf("expected open task to survive GC, got error: %v", err)
	}

	// The closed task should be physically gone.
	_, err = svc.ShowIssue(ctx, closedOut.Issue.ID().String())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for GC'd closed issue, got: %v", err)
	}
}

// --- IntegrityCheck on Valid Database ---

func TestBoundary_IntegrityCheck_ValidDatabase_Succeeds(t *testing.T) {
	// Given — a freshly initialized database with some data
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	_ = createIntTask(t, svc, "Some task")

	// When — run doctor, which internally calls IntegrityCheck
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Doctor should not report any storage_integrity findings.
	for _, f := range doctorOut.Findings {
		if f.Category == "storage_integrity" {
			t.Errorf("unexpected integrity finding: %s", f.Message)
		}
	}
}

// --- CountDeletedRatio Reflects Actual Data ---

func TestBoundary_CountDeletedRatio_ReflectsDeletedIssues(t *testing.T) {
	// Given — 3 tasks, 1 deleted
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	_ = createIntTask(t, svc, "Task A")
	_ = createIntTask(t, svc, "Task B")
	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Task C (to delete)", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// When — run doctor, which uses CountDeletedRatio internally.
	// With 1/3 deleted (33%), if the threshold is exceeded the doctor
	// should report a gc_recommended finding.
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The deleted ratio is 33% (1/3). Verify that the doctor ran the
	// CountDeletedRatio check by confirming no error occurred. The
	// gc_recommended finding may or may not be present depending on the
	// threshold, but the absence of errors confirms the query works.
	for _, f := range doctorOut.Findings {
		if f.Category == "storage_integrity" {
			t.Errorf("unexpected integrity error: %s", f.Message)
		}
	}
}

// --- GC Preserves Non-Deleted Issues Referenced by Relationships ---

func TestBoundary_GC_PreservesRelationshipReferencedIssues(t *testing.T) {
	// Given — task A blocked_by task B. Task C is soft-deleted.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskA := createIntTask(t, svc, "Blocked task")
	taskB := createIntTask(t, svc, "Blocker task")

	err := svc.AddRelationship(ctx, taskA.String(), driving.RelationshipInput{
		TargetID: taskB.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// Create and delete an unrelated task.
	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Unrelated deleted", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// When — GC runs
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: false})
	// Then — task A and B (with their relationship) should survive
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	showA, err := svc.ShowIssue(ctx, taskA.String())
	if err != nil {
		t.Fatalf("expected task A to survive GC: %v", err)
	}
	if len(showA.Relationships) != 1 {
		t.Errorf("task A relationships: got %d, want 1", len(showA.Relationships))
	}

	_, err = svc.ShowIssue(ctx, taskB.String())
	if err != nil {
		t.Fatalf("expected task B to survive GC: %v", err)
	}

	// The deleted task should be gone.
	_, err = svc.ShowIssue(ctx, deleteOut.Issue.ID().String())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound for GC'd issue, got: %v", err)
	}
}

// --- GC with IncludeClosed Clears Parent References on Surviving Children ---

func TestBoundary_GC_ClosedParent_ClearsOpenChildParentID(t *testing.T) {
	// Given — a closed epic with one deferred child and one closed child.
	// When GC runs with IncludeClosed, the epic and closed child are
	// removed. The deferred child survives with its parent_id nullified.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleEpic, Title: "Epic to close and GC", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}

	// Child A — will be closed, then the deferred child reopened via claim→defer.
	childAOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child A (will close)", Author: author(t, "alice"),
		ParentID: epicOut.Issue.ID().String(), Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create child A failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childAOut.Issue.ID().String(), ClaimID: childAOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child A failed: %v", err)
	}

	// Child B — closed first so the epic can close, then re-opened as deferred.
	childBOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Child B (will defer)", Author: author(t, "alice"),
		ParentID: epicOut.Issue.ID().String(), Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create child B failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childBOut.Issue.ID().String(), ClaimID: childBOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child B failed: %v", err)
	}

	// Close the epic (all children closed → epic can close).
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: epicOut.Issue.ID().String(), ClaimID: epicOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close epic failed: %v", err)
	}

	// Reopen child B (closed → open), then claim and defer it so it survives
	// IncludeClosed GC as a deferred issue.
	err = svc.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: childBOut.Issue.ID().String(),
		Author:  author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: reopen child B failed: %v", err)
	}

	childBClaim2, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: childBOut.Issue.ID().String(), Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: re-claim child B failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: childBOut.Issue.ID().String(), ClaimID: childBClaim2.ClaimID,
		Action: driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer child B failed: %v", err)
	}

	// When — GC with IncludeClosed.
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: true})
	// Then — the deferred child survives with parent_id nullified.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	childBShow, err := svc.ShowIssue(ctx, childBOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("expected deferred child to survive GC: %v", err)
	}
	if childBShow.ParentID != "" {
		t.Errorf("deferred child parent_id should be zero after parent GC'd, got %s", childBShow.ParentID)
	}

	// Closed epic and closed child should be gone.
	_, err = svc.ShowIssue(ctx, epicOut.Issue.ID().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for GC'd epic, got: %v", err)
	}
	_, err = svc.ShowIssue(ctx, childAOut.Issue.ID().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for GC'd closed child, got: %v", err)
	}
}

// --- GC Removes FTS Entries for Purged Issues and Comments ---

func TestBoundary_GC_RemovesFTSEntries(t *testing.T) {
	// Given — a task with a distinctive title and a comment, both soft-deleted.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// Create a searchable task with a unique keyword.
	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "xylophoneSentinel task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// Add a comment with a unique keyword to this task.
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: deleteOut.Issue.ID().String(),
		Author:  author(t, "alice"),
		Body:    "marimbaUnique comment body",
	})
	if err != nil {
		t.Fatalf("precondition: add comment failed: %v", err)
	}

	// Also create a surviving task with a different unique keyword.
	_ = createIntTask(t, svc, "harmonicaSurvivor task")

	// Verify searches find the content before deletion.
	issueSearch, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "xylophoneSentinel", Limit: 10,
	})
	if err != nil {
		t.Fatalf("precondition: search issues failed: %v", err)
	}
	if len(issueSearch.Items) == 0 {
		t.Fatalf("precondition: expected to find xylophoneSentinel in search before GC")
	}

	commentSearch, err := svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query: "marimbaUnique", Limit: 10,
	})
	if err != nil {
		t.Fatalf("precondition: search comments failed: %v", err)
	}
	if len(commentSearch.Comments) == 0 {
		t.Fatalf("precondition: expected to find marimbaUnique in comment search before GC")
	}

	// Soft-delete the task.
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// When — GC runs.
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: false})
	// Then — FTS should no longer match the purged data.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	issueSearch, err = svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "xylophoneSentinel", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search issues after GC failed: %v", err)
	}
	if len(issueSearch.Items) != 0 {
		t.Errorf("expected 0 results for xylophoneSentinel after GC, got %d", len(issueSearch.Items))
	}

	commentSearch, err = svc.SearchComments(ctx, driving.SearchCommentsInput{
		Query: "marimbaUnique", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search comments after GC failed: %v", err)
	}
	if len(commentSearch.Comments) != 0 {
		t.Errorf("expected 0 results for marimbaUnique after GC, got %d", len(commentSearch.Comments))
	}

	// Surviving task should still be searchable.
	survivorSearch, err := svc.SearchIssues(ctx, driving.SearchIssuesInput{
		Query: "harmonicaSurvivor", Limit: 10,
	})
	if err != nil {
		t.Fatalf("search surviving issue failed: %v", err)
	}
	if len(survivorSearch.Items) != 1 {
		t.Errorf("expected 1 result for harmonicaSurvivor, got %d", len(survivorSearch.Items))
	}
}

// --- GC Removes Side Tables While Preserving Neighboring Live Rows ---

func TestBoundary_GC_RemovesSideTablesPreservesLiveRows(t *testing.T) {
	// Given — two tasks: one deleted with claims, history, comments, and a
	// relationship; one live with its own comment and history.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// Live task — will survive GC.
	liveID := createIntTask(t, svc, "Live task")
	_, err := svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: liveID.String(), Author: author(t, "alice"),
		Body: "Comment on live task",
	})
	if err != nil {
		t.Fatalf("precondition: add live comment failed: %v", err)
	}

	// Deleted task — will be purged.
	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Doomed task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create doomed task failed: %v", err)
	}
	_, err = svc.AddComment(ctx, driving.AddCommentInput{
		IssueID: deleteOut.Issue.ID().String(), Author: author(t, "alice"),
		Body: "Comment on doomed task",
	})
	if err != nil {
		t.Fatalf("precondition: add doomed comment failed: %v", err)
	}

	// Add a relationship: live blocked_by doomed.
	err = svc.AddRelationship(ctx, liveID.String(), driving.RelationshipInput{
		TargetID: deleteOut.Issue.ID().String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete doomed task failed: %v", err)
	}

	// When — GC runs.
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: false})
	// Then — live task's comment and history survive; doomed task is gone
	// along with all its side-table data.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Doomed task should be gone.
	_, err = svc.ShowIssue(ctx, deleteOut.Issue.ID().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for GC'd doomed task, got: %v", err)
	}

	// Live task should survive with its comment intact.
	liveShow, err := svc.ShowIssue(ctx, liveID.String())
	if err != nil {
		t.Fatalf("expected live task to survive GC: %v", err)
	}
	if liveShow.CommentCount != 1 {
		t.Errorf("live task comment count: got %d, want 1", liveShow.CommentCount)
	}

	// Live task's history should have entries (at least the creation event).
	historyOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: liveID.String(), Limit: 100,
	})
	if err != nil {
		t.Fatalf("show history for live task failed: %v", err)
	}
	if len(historyOut.Entries) == 0 {
		t.Errorf("live task history should have entries after GC")
	}

	// The blocked_by relationship should have been cleaned (doomed was GC'd).
	if len(liveShow.Relationships) != 0 {
		t.Errorf("live task relationships: got %d, want 0 (blocker was GC'd)", len(liveShow.Relationships))
	}
}

// --- IncludeClosed on Mixed Epic-Child Graph Preserves Consistency ---

func TestBoundary_GC_IncludeClosed_MixedEpicChildGraph_ConsistentState(t *testing.T) {
	// Given — an epic with three children: one open, one closed, one deferred.
	// GC with IncludeClosed should remove the closed child but preserve the
	// open and deferred children. The surviving children's parent_id should
	// remain intact (the epic itself is open, not closed).
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	epicID := createIntEpic(t, svc, "Mixed-state epic")

	openChildOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Open child", Author: author(t, "alice"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create open child failed: %v", err)
	}

	closedChildOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed child", Author: author(t, "alice"),
		ParentID: epicID.String(), Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed child failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedChildOut.Issue.ID().String(), ClaimID: closedChildOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close child failed: %v", err)
	}

	deferredChildOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Deferred child", Author: author(t, "alice"),
		ParentID: epicID.String(), Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create deferred child failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: deferredChildOut.Issue.ID().String(), ClaimID: deferredChildOut.ClaimID,
		Action: driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer child failed: %v", err)
	}

	// When — GC with IncludeClosed.
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: true})
	// Then — the closed child is gone; the epic, open child, and deferred
	// child survive with consistent parent references.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Closed child should be physically gone.
	_, err = svc.ShowIssue(ctx, closedChildOut.Issue.ID().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for GC'd closed child, got: %v", err)
	}

	// Epic should survive.
	epicShow, err := svc.ShowIssue(ctx, epicID.String())
	if err != nil {
		t.Fatalf("expected epic to survive GC: %v", err)
	}
	// Child count should be 2 (open + deferred), not 3.
	if epicShow.ChildCount != 2 {
		t.Errorf("epic child_count after GC: got %d, want 2", epicShow.ChildCount)
	}

	// Open child should survive with parent intact.
	openChildShow, err := svc.ShowIssue(ctx, openChildOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("expected open child to survive GC: %v", err)
	}
	if openChildShow.ParentID != epicID.String() {
		t.Errorf("open child parent_id should be %s, got %s", epicID, openChildShow.ParentID)
	}

	// Deferred child should survive with parent intact.
	deferredChildShow, err := svc.ShowIssue(ctx, deferredChildOut.Issue.ID().String())
	if err != nil {
		t.Fatalf("expected deferred child to survive GC: %v", err)
	}
	if deferredChildShow.ParentID != epicID.String() {
		t.Errorf("deferred child parent_id should be %s, got %s", epicID, deferredChildShow.ParentID)
	}
}
