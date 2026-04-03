//go:build boundary

package sqlite_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- CreateRelationship and ListRelationships Roundtrip ---

func TestBoundary_CreateRelationship_BlockedBy_AppearsInShowIssue(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker task")
	blockedID := createIntTask(t, svc, "Blocked task")

	// When
	err := svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	showOut, err := svc.ShowIssue(ctx, blockedID.String())
	if err != nil {
		t.Fatalf("unexpected error showing issue: %v", err)
	}
	if len(showOut.Relationships) != 1 {
		t.Fatalf("relationships: got %d, want 1", len(showOut.Relationships))
	}
	rel := showOut.Relationships[0]
	if rel.SourceID != blockedID.String() {
		t.Errorf("source: got %s, want %s", rel.SourceID, blockedID)
	}
	if rel.TargetID != blockerID.String() {
		t.Errorf("target: got %s, want %s", rel.TargetID, blockerID)
	}
	if rel.Type != domain.RelBlockedBy.String() {
		t.Errorf("type: got %s, want blocked_by", rel.Type)
	}
}

// --- DeleteRelationship ---

func TestBoundary_RemoveRelationship_BlockedBy_NoLongerListed(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker task")
	blockedID := createIntTask(t, svc, "Blocked task")

	err := svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// When
	err = svc.RemoveRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	showOut, err := svc.ShowIssue(ctx, blockedID.String())
	if err != nil {
		t.Fatalf("unexpected error showing issue: %v", err)
	}
	if len(showOut.Relationships) != 0 {
		t.Errorf("relationships: got %d, want 0 after removal", len(showOut.Relationships))
	}
}

// --- GetBlockerStatuses: Unresolved Blocker Prevents Readiness ---

func TestBoundary_GetBlockerStatuses_UnresolvedBlocker_PreventsReadiness(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker task")
	blockedID := createIntTask(t, svc, "Blocked task")

	err := svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// When
	showOut, err := svc.ShowIssue(ctx, blockedID.String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showOut.IsReady {
		t.Error("issue with unresolved blocker should not be ready")
	}
}

// --- GetBlockerStatuses: Closed Blocker Resolves Readiness ---

func TestBoundary_GetBlockerStatuses_ClosedBlocker_RestoresReadiness(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker task")
	blockedID := createIntTask(t, svc, "Blocked task")

	err := svc.AddRelationship(ctx, blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(), Type: domain.RelBlockedBy,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// Close the blocker.
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: blockerID.String(), Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: blockerID.String(), ClaimID: claimOut.ClaimID, Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}

	// When
	showOut, err := svc.ShowIssue(ctx, blockedID.String())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !showOut.IsReady {
		t.Error("issue should be ready after blocker is closed")
	}
}

// --- Symmetric Relationship: Refs Stored Once, Listed Both Ways ---

func TestBoundary_Refs_StoredOnce_ListedBothWays(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskA := createIntTask(t, svc, "Task A")
	taskB := createIntTask(t, svc, "Task B")

	// When — add refs from A to B
	err := svc.AddRelationship(ctx, taskA.String(), driving.RelationshipInput{
		TargetID: taskB.String(), Type: domain.RelRefs,
	}, author(t, "alice"))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify A lists the relationship.
	showA, err := svc.ShowIssue(ctx, taskA.String())
	if err != nil {
		t.Fatalf("unexpected error showing A: %v", err)
	}
	if len(showA.Relationships) != 1 {
		t.Fatalf("A relationships: got %d, want 1", len(showA.Relationships))
	}
	if showA.Relationships[0].SourceID != taskA.String() {
		t.Errorf("A source: got %s, want %s", showA.Relationships[0].SourceID, taskA)
	}
	if showA.Relationships[0].TargetID != taskB.String() {
		t.Errorf("A target: got %s, want %s", showA.Relationships[0].TargetID, taskB)
	}

	// Verify B also lists the relationship (symmetric — shown from B's perspective).
	showB, err := svc.ShowIssue(ctx, taskB.String())
	if err != nil {
		t.Fatalf("unexpected error showing B: %v", err)
	}
	if len(showB.Relationships) != 1 {
		t.Fatalf("B relationships: got %d, want 1", len(showB.Relationships))
	}
	if showB.Relationships[0].SourceID != taskB.String() {
		t.Errorf("B source: got %s, want %s", showB.Relationships[0].SourceID, taskB)
	}
	if showB.Relationships[0].TargetID != taskA.String() {
		t.Errorf("B target: got %s, want %s", showB.Relationships[0].TargetID, taskA)
	}
}

// --- Refs Idempotent: Adding Same Refs Twice Does Not Duplicate ---

func TestBoundary_Refs_Idempotent_NoDuplicate(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskA := createIntTask(t, svc, "Task A")
	taskB := createIntTask(t, svc, "Task B")

	err := svc.AddRelationship(ctx, taskA.String(), driving.RelationshipInput{
		TargetID: taskB.String(), Type: domain.RelRefs,
	}, author(t, "alice"))
	if err != nil {
		t.Fatalf("precondition: first add failed: %v", err)
	}

	// When — add the same refs again (reverse direction)
	err = svc.AddRelationship(ctx, taskB.String(), driving.RelationshipInput{
		TargetID: taskA.String(), Type: domain.RelRefs,
	}, author(t, "alice"))
	// Then — should succeed (idempotent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only one relationship should be listed for A.
	showA, err := svc.ShowIssue(ctx, taskA.String())
	if err != nil {
		t.Fatalf("unexpected error showing A: %v", err)
	}
	if len(showA.Relationships) != 1 {
		t.Errorf("A relationships: got %d, want 1 (idempotent)", len(showA.Relationships))
	}
}

// --- Blocking Relationship: blocks Stored As-Is, Visible From Target ---

func TestBoundary_Blocks_VisibleFromTargetPerspective(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	blockerID := createIntTask(t, svc, "Blocker task")
	blockedID := createIntTask(t, svc, "Blocked task")

	// When — add "blocks" from blockerID to blockedID
	err := svc.AddRelationship(ctx, blockerID.String(), driving.RelationshipInput{
		TargetID: blockedID.String(), Type: domain.RelBlocks,
	}, author(t, "alice"))
	// Then — the relationship is visible from the blocked issue's side,
	// presented from the blocker's perspective (source=blocker, target=blocked,
	// type=blocks). The "blocks" type does not affect readiness — only
	// "blocked_by" does. The CLI layer is responsible for converting user
	// intent into the appropriate type.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	showBlocked, err := svc.ShowIssue(ctx, blockedID.String())
	if err != nil {
		t.Fatalf("unexpected error showing blocked issue: %v", err)
	}

	foundBlocks := false
	for _, rel := range showBlocked.Relationships {
		if rel.Type == domain.RelBlocks.String() && rel.SourceID == blockerID.String() && rel.TargetID == blockedID.String() {
			foundBlocks = true
		}
	}
	if !foundBlocks {
		t.Errorf("expected blocks relationship visible from blocked issue's side; got relationships: %v", showBlocked.Relationships)
	}
}
