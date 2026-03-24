package claim

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// IssueClaimStatus summarizes an issue's state for claim validation.
type IssueClaimStatus struct {
	// State is the issue's current lifecycle state.
	State issue.State
	// IsDeleted is true if the issue has been soft-deleted.
	IsDeleted bool
	// ActiveClaim is the current claim on the issue, if any. A zero-value
	// Claim (empty ID) indicates no active claim.
	ActiveClaim Claim
}

// ValidateClaim checks whether an issue can be claimed per §6.1.
// Returns nil if the issue is claimable, or an appropriate error.
func ValidateClaim(status IssueClaimStatus, allowSteal bool, now time.Time) error {
	if status.IsDeleted {
		return fmt.Errorf("cannot claim deleted issue: %w", domain.ErrTerminalState)
	}

	if status.State == issue.StateClosed {
		return fmt.Errorf("cannot claim closed issue: %w", domain.ErrTerminalState)
	}

	if status.ActiveClaim.ID() != "" {
		if !allowSteal {
			return &domain.ClaimConflictError{
				IssueID:       status.ActiveClaim.IssueID().String(),
				CurrentHolder: status.ActiveClaim.Author().String(),
				StaleAt:       status.ActiveClaim.StaleAt(),
			}
		}

		if !status.ActiveClaim.IsStale(now) {
			return &domain.ClaimConflictError{
				IssueID:       status.ActiveClaim.IssueID().String(),
				CurrentHolder: status.ActiveClaim.Author().String(),
				StaleAt:       status.ActiveClaim.StaleAt(),
			}
		}

		// Claim is stale and steal is allowed — proceed.
	}

	return nil
}

// StealComment generates the auto-comment body added when an issue is stolen.
func StealComment(previousHolder string) string {
	return fmt.Sprintf("Stolen from %q.", previousHolder)
}
