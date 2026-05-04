package core_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Test helpers ---

// setupRepairService creates a fresh service and repository initialised with
// the NP prefix, ready for repair tests.
func setupRepairService(t *testing.T) (driving.Service, *memory.Repository) {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)
	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("init: %v", err)
	}
	return svc, repo
}

// createRepairEpic creates an unclaimed epic and returns its domain ID.
func createRepairEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: "test-setup",
	})
	if err != nil {
		t.Fatalf("create epic %q: %v", title, err)
	}
	return out.Issue.ID()
}

// createRepairTask creates a task (optionally under parentID) and returns its
// domain ID. Pass the zero domain.ID to create an unparented task.
func createRepairTask(t *testing.T, svc driving.Service, title string, parentID domain.ID) domain.ID {
	t.Helper()
	input := driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: "test-setup",
	}
	if !parentID.IsZero() {
		input.ParentID = parentID.String()
	}
	out, err := svc.CreateIssue(t.Context(), input)
	if err != nil {
		t.Fatalf("create task %q: %v", title, err)
	}
	return out.Issue.ID()
}

// softDeleteViaRepo marks an issue as soft-deleted directly in the repository,
// bypassing the service claim path. Used when the service's cascade-delete
// behaviour would also remove child issues.
func softDeleteViaRepo(t *testing.T, repo *memory.Repository, id domain.ID) {
	t.Helper()
	ctx := context.Background()
	issue, err := repo.GetIssue(ctx, id, false)
	if err != nil {
		t.Fatalf("get issue %s for soft-delete: %v", id, err)
	}
	if err := repo.UpdateIssue(ctx, issue.WithDeleted()); err != nil {
		t.Fatalf("soft-delete %s: %v", id, err)
	}
}

// --- Tests ---

// TestRepairInvalidParentReferences_SoftDeletedParent verifies that a child
// whose parent is soft-deleted has its parent_id cleared and receives an
// audit comment with the spec-exact body.
func TestRepairInvalidParentReferences_SoftDeletedParent_ClearsParentAndAddsComment(t *testing.T) {
	t.Parallel()

	// Given — a task whose parent epic is soft-deleted.
	svc, repo := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Parent Epic")
	childID := createRepairTask(t, svc, "Child Task", parentID)
	softDeleteViaRepo(t, repo, parentID)

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then — output records the repair.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 1 {
		t.Fatalf("repaired count: got %d, want 1", len(out.Repaired))
	}
	rec := out.Repaired[0]
	if rec.IssueID != childID.String() {
		t.Errorf("IssueID: got %q, want %q", rec.IssueID, childID.String())
	}
	if rec.RemovedParentID != parentID.String() {
		t.Errorf("RemovedParentID: got %q, want %q", rec.RemovedParentID, parentID.String())
	}

	// Then — child's parent_id is cleared.
	shown, err := svc.ShowIssue(t.Context(), childID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("ParentID after repair: got %q, want empty", shown.ParentID)
	}

	// Then — audit comment is present with the spec-exact body.
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{IssueID: childID.String(), Limit: -1})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	want := fmt.Sprintf(
		"Removed dangling parent reference %s; parent did not exist in the database. Automated cleanup by 'np admin fix invalid-parent-reference'.",
		parentID.String(),
	)
	found := false
	for _, c := range comments.Comments {
		if c.Body == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("audit comment not found; want body:\n%q", want)
	}
}

// TestRepairInvalidParentReferences_HardDeletedParent verifies that a child
// whose parent has been hard-deleted (removed from storage via GC) has its
// parent_id cleared and receives an audit comment.
func TestRepairInvalidParentReferences_HardDeletedParent_ClearsParentAndAddsComment(t *testing.T) {
	t.Parallel()

	// Given — parent is soft-deleted then GC'd, leaving only the child.
	svc, repo := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Parent Epic")
	childID := createRepairTask(t, svc, "Child Task", parentID)
	softDeleteViaRepo(t, repo, parentID)
	if _, err := svc.GC(t.Context(), driving.GCInput{IncludeClosed: false}); err != nil {
		t.Fatalf("gc: %v", err)
	}

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then — same behaviour as soft-deleted parent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 1 {
		t.Fatalf("repaired count: got %d, want 1", len(out.Repaired))
	}
	if out.Repaired[0].IssueID != childID.String() {
		t.Errorf("IssueID: got %q, want %q", out.Repaired[0].IssueID, childID.String())
	}
	if out.Repaired[0].RemovedParentID != parentID.String() {
		t.Errorf("RemovedParentID: got %q, want %q", out.Repaired[0].RemovedParentID, parentID.String())
	}

	shown, err := svc.ShowIssue(t.Context(), childID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if shown.ParentID != "" {
		t.Errorf("ParentID after repair: got %q, want empty", shown.ParentID)
	}

	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{IssueID: childID.String(), Limit: -1})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	want := fmt.Sprintf(
		"Removed dangling parent reference %s; parent did not exist in the database. Automated cleanup by 'np admin fix invalid-parent-reference'.",
		parentID.String(),
	)
	found := false
	for _, c := range comments.Comments {
		if c.Body == want {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("audit comment not found; want body:\n%q", want)
	}
}

// TestRepairInvalidParentReferences_ValidParent verifies that an issue with a
// valid (non-deleted) parent is left completely untouched.
func TestRepairInvalidParentReferences_ValidParent_NoChanges(t *testing.T) {
	t.Parallel()

	// Given — a task with a live parent.
	svc, _ := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Live Parent")
	childID := createRepairTask(t, svc, "Child Task", parentID)

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then — nothing repaired.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 0 {
		t.Errorf("repaired count: got %d, want 0", len(out.Repaired))
	}

	// Then — child's parent_id is unchanged.
	shown, err := svc.ShowIssue(t.Context(), childID.String())
	if err != nil {
		t.Fatalf("show issue: %v", err)
	}
	if shown.ParentID != parentID.String() {
		t.Errorf("ParentID: got %q, want %q", shown.ParentID, parentID.String())
	}

	// Then — no comment was added.
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{IssueID: childID.String(), Limit: -1})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments.Comments) != 0 {
		t.Errorf("comment count: got %d, want 0", len(comments.Comments))
	}
}

