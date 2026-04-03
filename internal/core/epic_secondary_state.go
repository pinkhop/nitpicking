package core

import "github.com/pinkhop/nitpicking/internal/domain"

// EpicSecondaryState computes the secondary state for an epic based on its
// primary state, child status, blockers, and ancestor conditions.
//
// The function applies list-view priority rules: completed > blocked > ready >
// active. Detail-view states capture the full set of applicable conditions
// (e.g., [blocked, active] for a blocked epic with in-progress children).
//
// Returns a zero-value SecondaryStateResult (ListState = SecondaryNone) for
// claimed and closed epics, or for deferred epics that are not blocked.
func EpicSecondaryState(
	state domain.State,
	hasChildren bool,
	allChildrenClosed bool,
	blockers []domain.BlockerStatus,
	ancestors []domain.AncestorStatus,
) domain.SecondaryStateResult {
	switch state {
	case domain.StateClaimed, domain.StateClosed:
		return domain.SecondaryStateResult{}

	case domain.StateDeferred:
		if isBlocked(blockers, ancestors) {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryBlocked,
				DetailStates: []domain.SecondaryState{domain.SecondaryBlocked},
			}
		}
		return domain.SecondaryStateResult{}

	case domain.StateOpen:
		return epicOpenSecondaryState(hasChildren, allChildrenClosed, blockers, ancestors)

	default:
		return domain.SecondaryStateResult{}
	}
}

// epicOpenSecondaryState handles the open-state branches for epic secondary
// state computation. Split from EpicSecondaryState for readability — the open
// state has six distinct paths depending on child status and blockers.
func epicOpenSecondaryState(
	hasChildren bool,
	allChildrenClosed bool,
	blockers []domain.BlockerStatus,
	ancestors []domain.AncestorStatus,
) domain.SecondaryStateResult {
	blocked := isBlocked(blockers, ancestors)

	if !hasChildren {
		if blocked {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryBlocked,
				DetailStates: []domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryUnplanned},
			}
		}
		return domain.SecondaryStateResult{
			ListState:    domain.SecondaryReady,
			DetailStates: []domain.SecondaryState{domain.SecondaryReady},
		}
	}

	if allChildrenClosed {
		// Completed wins over blocked in list-view priority.
		if blocked {
			return domain.SecondaryStateResult{
				ListState:    domain.SecondaryCompleted,
				DetailStates: []domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryCompleted},
			}
		}
		return domain.SecondaryStateResult{
			ListState:    domain.SecondaryCompleted,
			DetailStates: []domain.SecondaryState{domain.SecondaryCompleted},
		}
	}

	// Has children, not all closed.
	if blocked {
		return domain.SecondaryStateResult{
			ListState:    domain.SecondaryBlocked,
			DetailStates: []domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryActive},
		}
	}
	return domain.SecondaryStateResult{
		ListState:    domain.SecondaryActive,
		DetailStates: []domain.SecondaryState{domain.SecondaryActive},
	}
}
