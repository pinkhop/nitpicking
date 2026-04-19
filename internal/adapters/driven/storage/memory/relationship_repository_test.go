package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- CreateRelationship ---

func TestCreateRelationship_NewRelationship_ReturnsTrue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	srcID := mustIssueID(t)
	tgtID := mustIssueID(t)
	rel := mustRelationship(t, srcID, tgtID, domain.RelBlockedBy)

	// When
	created, err := repo.CreateRelationship(ctx, rel)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Error("expected created=true for new relationship")
	}
}

func TestCreateRelationship_Duplicate_ReturnsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	srcID := mustIssueID(t)
	tgtID := mustIssueID(t)
	rel := mustRelationship(t, srcID, tgtID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — create the same relationship again
	created, err := repo.CreateRelationship(ctx, rel)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false for duplicate relationship")
	}
}

func TestCreateRelationship_SymmetricDuplicate_ReturnsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — create A refs B
	aID := mustIssueID(t)
	bID := mustIssueID(t)
	forward := mustRelationship(t, aID, bID, domain.RelRefs)
	if _, err := repo.CreateRelationship(ctx, forward); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — create B refs A (symmetric duplicate)
	reverse := mustRelationship(t, bID, aID, domain.RelRefs)
	created, err := repo.CreateRelationship(ctx, reverse)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false for symmetric duplicate (refs is symmetric)")
	}
}

// --- DeleteRelationship ---

func TestDeleteRelationship_Existing_ReturnsTrue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	srcID := mustIssueID(t)
	tgtID := mustIssueID(t)
	rel := mustRelationship(t, srcID, tgtID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	deleted, err := repo.DeleteRelationship(ctx, srcID, tgtID, domain.RelBlockedBy)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true for existing relationship")
	}
}

func TestDeleteRelationship_Nonexistent_ReturnsFalse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — no relationships exist

	// When
	deleted, err := repo.DeleteRelationship(ctx, mustIssueID(t), mustIssueID(t), domain.RelBlockedBy)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted {
		t.Error("expected deleted=false for nonexistent relationship")
	}
}

func TestDeleteRelationship_SymmetricReverse_ReturnsTrue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — create A refs B
	aID := mustIssueID(t)
	bID := mustIssueID(t)
	forward := mustRelationship(t, aID, bID, domain.RelRefs)
	if _, err := repo.CreateRelationship(ctx, forward); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — delete B refs A (reverse direction of symmetric)
	deleted, err := repo.DeleteRelationship(ctx, bID, aID, domain.RelRefs)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleted {
		t.Error("expected deleted=true for reverse-direction symmetric relationship")
	}
}

// --- ListRelationships ---

func TestListRelationships_ReturnsAllDirections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — A blocked_by B, A refs C
	aID := mustIssueID(t)
	bID := mustIssueID(t)
	cID := mustIssueID(t)

	blockedBy := mustRelationship(t, aID, bID, domain.RelBlockedBy)
	refs := mustRelationship(t, aID, cID, domain.RelRefs)
	if _, err := repo.CreateRelationship(ctx, blockedBy); err != nil {
		t.Fatalf("precondition: %v", err)
	}
	if _, err := repo.CreateRelationship(ctx, refs); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — list relationships for A
	rels, err := repo.ListRelationships(ctx, aID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rels) != 2 {
		t.Fatalf("expected 2 relationships for A, got %d", len(rels))
	}
}

func TestListRelationships_IncludesIncomingRelationships(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — A blocked_by B; list for B should include this as an
	// incoming relationship.
	aID := mustIssueID(t)
	bID := mustIssueID(t)

	blockedBy := mustRelationship(t, aID, bID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, blockedBy); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — list relationships for B (the target)
	rels, err := repo.ListRelationships(ctx, bID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for B, got %d", len(rels))
	}
	// The incoming relationship should still show A as source, B as target.
	if rels[0].SourceID() != aID {
		t.Errorf("expected source %s, got %s", aID, rels[0].SourceID())
	}
}

