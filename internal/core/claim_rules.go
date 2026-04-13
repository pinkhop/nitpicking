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
// Claims are only valid on open issues. Closed and deferred issues cannot be
// claimed — reopen operations are claim-free. A stale claim is treated as
// nonexistent — the caller is responsible for deleting or overwriting the
// expired row before creating the new claim. An active (non-stale) claim
// always produces a ClaimConflictError, because steal mechanics have been
// removed; callers must wait for the existing claim to expire.
//
// Returns nil if the issue is claimable, or an appropriate error.
func ValidateClaim(status IssueClaimStatus, now time.Time) error {
	if status.IsDeleted {
		return fmt.Errorf("cannot claim deleted issue: %w", domain.ErrTerminalState)
	}

	if status.State != domain.StateOpen {
		return fmt.Errorf("cannot claim %s issue: only open issues can be claimed: %w",
			status.State, domain.ErrIllegalTransition)
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

// ValidateActiveClaim checks that a claim retrieved from storage has not yet
// gone stale. Operations that mutate an issue (update, close, defer, delete)
// must call this after loading the claim so that expired claims are rejected
// with a clear error rather than silently accepted.
//
// Returns nil if the claim is still active, or ErrStaleClaim if it has
// passed its stale-at timestamp.
func ValidateActiveClaim(c domain.Claim, now time.Time) error {
	if c.IsStale(now) {
		return fmt.Errorf("claim %s expired at %s; re-claim the issue before retrying: %w",
			c.ID(), c.StaleAt().Format(time.RFC3339), domain.ErrStaleClaim)
	}
	return nil
}
