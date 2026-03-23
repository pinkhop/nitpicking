package claim_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestValidateClaim_UnclaimedOpenTicket_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	status := claim.TicketClaimStatus{
		State: ticket.StateOpen,
	}

	// When
	err := claim.ValidateClaim(status, false, time.Now())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaim_DeletedTicket_Fails(t *testing.T) {
	t.Parallel()

	// Given
	status := claim.TicketClaimStatus{
		State:     ticket.StateOpen,
		IsDeleted: true,
	}

	// When
	err := claim.ValidateClaim(status, false, time.Now())

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestValidateClaim_ClosedTicket_Fails(t *testing.T) {
	t.Parallel()

	// Given
	status := claim.TicketClaimStatus{
		State: ticket.StateClosed,
	}

	// When
	err := claim.ValidateClaim(status, false, time.Now())

	// Then
	if !errors.Is(err, domain.ErrTerminalState) {
		t.Errorf("expected ErrTerminalState, got %v", err)
	}
}

func TestValidateClaim_AlreadyClaimed_NoSteal_Fails(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := claim.NewClaim(claim.NewClaimParams{
		TicketID: mustTicketID(t),
		Author:   mustAuthor(t, "alice"),
		Now:      now,
	})
	status := claim.TicketClaimStatus{
		State:       ticket.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When
	err := claim.ValidateClaim(status, false, now.Add(1*time.Hour))

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
	}
}

func TestValidateClaim_StaleAndStealAllowed_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := claim.NewClaim(claim.NewClaimParams{
		TicketID: mustTicketID(t),
		Author:   mustAuthor(t, "alice"),
		Now:      now,
	})
	status := claim.TicketClaimStatus{
		State:       ticket.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When — 3 hours later, claim is stale (default 2h threshold)
	err := claim.ValidateClaim(status, true, now.Add(3*time.Hour))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaim_NotStaleButStealRequested_Fails(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	activeClaim, _ := claim.NewClaim(claim.NewClaimParams{
		TicketID: mustTicketID(t),
		Author:   mustAuthor(t, "alice"),
		Now:      now,
	})
	status := claim.TicketClaimStatus{
		State:       ticket.StateClaimed,
		ActiveClaim: activeClaim,
	}

	// When — only 1 hour later, not stale yet
	err := claim.ValidateClaim(status, true, now.Add(1*time.Hour))

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
	}
}

func TestStealNote_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	// When
	note := claim.StealNote("alice")

	// Then
	expected := `Stolen from "alice".`
	if note != expected {
		t.Errorf("expected %q, got %q", expected, note)
	}
}
