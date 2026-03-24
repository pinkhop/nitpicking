package issue

// BlockerStatus summarizes a blocked_by target's state for readiness checks.
type BlockerStatus struct {
	// IsClosed is true if the blocker is in a terminal state (closed).
	IsClosed bool
	// IsDeleted is true if the blocker has been soft-deleted.
	IsDeleted bool
	// IsComplete is true if the blocker is a complete epic (all children
	// closed or recursively complete). Epics have no closed state, so
	// derived completion is the only way an epic blocker can be resolved
	// without deletion.
	IsComplete bool
}

// AncestorStatus summarizes an ancestor epic's state for readiness propagation.
type AncestorStatus struct {
	// State is the ancestor's current state.
	State State
}

// IsTaskReady determines whether a task is ready for work per §6.3.
//
// A task is ready when:
//  1. Its state is open.
//  2. It has no unresolved blocked_by relationships (closed, deleted, or
//     complete targets count as resolved).
//  3. No ancestor epic is deferred or waiting.
func IsTaskReady(state State, blockers []BlockerStatus, ancestors []AncestorStatus) bool {
	if state != StateOpen {
		return false
	}

	for _, b := range blockers {
		if !blockerResolved(b) {
			return false
		}
	}

	for _, a := range ancestors {
		if a.State == StateDeferred || a.State == StateWaiting {
			return false
		}
	}

	return true
}

// IsEpicReady determines whether an epic is ready for decomposition per §6.3.
//
// An epic is ready when:
//  1. Its state is active.
//  2. It has no children (needs decomposition).
//  3. It has no unresolved blocked_by relationships.
//  4. No ancestor epic is deferred or waiting.
func IsEpicReady(state State, hasChildren bool, blockers []BlockerStatus, ancestors []AncestorStatus) bool {
	if state != StateActive {
		return false
	}

	if hasChildren {
		return false
	}

	for _, b := range blockers {
		if !blockerResolved(b) {
			return false
		}
	}

	for _, a := range ancestors {
		if a.State == StateDeferred || a.State == StateWaiting {
			return false
		}
	}

	return true
}

// blockerResolved reports whether a blocked_by target is resolved. A blocker
// is resolved when it has been closed, soft-deleted, or — for epics — derived
// as complete (all children closed or recursively complete).
func blockerResolved(b BlockerStatus) bool {
	return b.IsClosed || b.IsDeleted || b.IsComplete
}
