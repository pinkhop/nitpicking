package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- CreateClaim ---

func TestCreateClaim_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)
	author := mustAuthor(t, "alice")
	now := time.Now()

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}

	// When
	err = repo.CreateClaim(ctx, c)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// --- GetClaimByIssue ---

func TestGetClaimByIssue_Found(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)
	author := mustAuthor(t, "bob")
	now := time.Now()

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When
	got, err := repo.GetClaimByIssue(ctx, issueID)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.IssueID() != issueID {
		t.Errorf("expected issue ID %s, got %s", issueID, got.IssueID())
	}
	if !got.Author().Equal(author) {
		t.Errorf("expected author %s, got %s", author, got.Author())
	}
}

func TestGetClaimByIssue_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)

	// When
	_, err := repo.GetClaimByIssue(ctx, issueID)

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- GetClaimByID ---

func TestGetClaimByID_FoundByToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — GetClaimByID takes a plaintext token and hashes it to match
	// the stored SHA-512 hash.
	issueID := mustIssueID(t)
	author := mustAuthor(t, "carol")
	now := time.Now()

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	token := c.Token()
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When — look up by plaintext token
	got, err := repo.GetClaimByID(ctx, token)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got.IssueID() != issueID {
		t.Errorf("expected issue ID %s, got %s", issueID, got.IssueID())
	}
}

func TestGetClaimByID_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	_, err := repo.GetClaimByID(ctx, "nonexistent-token")

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- InvalidateClaim ---

func TestInvalidateClaim_RemovesClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)
	author := mustAuthor(t, "dave")
	now := time.Now()

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When — invalidate using the SHA-512 hash ID
	err = repo.InvalidateClaim(ctx, c.ID())
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Verify claim is no longer retrievable.
	_, byIssueErr := repo.GetClaimByIssue(ctx, issueID)
	if !errors.Is(byIssueErr, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound after invalidation, got %v", byIssueErr)
	}
}

func TestInvalidateClaim_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.InvalidateClaim(ctx, "nonexistent-claim-id")

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- UpdateClaimStaleAt ---

func TestUpdateClaimStaleAt_UpdatesTimestamp(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)
	author := mustAuthor(t, "eve")
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	newStaleAt := now.Add(6 * time.Hour)

	// When — update using the hash ID
	err = repo.UpdateClaimStaleAt(ctx, c.ID(), newStaleAt)
	// Then
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	got, _ := repo.GetClaimByIssue(ctx, issueID)
	if !got.StaleAt().Equal(newStaleAt) {
		t.Errorf("expected staleAt %v, got %v", newStaleAt, got.StaleAt())
	}
}

func TestUpdateClaimStaleAt_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	err := repo.UpdateClaimStaleAt(ctx, "nonexistent", time.Now())

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- ListStaleClaims ---

func TestListStaleClaims_ReturnsStaleClaims(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one stale claim (created 3h ago with 2h threshold), one fresh
	staleIssueID := mustIssueID(t)
	freshIssueID := mustIssueID(t)
	author := mustAuthor(t, "grace")
	now := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	threeHoursAgo := now.Add(-3 * time.Hour)
	tenMinutesAgo := now.Add(-10 * time.Minute)

	staleClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: staleIssueID,
		Author:  author,
		Now:     threeHoursAgo,
	})
	if err != nil {
		t.Fatalf("precondition: create stale claim: %v", err)
	}

	freshClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: freshIssueID,
		Author:  author,
		Now:     tenMinutesAgo,
	})
	if err != nil {
		t.Fatalf("precondition: create fresh claim: %v", err)
	}

	for _, c := range []domain.Claim{staleClaim, freshClaim} {
		if err := repo.CreateClaim(ctx, c); err != nil {
			t.Fatalf("precondition: persist claim: %v", err)
		}
	}

	// When
	stale, err := repo.ListStaleClaims(ctx, now)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale claim, got %d", len(stale))
	}
	if stale[0].IssueID() != staleIssueID {
		t.Errorf("expected stale claim for %s, got %s", staleIssueID, stale[0].IssueID())
	}
}

func TestListStaleClaims_EmptyWhenNoneStale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one fresh claim
	issueID := mustIssueID(t)
	author := mustAuthor(t, "heidi")
	now := time.Now()

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When — query at the same time as creation (not stale yet)
	stale, err := repo.ListStaleClaims(ctx, now)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stale) != 0 {
		t.Errorf("expected 0 stale claims, got %d", len(stale))
	}
}

// --- ListActiveClaims ---

func TestListActiveClaims_ReturnsActiveClaims(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one active claim, one stale
	activeIssueID := mustIssueID(t)
	staleIssueID := mustIssueID(t)
	author := mustAuthor(t, "ivan")
	now := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	threeHoursAgo := now.Add(-3 * time.Hour)
	tenMinutesAgo := now.Add(-10 * time.Minute)

	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: activeIssueID,
		Author:  author,
		Now:     tenMinutesAgo,
	})
	if err != nil {
		t.Fatalf("precondition: create active claim: %v", err)
	}

	staleClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: staleIssueID,
		Author:  author,
		Now:     threeHoursAgo,
	})
	if err != nil {
		t.Fatalf("precondition: create stale claim: %v", err)
	}

	for _, c := range []domain.Claim{activeClaim, staleClaim} {
		if err := repo.CreateClaim(ctx, c); err != nil {
			t.Fatalf("precondition: persist claim: %v", err)
		}
	}

	// When
	active, err := repo.ListActiveClaims(ctx, now)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active claim, got %d", len(active))
	}
	if active[0].IssueID() != activeIssueID {
		t.Errorf("expected active claim for %s, got %s", activeIssueID, active[0].IssueID())
	}
}

func TestListActiveClaims_EmptyWhenAllStale(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — one claim created long ago
	issueID := mustIssueID(t)
	author := mustAuthor(t, "judy")
	now := time.Date(2026, 1, 1, 15, 0, 0, 0, time.UTC)
	yesterday := now.Add(-24 * time.Hour)

	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: issueID,
		Author:  author,
		Now:     yesterday,
	})
	if err != nil {
		t.Fatalf("precondition: create claim: %v", err)
	}
	if err := repo.CreateClaim(ctx, c); err != nil {
		t.Fatalf("precondition: persist claim: %v", err)
	}

	// When
	active, err := repo.ListActiveClaims(ctx, now)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active claims, got %d", len(active))
	}
}
