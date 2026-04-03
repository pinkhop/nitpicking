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
	err := core.ValidateClaim(status, false, time.Now())
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
	err := core.ValidateClaim(status, false, time.Now())

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestValidateClaim_ClosedIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Closed issues can be reclaimed for reopening.

	// Given
	status := core.IssueClaimStatus{
		State: domain.StateClosed,
	}

	// When
	err := core.ValidateClaim(status, false, time.Now())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaim_AlreadyClaimed_NoSteal_Fails(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	status := core.IssueClaimStatus{
		State:       domain.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When
	err := core.ValidateClaim(status, false, now.Add(1*time.Hour))

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
	}
}

func TestValidateClaim_StaleAndStealAllowed_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	status := core.IssueClaimStatus{
		State:       domain.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When — 3 hours later, claim is stale (default 2h threshold)
	err := core.ValidateClaim(status, true, now.Add(3*time.Hour))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaim_NotStaleButStealRequested_Fails(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustDomainID(t),
		Author:  mustDomainAuthor(t, "alice"),
		Now:     now,
	})
	status := core.IssueClaimStatus{
		State:       domain.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When — only 1 hour later, not stale yet
	err := core.ValidateClaim(status, true, now.Add(1*time.Hour))

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
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
		State:       domain.StateClaimed,
		IsDeleted:   true,
		ActiveClaim: activeClaim,
	}

	// When — attempt to claim with steal allowed
	claimErr := core.ValidateClaim(status, true, now.Add(3*time.Hour))

	// Then — deletion error, not claim conflict
	if !errors.Is(claimErr, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState (deletion takes precedence), got %v", claimErr)
	}
}

func TestValidateClaim_ExactStaleAtBoundary_StealFails(t *testing.T) {
	t.Parallel()

	// Given — claim with known stale-at time
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
		State:       domain.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When — attempt steal at exact stale-at boundary
	exactBoundary := activeClaim.StaleAt()
	claimErr := core.ValidateClaim(status, true, exactBoundary)

	// Then — at the exact boundary, IsStale returns false (strict >),
	// so the steal is rejected
	if !errors.Is(claimErr, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError at exact boundary, got %v", claimErr)
	}
}

func TestStealComment_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	// When
	comment := core.StealComment("alice")

	// Then
	expected := `Stolen from "alice".`
	if comment != expected {
		t.Errorf("expected %q, got %q", expected, comment)
	}
}
