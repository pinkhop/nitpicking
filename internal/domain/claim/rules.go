package claim

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// TicketClaimStatus summarizes a ticket's state for claim validation.
type TicketClaimStatus struct {
	// State is the ticket's current lifecycle state.
	State ticket.State
	// IsDeleted is true if the ticket has been soft-deleted.
	IsDeleted bool
	// ActiveClaim is the current claim on the ticket, if any. A zero-value
	// Claim (empty ID) indicates no active claim.
	ActiveClaim Claim
}

// ValidateClaim checks whether a ticket can be claimed per §6.1.
// Returns nil if the ticket is claimable, or an appropriate error.
func ValidateClaim(status TicketClaimStatus, allowSteal bool, now time.Time) error {
	if status.IsDeleted {
		return fmt.Errorf("cannot claim deleted ticket: %w", domain.ErrTerminalState)
	}

	if status.State == ticket.StateClosed {
		return fmt.Errorf("cannot claim closed ticket: %w", domain.ErrTerminalState)
	}

	if status.ActiveClaim.ID() != "" {
		if !allowSteal {
			return &domain.ClaimConflictError{
				TicketID:      status.ActiveClaim.TicketID().String(),
				CurrentHolder: status.ActiveClaim.Author().String(),
				StaleAt:       status.ActiveClaim.StaleAt(),
			}
		}

		if !status.ActiveClaim.IsStale(now) {
			return &domain.ClaimConflictError{
				TicketID:      status.ActiveClaim.TicketID().String(),
				CurrentHolder: status.ActiveClaim.Author().String(),
				StaleAt:       status.ActiveClaim.StaleAt(),
			}
		}

		// Claim is stale and steal is allowed — proceed.
	}

	return nil
}

// StealNote generates the auto-note body added when a ticket is stolen.
func StealNote(previousHolder string) string {
	return fmt.Sprintf("Stolen from %q.", previousHolder)
}
