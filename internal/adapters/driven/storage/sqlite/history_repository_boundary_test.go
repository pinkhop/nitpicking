//go:build boundary

package sqlite_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- AppendHistory and ListHistory Roundtrip ---

func TestBoundary_AppendHistory_ListHistory_Roundtrip(t *testing.T) {
	// Given — create an issue (which appends a "created" history entry)
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "History roundtrip", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) < 1 {
		t.Fatal("expected at least 1 history entry for creation")
	}

	first := histOut.Entries[0]
	if first.IssueID != createOut.Issue.ID().String() {
		t.Errorf("issue ID: got %s, want %s", first.IssueID, createOut.Issue.ID().String())
	}
	if first.EventType != history.EventCreated.String() {
		t.Errorf("event type: got %s, want %s", first.EventType, history.EventCreated.String())
	}
	if first.Author != "alice" {
		t.Errorf("author: got %q, want %q", first.Author, "alice")
	}
	if first.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

// --- CountHistory (via ShowIssue revision) ---

func TestBoundary_CountHistory_IncrementsThroughOperations(t *testing.T) {
	// Given — create an issue with a claim, then update it.
	// Claiming no longer creates history, so this generates: created + updated = 2 entries.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Count history", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	newTitle := "Count history updated"
	if err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Title:   &newTitle,
	}); err != nil {
		t.Fatalf("precondition: update failed: %v", err)
	}

	// When — list all history entries
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
	})
	// Then — should have at least 2 entries (created + updated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) < 2 {
		t.Fatalf("entries: got %d, want >= 2 (created + updated)", len(histOut.Entries))
	}
}

// --- GetLatestHistory (via ShowIssue author) ---

func TestBoundary_GetLatestHistory_ReflectsLastActor(t *testing.T) {
	// Given — alice creates with a claim, then bob updates the issue.
	// Claiming no longer creates history, so the latest actor is determined by
	// the last mutation (update), not by who holds the claim.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Latest history", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// Release alice's claim so bob can claim.
	if err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Action:  driving.ActionRelease,
	}); err != nil {
		t.Fatalf("precondition: release failed: %v", err)
	}

	bobClaim, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: createOut.Issue.ID().String(), Author: author(t, "bob"),
	})
	if err != nil {
		t.Fatalf("precondition: bob claim failed: %v", err)
	}

	// Bob updates the issue, creating a history entry attributed to bob.
	newDesc := "bob's update"
	if err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:     createOut.Issue.ID().String(),
		ClaimID:     bobClaim.ClaimID,
		Description: &newDesc,
	}); err != nil {
		t.Fatalf("precondition: bob update failed: %v", err)
	}

	// When — show the issue (author is derived from GetLatestHistory)
	showOut, err := svc.ShowIssue(ctx, createOut.Issue.ID().String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.Author != "bob" {
		t.Errorf("author: got %q, want %q (latest actor)", showOut.Author, "bob")
	}
}

// --- ListHistory with Author Filter ---

func TestBoundary_ListHistory_FilterByAuthor_OnlyReturnsMatchingEntries(t *testing.T) {
	// Given — alice creates with a claim; bob claims and updates the issue.
	// Both produce history entries from different authors. Claiming itself no
	// longer creates history, so only alice's creation and bob's update appear.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Author filter", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// Release alice's claim and let bob claim and update.
	if err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Action:  driving.ActionRelease,
	}); err != nil {
		t.Fatalf("precondition: release failed: %v", err)
	}

	bobClaim, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: createOut.Issue.ID().String(), Author: author(t, "bob"),
	})
	if err != nil {
		t.Fatalf("precondition: bob claim failed: %v", err)
	}

	newDesc := "bob's update"
	if err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID:     createOut.Issue.ID().String(),
		ClaimID:     bobClaim.ClaimID,
		Description: &newDesc,
	}); err != nil {
		t.Fatalf("precondition: bob update failed: %v", err)
	}

	// When — filter history by alice
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
		Filter:  driving.HistoryFilterInput{Author: "alice"},
	})
	// Then — only alice's creation entry; bob's update is excluded.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1 (only alice's creation)", len(histOut.Entries))
	}
	if histOut.Entries[0].Author != "alice" {
		t.Errorf("author: got %q, want %q", histOut.Entries[0].Author, "alice")
	}
}

// --- ListHistory with After and Before Filters ---

func TestBoundary_ListHistory_FilterByAfter_OnlyReturnsNewerEntries(t *testing.T) {
	// Given — create and claim an issue, record a cutoff, then close it.
	// Claiming no longer creates a history entry; closing does.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "After filter", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)

	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}

	// When — filter history after the cutoff
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
		Filter:  driving.HistoryFilterInput{After: cutoff},
	})
	// Then — should only include the state_changed (close) entry. Claiming no
	// longer creates a history entry so the close is the only post-cutoff event.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1 (only post-cutoff close)", len(histOut.Entries))
	}
	if histOut.Entries[0].EventType != history.EventStateChanged.String() {
		t.Errorf("event type: got %s, want %s", histOut.Entries[0].EventType, history.EventStateChanged.String())
	}
}

func TestBoundary_ListHistory_FilterByBefore_OnlyReturnsOlderEntries(t *testing.T) {
	// Given — create an issue, wait, record a cutoff, then claim it
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Before filter", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()

	_, err = svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: createOut.Issue.ID().String(), Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	// When — filter history before the cutoff
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
		Filter:  driving.HistoryFilterInput{Before: cutoff},
	})
	// Then — should only include the creation entry
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1 (only pre-cutoff creation)", len(histOut.Entries))
	}
	if histOut.Entries[0].EventType != history.EventCreated.String() {
		t.Errorf("event type: got %s, want %s", histOut.Entries[0].EventType, history.EventCreated.String())
	}
}

// --- Revision Numbering Increments Correctly ---

func TestBoundary_HistoryRevision_IncrementsSequentially(t *testing.T) {
	// Given — create an issue with a claim, then update it and close it.
	// Claiming no longer creates a history entry, so this generates:
	// created (rev 0), updated (rev 1), closed (rev 2).
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Revision test", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	newTitle := "Updated revision test"
	err = svc.UpdateIssue(ctx, driving.UpdateIssueInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Title:   &newTitle,
	})
	if err != nil {
		t.Fatalf("precondition: update failed: %v", err)
	}

	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: createOut.Issue.ID().String(),
		ClaimID: createOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}

	// When
	histOut, err := svc.ShowHistory(ctx, driving.ListHistoryInput{
		IssueID: createOut.Issue.ID().String(),
	})
	// Then — created + updated + closed = 3 entries; no claim/release history.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(histOut.Entries) < 3 {
		t.Fatalf("entries: got %d, want >= 3 (created, updated, closed)", len(histOut.Entries))
	}

	// Verify revisions are sequential starting from 0.
	for i, entry := range histOut.Entries {
		if entry.Revision != i {
			t.Errorf("entry %d: revision got %d, want %d", i, entry.Revision, i)
		}
	}
}
