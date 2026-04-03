package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- InitDatabase ---

func TestInitDatabase_StoresPrefix(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — a fresh repository with no prefix set.

	// When
	err := repo.InitDatabase(ctx, "TST")
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	prefix, err := repo.GetPrefix(ctx)
	if err != nil {
		t.Fatalf("expected no error from GetPrefix, got %v", err)
	}
	if prefix != "TST" {
		t.Errorf("expected prefix %q, got %q", "TST", prefix)
	}
}

func TestInitDatabase_AlreadyInitialized_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — the database has already been initialized.
	if err := repo.InitDatabase(ctx, "AAA"); err != nil {
		t.Fatalf("precondition: init database: %v", err)
	}

	// When
	err := repo.InitDatabase(ctx, "BBB")

	// Then
	if err == nil {
		t.Fatal("expected error when initializing twice, got nil")
	}
}

// --- GetPrefix ---

func TestGetPrefix_NotInitialized_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — no prefix has been set.

	// When
	_, err := repo.GetPrefix(ctx)

	// Then
	if err == nil {
		t.Fatal("expected error for uninitialized database, got nil")
	}
}

// --- GC ---

func TestGC_RemovesDeletedIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one deleted issue and one live domain.
	liveID := mustIssueID(t)
	deletedID := mustIssueID(t)
	now := time.Now()

	live := mustTask(t, liveID, "live issue", now)
	deleted := mustTask(t, deletedID, "deleted issue", now).WithDeleted()

	for _, iss := range []domain.Issue{live, deleted} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: create issue: %v", err)
		}
	}

	// When
	deletedCount, closedCount, err := repo.GC(ctx, false)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedCount != 1 {
		t.Errorf("expected 1 deleted removed, got %d", deletedCount)
	}
	if closedCount != 0 {
		t.Errorf("expected 0 closed removed, got %d", closedCount)
	}
	// The live issue should still be retrievable.
	if _, err := repo.GetIssue(ctx, liveID, false); err != nil {
		t.Errorf("live issue should still exist: %v", err)
	}
	// The deleted issue should be gone.
	if _, err := repo.GetIssue(ctx, deletedID, true); err == nil {
		t.Error("deleted issue should have been removed by GC")
	}
}

func TestGC_IncludeClosed_RemovesClosedIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one closed issue and one open domain.
	openID := mustIssueID(t)
	closedID := mustIssueID(t)
	now := time.Now()

	open := mustTask(t, openID, "open issue", now)
	closed := mustTask(t, closedID, "closed issue", now).WithState(domain.StateClosed)

	for _, iss := range []domain.Issue{open, closed} {
		if err := repo.CreateIssue(ctx, iss); err != nil {
			t.Fatalf("precondition: create issue: %v", err)
		}
	}

	// When
	deletedCount, closedCount, err := repo.GC(ctx, true)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedCount != 0 {
		t.Errorf("expected 0 deleted removed, got %d", deletedCount)
	}
	if closedCount != 1 {
		t.Errorf("expected 1 closed removed, got %d", closedCount)
	}
}

func TestGC_ExcludeClosed_KeepsClosedIssues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one closed domain.
	closedID := mustIssueID(t)
	now := time.Now()
	closed := mustTask(t, closedID, "closed issue", now).WithState(domain.StateClosed)

	if err := repo.CreateIssue(ctx, closed); err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	deletedCount, closedCount, err := repo.GC(ctx, false)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedCount != 0 {
		t.Errorf("expected 0 deleted removed, got %d", deletedCount)
	}
	if closedCount != 0 {
		t.Errorf("expected 0 closed removed, got %d", closedCount)
	}
}

func TestGC_EmptyRepository_ReturnsZeroCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — empty repository.

	// When
	deletedCount, closedCount, err := repo.GC(ctx, true)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if deletedCount != 0 {
		t.Errorf("expected 0 deleted, got %d", deletedCount)
	}
	if closedCount != 0 {
		t.Errorf("expected 0 closed, got %d", closedCount)
	}
}

// --- IntegrityCheck ---

func TestIntegrityCheck_AlwaysReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.IntegrityCheck(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- CountDeletedRatio ---

func TestCountDeletedRatio_ReturnsCorrectCounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — two live issues and one deleted domain.
	now := time.Now()
	for range 2 {
		id := mustIssueID(t)
		if err := repo.CreateIssue(ctx, mustTask(t, id, "live", now)); err != nil {
			t.Fatalf("precondition: create issue: %v", err)
		}
	}
	deletedID := mustIssueID(t)
	deleted := mustTask(t, deletedID, "deleted", now).WithDeleted()
	if err := repo.CreateIssue(ctx, deleted); err != nil {
		t.Fatalf("precondition: create deleted issue: %v", err)
	}

	// When
	total, deletedN, err := repo.CountDeletedRatio(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if deletedN != 1 {
		t.Errorf("expected 1 deleted, got %d", deletedN)
	}
}

func TestCountDeletedRatio_EmptyRepository_ReturnsZeros(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	total, deleted, err := repo.CountDeletedRatio(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if deleted != 0 {
		t.Errorf("expected deleted 0, got %d", deleted)
	}
}

// --- CountVirtualLabelsInTable ---

func TestCountVirtualLabelsInTable_AlwaysReturnsZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	count, err := repo.CountVirtualLabelsInTable(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

// --- ClearAllData ---

func TestClearAllData_RemovesAllState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — a repository with a prefix and an domain.
	if err := repo.InitDatabase(ctx, "CLR"); err != nil {
		t.Fatalf("precondition: init database: %v", err)
	}
	id := mustIssueID(t)
	now := time.Now()
	if err := repo.CreateIssue(ctx, mustTask(t, id, "will be cleared", now)); err != nil {
		t.Fatalf("precondition: create issue: %v", err)
	}

	// When
	err := repo.ClearAllData(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Prefix should be cleared — GetPrefix should error.
	if _, prefixErr := repo.GetPrefix(ctx); prefixErr == nil {
		t.Error("expected error from GetPrefix after ClearAllData, got nil")
	}
	// Issue should be gone.
	if _, issueErr := repo.GetIssue(ctx, id, true); issueErr == nil {
		t.Error("expected error from GetIssue after ClearAllData, got nil")
	}
}

// --- Restore*Raw (no-ops) ---

func TestRestoreIssueRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreIssueRaw(ctx, domain.BackupIssueRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRestoreCommentRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreCommentRaw(ctx, "NP-test1", domain.BackupCommentRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRestoreClaimRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreClaimRaw(ctx, "NP-test1", domain.BackupClaimRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRestoreRelationshipRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreRelationshipRaw(ctx, "NP-test1", domain.BackupRelationshipRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRestoreHistoryRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreHistoryRaw(ctx, "NP-test1", domain.BackupHistoryRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRestoreLabelRaw_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RestoreLabelRaw(ctx, "NP-test1", domain.BackupLabelRecord{})
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// --- RebuildFTS ---

func TestRebuildFTS_ReturnsNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.RebuildFTS(ctx)
	// Then
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
