package core

import "github.com/pinkhop/nitpicking/internal/domain"

// IsTaskReady determines whether a task is ready for work.
//
// A task is ready when:
//  1. Its state is open.
//  2. It has no active (non-stale) claim — claimed issues are already being
//     worked on and are not available for new claimants.
//  3. It has no unresolved blocked_by relationships (closed or deleted
//     targets count as resolved).
//  4. No ancestor is deferred or blocked.
func IsTaskReady(state domain.State, hasActiveClaim bool, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) bool {
	if state != domain.StateOpen {
		return false
	}
	if hasActiveClaim {
		return false
	}
	return !isBlocked(blockers, ancestors)
}

// IsEpicReady determines whether an epic is ready for decomposition.
//
// An epic is ready when:
//  1. Its state is open.
//  2. It has no active (non-stale) claim — claimed epics are already being
//     decomposed and are not available for new claimants.
//  3. It has no children (needs decomposition).
//  4. It has no unresolved blocked_by relationships.
//  5. No ancestor is deferred or blocked.
func IsEpicReady(state domain.State, hasActiveClaim bool, hasChildren bool, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) bool {
	if state != domain.StateOpen {
		return false
	}
	if hasActiveClaim {
		return false
	}
	if hasChildren {
		return false
	}
	return !isBlocked(blockers, ancestors)
}

// TaskSecondaryState computes the secondary state for a task given its
// primary state, claim status, blockers, and ancestor statuses.
//
// Rules:
//   - open + active claim → claimed (takes priority over ready/blocked)
//   - open + no active claim + not blocked → ready
//   - open + no active claim + blocked → blocked
//   - deferred + blocked → blocked
//   - deferred + not blocked → none
//   - closed → none
func TaskSecondaryState(state domain.State, hasActiveClaim bool, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) domain.SecondaryStateResult {
	switch state {
	case domain.StateClosed:
		return domain.SecondaryStateResult{}

	case domain.StateOpen:
		// An active claim takes display priority over ready and blocked: the
		// issue is being worked on, not available for new work.
		if hasActiveClaim {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryClaimed,
				DetailStates: []domain.SecondaryState{domain.SecondaryClaimed},
			}
		}
		if isBlocked(blockers, ancestors) {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryBlocked,
				DetailStates: []domain.SecondaryState{domain.SecondaryBlocked},
			}
		}
		return domain.SecondaryStateResult{
			ListState:    domain.SecondaryReady,
			DetailStates: []domain.SecondaryState{domain.SecondaryReady},
		}

	case domain.StateDeferred:
		if isBlocked(blockers, ancestors) {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryBlocked,
				DetailStates: []domain.SecondaryState{domain.SecondaryBlocked},
			}
		}
		return domain.SecondaryStateResult{}

	default:
		return domain.SecondaryStateResult{}
	}
}

// isBlocked reports whether an issue is blocked by unresolved blockers or
// by a deferred/blocked ancestor.
func isBlocked(blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) bool {
	for _, b := range blockers {
		if !blockerResolved(b) {
			return true
		}
	}
	for _, a := range ancestors {
		if a.State == domain.StateDeferred || a.IsBlocked {
			return true
		}
	}
	return false
}

// blockerResolved reports whether a blocked_by target is resolved. A blocker
// is resolved when it has been closed or soft-deleted.
func blockerResolved(b domain.BlockerStatus) bool {
	return b.IsClosed || b.IsDeleted
}
