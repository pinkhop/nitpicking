package core

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// IssueClaimStatus summarizes an issue's state for claim validation.
type IssueClaimStatus struct {
	// State is the issue's current lifecycle state.
	State domain.State
	// IsDeleted is true if the issue has been soft-deleted.
	IsDeleted bool
	// ActiveClaim is the current claim on the issue, if any. A zero-value
	// Claim (empty ID) indicates no active claim.
	ActiveClaim domain.Claim
}

// ValidateClaim checks whether an issue can be claimed.
//
// A stale claim is treated as nonexistent — the caller is responsible for
// deleting or overwriting the expired row before creating the new claim.
// An active (non-stale) claim always produces a ClaimConflictError, because
// steal mechanics have been removed; callers must wait for the existing claim
// to expire.
//
// Returns nil if the issue is claimable, or an appropriate error.
func ValidateClaim(status IssueClaimStatus, now time.Time) error {
	if status.IsDeleted {
		return fmt.Errorf("cannot claim deleted issue: %w", domain.ErrTerminalState)
	}

	if status.ActiveClaim.ID() == "" {
		// No claim present — always claimable.
		return nil
	}

	if status.ActiveClaim.IsStale(now) {
		// Stale claims are treated as nonexistent; the caller will overwrite
		// the expired row when creating the new claim.
		return nil
	}

	return &domain.ClaimConflictError{
		IssueID:       status.ActiveClaim.IssueID().String(),
		CurrentHolder: status.ActiveClaim.Author().String(),
		StaleAt:       status.ActiveClaim.StaleAt(),
	}
}
