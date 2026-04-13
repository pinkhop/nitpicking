package core_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// mustDomainID generates a domain.ID for claim rule tests, fatally failing
// the test if generation fails.
func mustDomainID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate ID: %v", err)
	}
	return id
}

// mustDomainAuthor constructs a domain.Author for claim rule tests, fatally
// failing the test if construction fails.
func mustDomainAuthor(t *testing.T, name string) domain.Author {
	t.Helper()
	a, err := domain.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func TestValidateClaim_UnclaimedOpenIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	status := core.IssueClaimStatus{
		State: domain.StateOpen,
	}

	// When
	err := core.ValidateClaim(status, time.Now())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaim_DeletedIssue_Fails(t *testing.T) {
	t.Parallel()

	// Given
	status := core.IssueClaimStatus{
		State:     domain.StateOpen,
		IsDeleted: true,
	}

	// When
	err := core.ValidateClaim(status, time.Now())

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestValidateClaim_ClosedIssue_Fails(t *testing.T) {
	t.Parallel()

	// Closed issues cannot be claimed; reopen is a claim-free operation.

	// Given
	status := core.IssueClaimStatus{
		State: domain.StateClosed,
	}

	// When
	err := core.ValidateClaim(status, time.Now())

	// Then
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition for closed issue, got %v", err)
	}
}

func TestValidateClaim_DeferredIssue_Fails(t *testing.T) {
	t.Parallel()

	// Deferred issues cannot be claimed; undefer is a claim-free operation.

	// Given
	status := core.IssueClaimStatus{
		State: domain.StateDeferred,
	}

	// When
	err := core.ValidateClaim(status, time.Now())

	// Then
	if !errors.Is(err, domain.ErrIllegalTransition) {
		t.Errorf("expected ErrIllegalTransition for deferred issue, got %v", err)
	}
}

func TestValidateClaim_ActiveClaim_Fails(t *testing.T) {
	t.Parallel()

	// An active (non-stale) claim always blocks a new claim — there is no
	// steal mechanic.

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	status := core.IssueClaimStatus{
		State:       domain.StateOpen,
		ActiveClaim: activeClaim,
	}

	// When — 1 hour later, still within the default 2h stale threshold
	claimErr := core.ValidateClaim(status, now.Add(1*time.Hour))

	// Then
	if !errors.Is(claimErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", claimErr)
	}
}

func TestValidateClaim_StaleClaim_Succeeds(t *testing.T) {
	t.Parallel()

	// A stale claim is treated as nonexistent; the caller overwrites the
	// expired row when creating the new claim.

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	status := core.IssueClaimStatus{
		State:       domain.StateOpen,
		ActiveClaim: activeClaim,
	}

	// When — 3 hours later, past the default 2h stale threshold
	claimErr := core.ValidateClaim(status, now.Add(3*time.Hour))

	// Then
	if claimErr != nil {
		t.Fatalf("unexpected error for stale claim: %v", claimErr)
	}
}

func TestValidateClaim_DeletedAndClaimed_DeletionTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Given — issue is both deleted and has an active claim
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	status := core.IssueClaimStatus{
		State:       domain.StateOpen,
		IsDeleted:   true,
		ActiveClaim: activeClaim,
	}

	// When — attempt to claim a deleted issue
	claimErr := core.ValidateClaim(status, now.Add(3*time.Hour))

	// Then — deletion error takes precedence over any claim state
	if !errors.Is(claimErr, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState (deletion takes precedence), got %v", claimErr)
	}
}

func TestValidateClaim_ExactStaleAtBoundary_Fails(t *testing.T) {
	t.Parallel()

	// IsStale uses strict greater-than, so at the exact boundary the claim is
	// still considered active and must be rejected.

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	status := core.IssueClaimStatus{
		State:       domain.StateOpen,
		ActiveClaim: activeClaim,
	}

	// When — attempt claim at the exact stale-at boundary
	exactBoundary := activeClaim.StaleAt()
	claimErr := core.ValidateClaim(status, exactBoundary)

	// Then — at the exact boundary IsStale returns false (strict >),
	// so the claim is still active and must be rejected
	if !errors.Is(claimErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError at exact boundary, got %v", claimErr)
	}
}

func TestValidateClaim_SelfReclaimActiveClaimFails(t *testing.T) {
	t.Parallel()

	// Self-re-claim on an active claim held by the same author fails with
	// ClaimConflictError; there is no special-case bypass for the current holder.

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	author := mustDomainAuthor(t, "alice")
	activeClaim, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  author,
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	status := core.IssueClaimStatus{
		State:       domain.StateOpen,
		ActiveClaim: activeClaim,
	}

	// When — same author tries to re-claim the same issue (1 hour later, still
	// within the 2h stale threshold)
	claimErr := core.ValidateClaim(status, now.Add(1*time.Hour))

	// Then — conflict even for the same author; caller must use extend or wait
	if !errors.Is(claimErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError for self-re-claim, got %v", claimErr)
	}
}

// --- ValidateActiveClaim ---

func TestValidateActiveClaim_ActiveClaim_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a claim created now (not yet stale).
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — validate 1 hour after creation (still within 2h window)
	validateErr := core.ValidateActiveClaim(c, now.Add(1*time.Hour))

	// Then
	if validateErr != nil {
		t.Fatalf("unexpected error: %v", validateErr)
	}
}

func TestValidateActiveClaim_StaleClaim_Fails(t *testing.T) {
	t.Parallel()

	// Given — a claim created long enough ago to be stale.
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — validate 3 hours after creation (past the 2h stale threshold)
	validateErr := core.ValidateActiveClaim(c, now.Add(3*time.Hour))

	// Then — ErrStaleClaim wraps the error
	if !errors.Is(validateErr, domain.ErrStaleClaim) {
		t.Errorf("expected ErrStaleClaim for stale claim, got %v", validateErr)
	}
}