// TestRepairInvalidParentReferences_MultipleAffected verifies that all
// affected issues are repaired in a single run.
func TestRepairInvalidParentReferences_MultipleAffected_AllRepaired(t *testing.T) {
	t.Parallel()

	// Given — two tasks each with a different soft-deleted parent.
	svc, repo := setupRepairService(t)
	parent1ID := createRepairEpic(t, svc, "Parent 1")
	parent2ID := createRepairEpic(t, svc, "Parent 2")
	child1ID := createRepairTask(t, svc, "Child 1", parent1ID)
	child2ID := createRepairTask(t, svc, "Child 2", parent2ID)
	softDeleteViaRepo(t, repo, parent1ID)
	softDeleteViaRepo(t, repo, parent2ID)

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then — both issues appear in the output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 2 {
		t.Fatalf("repaired count: got %d, want 2", len(out.Repaired))
	}

	repairedMap := make(map[string]string, 2)
	for _, r := range out.Repaired {
		repairedMap[r.IssueID] = r.RemovedParentID
	}
	if repairedMap[child1ID.String()] != parent1ID.String() {
		t.Errorf("child1 removed parent: got %q, want %q", repairedMap[child1ID.String()], parent1ID.String())
	}
	if repairedMap[child2ID.String()] != parent2ID.String() {
		t.Errorf("child2 removed parent: got %q, want %q", repairedMap[child2ID.String()], parent2ID.String())
	}

	// Both children should now be parentless.
	for _, id := range []domain.ID{child1ID, child2ID} {
		shown, showErr := svc.ShowIssue(t.Context(), id.String())
		if showErr != nil {
			t.Fatalf("show issue %s: %v", id, showErr)
		}
		if shown.ParentID != "" {
			t.Errorf("issue %s ParentID after repair: got %q, want empty", id, shown.ParentID)
		}
	}
}

// TestRepairInvalidParentReferences_DryRun verifies that dry-run identifies
// affected issues but performs no writes.
func TestRepairInvalidParentReferences_DryRun_NoSideEffects(t *testing.T) {
	t.Parallel()

	// Given — a task with a soft-deleted parent.
	svc, repo := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Parent Epic")
	childID := createRepairTask(t, svc, "Child Task", parentID)
	softDeleteViaRepo(t, repo, parentID)

	// When — dry run
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: true,
	})
	// Then — output identifies the affected issue.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 1 {
		t.Fatalf("repaired count: got %d, want 1", len(out.Repaired))
	}
	if out.Repaired[0].IssueID != childID.String() {
		t.Errorf("IssueID: got %q, want %q", out.Repaired[0].IssueID, childID.String())
	}
	if out.Repaired[0].RemovedParentID != parentID.String() {
		t.Errorf("RemovedParentID: got %q, want %q", out.Repaired[0].RemovedParentID, parentID.String())
	}

	// Then — no comment was added (the real no-write verification).
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{IssueID: childID.String(), Limit: -1})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments.Comments) != 0 {
		t.Errorf("comment count after dry run: got %d, want 0", len(comments.Comments))
	}
}

// TestRepairInvalidParentReferences_ZeroAffected verifies that a database
// with no dangling references returns an empty result without error.
func TestRepairInvalidParentReferences_ZeroAffected_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// Given — no issues at all.
	svc, _ := setupRepairService(t)

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 0 {
		t.Errorf("repaired count: got %d, want 0", len(out.Repaired))
	}
}