func TestListRelationships_SymmetricPresentedFromQueryPerspective(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — A refs B (symmetric); when listing for B, it should be
	// presented as B refs A.
	aID := mustIssueID(t)
	bID := mustIssueID(t)

	refs := mustRelationship(t, aID, bID, domain.RelRefs)
	if _, err := repo.CreateRelationship(ctx, refs); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — list for B
	rels, err := repo.ListRelationships(ctx, bID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected 1 relationship for B, got %d", len(rels))
	}
	// Symmetric relationship should be swapped so B is the source.
	if rels[0].SourceID() != bID {
		t.Errorf("expected source %s (swapped perspective), got %s", bID, rels[0].SourceID())
	}
	if rels[0].TargetID() != aID {
		t.Errorf("expected target %s (swapped perspective), got %s", aID, rels[0].TargetID())
	}
}

func TestListRelationships_EmptyForUnrelatedIssue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — A blocked_by B exists, but C has no relationships.
	aID := mustIssueID(t)
	bID := mustIssueID(t)
	cID := mustIssueID(t)

	rel := mustRelationship(t, aID, bID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	rels, err := repo.ListRelationships(ctx, cID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rels) != 0 {
		t.Errorf("expected 0 relationships for unrelated issue, got %d", len(rels))
	}
}

// --- GetBlockerStatuses ---

func TestGetBlockerStatuses_OpenBlocker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blockedID blocked_by blockerID; blocker is open
	now := time.Now()
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)

	blocker := mustTask(t, blockerID, "Blocker", now)
	blocked := mustTask(t, blockedID, "Blocked", now)
	for _, iss := range []domain.Issue{blocker, blocked} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetBlockerStatuses(ctx, blockedID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 blocker status, got %d", len(statuses))
	}
	if statuses[0].IsClosed {
		t.Error("expected IsClosed=false for open blocker")
	}
	if statuses[0].IsDeleted {
		t.Error("expected IsDeleted=false for non-deleted blocker")
	}
}

func TestGetBlockerStatuses_ClosedBlocker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blocker is closed
	now := time.Now()
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)

	blocker := mustTask(t, blockerID, "Blocker", now).WithState(domain.StateClosed)
	blocked := mustTask(t, blockedID, "Blocked", now)
	for _, iss := range []domain.Issue{blocker, blocked} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetBlockerStatuses(ctx, blockedID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 blocker status, got %d", len(statuses))
	}
	if !statuses[0].IsClosed {
		t.Error("expected IsClosed=true for closed blocker")
	}
}

func TestGetBlockerStatuses_DeletedBlocker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blocker is soft-deleted
	now := time.Now()
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)

	blocker := mustTask(t, blockerID, "Blocker", now).WithDeleted()
	blocked := mustTask(t, blockedID, "Blocked", now)
	for _, iss := range []domain.Issue{blocker, blocked} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetBlockerStatuses(ctx, blockedID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 blocker status, got %d", len(statuses))
	}
	if !statuses[0].IsDeleted {
		t.Error("expected IsDeleted=true for deleted blocker")
	}
}

func TestGetBlockerStatuses_MissingBlockerTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — blocked_by relationship exists but the target issue does not.
	// The in-memory adapter treats missing targets as deleted.
	blockerID := mustIssueID(t)
	blockedID := mustIssueID(t)

	blocked := mustTask(t, blockedID, "Blocked", time.Now())
	if err := repo.CreateIssue(ctx, blocked); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	rel := mustRelationship(t, blockedID, blockerID, domain.RelBlockedBy)
	if _, err := repo.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetBlockerStatuses(ctx, blockedID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 blocker status, got %d", len(statuses))
	}
	if !statuses[0].IsDeleted {
		t.Error("expected IsDeleted=true for missing blocker target")
	}
}

func TestGetBlockerStatuses_NoBlockers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — issue with no blocked_by relationships
	id := mustIssueID(t)
	if err := repo.CreateIssue(ctx, mustTask(t, id, "Unblocked", time.Now())); err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	statuses, err := repo.GetBlockerStatuses(ctx, id)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected 0 blocker statuses, got %d", len(statuses))
	}
}
