package core

import "github.com/pinkhop/nitpicking/internal/domain"

// IsTaskReady determines whether a task is ready for work.
//
// A task is ready when:
//  1. Its state is open.
//  2. It has no unresolved blocked_by relationships (closed or deleted
//     targets count as resolved).
//  3. No ancestor is deferred or blocked.
func IsTaskReady(state domain.State, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) bool {
	if state != domain.StateOpen {
		return false
	}
	return !isBlocked(blockers, ancestors)
}

// IsEpicReady determines whether an epic is ready for decomposition.
//
// An epic is ready when:
//  1. Its state is open.
//  2. It has no children (needs decomposition).
//  3. It has no unresolved blocked_by relationships.
//  4. No ancestor is deferred or blocked.
func IsEpicReady(state domain.State, hasChildren bool, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) bool {
	if state != domain.StateOpen {
		return false
	}
	if hasChildren {
		return false
	}
	return !isBlocked(blockers, ancestors)
}

// TaskSecondaryState computes the secondary state for a task given its
// primary state, blockers, and ancestor statuses.
//
// Rules:
//   - open + not blocked → ready
//   - open + blocked (unresolved blockers or blocked/deferred ancestor) → blocked
//   - deferred + blocked → blocked
//   - deferred + not blocked → none
//   - claimed → none
//   - closed → none
func TaskSecondaryState(state domain.State, blockers []domain.BlockerStatus, ancestors []domain.AncestorStatus) domain.SecondaryStateResult {
	switch state {
	case domain.StateClaimed, domain.StateClosed:
		return domain.SecondaryStateResult{}

	case domain.StateOpen:
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