// TestRepairInvalidParentReferences_ChildrenOfAffectedIssue_RetainValidParentID
// verifies that removing issue A's dangling parent does not affect issue B's
// valid reference to issue A.
func TestRepairInvalidParentReferences_ChildrenOfAffectedIssue_RetainValidParentID(t *testing.T) {
	t.Parallel()

	// Given — a three-level chain: grandparent (epic) → parent (task) → grandchild (task).
	// The grandparent is soft-deleted so the parent (middle tier) has a dangling
	// parent reference. The grandchild's reference to the parent is valid and
	// must not be touched.
	svc, repo := setupRepairService(t)
	grandparentID := createRepairEpic(t, svc, "Grandparent")
	parentID := createRepairTask(t, svc, "Middle Task (affected)", grandparentID)
	grandchildID := createRepairTask(t, svc, "Grandchild (untouched)", parentID)
	softDeleteViaRepo(t, repo, grandparentID)

	// When
	out, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "admin",
		DryRun: false,
	})
	// Then — only the middle task is repaired.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Repaired) != 1 {
		t.Fatalf("repaired count: got %d, want 1", len(out.Repaired))
	}
	if out.Repaired[0].IssueID != parentID.String() {
		t.Errorf("repaired issue: got %q, want %q", out.Repaired[0].IssueID, parentID.String())
	}

	// Then — grandchild still points to the (now parentless) middle task.
	gcShown, err := svc.ShowIssue(t.Context(), grandchildID.String())
	if err != nil {
		t.Fatalf("show grandchild: %v", err)
	}
	if gcShown.ParentID != parentID.String() {
		t.Errorf("grandchild ParentID: got %q, want %q", gcShown.ParentID, parentID.String())
	}
}

// TestRepairInvalidParentReferences_AuditComment_BodyMatchesSpec verifies the
// exact comment body mandated by the specification.
func TestRepairInvalidParentReferences_AuditComment_BodyMatchesSpec(t *testing.T) {
	t.Parallel()

	// Given — a task with a soft-deleted parent.
	svc, repo := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Doomed Parent")
	childID := createRepairTask(t, svc, "Orphaned Child", parentID)
	softDeleteViaRepo(t, repo, parentID)

	// When
	_, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "bot",
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the comment body exactly matches the specification.
	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{IssueID: childID.String(), Limit: -1})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}

	wantBody := fmt.Sprintf(
		"Removed dangling parent reference %s; parent did not exist in the database. Automated cleanup by 'np admin fix invalid-parent-reference'.",
		parentID.String(),
	)
	gotBody := comments.Comments[0].Body
	if gotBody != wantBody {
		t.Errorf("comment body mismatch:\ngot:  %q\nwant: %q", gotBody, wantBody)
		if strings.Contains(gotBody, parentID.String()) {
			t.Log("(parent ID is present; difference is in punctuation or surrounding text)")
		}
	}
}

// TestRepairInvalidParentReferences_Repair_WritesHistoryEntries verifies that
// the repair writes both an EventUpdated entry (parent field change) and an
// EventCommentAdded entry to the issue's history.
func TestRepairInvalidParentReferences_Repair_WritesHistoryEntries(t *testing.T) {
	t.Parallel()

	// Given — a task with a soft-deleted parent.
	svc, repo := setupRepairService(t)
	parentID := createRepairEpic(t, svc, "Ephemeral Parent")
	childID := createRepairTask(t, svc, "Tracked Child", parentID)
	softDeleteViaRepo(t, repo, parentID)

	// When
	_, err := svc.RepairInvalidParentReferences(t.Context(), driving.RepairInvalidParentsInput{
		Author: "auditor",
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then — the history log contains an EventUpdated entry for the parent field
	// change and an EventCommentAdded entry for the audit comment.
	histOut, err := svc.ShowHistory(t.Context(), driving.ListHistoryInput{
		IssueID: childID.String(),
		Limit:   -1,
	})
	if err != nil {
		t.Fatalf("show history: %v", err)
	}

	var hasParentUpdated, hasCommentAdded bool
	for _, entry := range histOut.Entries {
		switch entry.EventType {
		case history.EventUpdated.String():
			for _, ch := range entry.Changes {
				if ch.Field == "parent" && ch.Before == parentID.String() && ch.After == "-" {
					hasParentUpdated = true
				}
			}
		case history.EventCommentAdded.String():
			for _, ch := range entry.Changes {
				if ch.Field == "body" && strings.Contains(ch.After, parentID.String()) {
					hasCommentAdded = true
				}
			}
		}
	}

	if !hasParentUpdated {
		t.Errorf("missing EventUpdated history entry with parent field change (before=%s, after=-)", parentID)
	}
	if !hasCommentAdded {
		t.Errorf("missing EventCommentAdded history entry with audit comment body containing %s", parentID)
	}
}
