package issue

// BlockerStatus summarizes a blocked_by target's state for readiness checks.
type BlockerStatus struct {
	// IsClosed is true if the blocker is closed (resolved).
	IsClosed bool
	// IsDeleted is true if the blocker has been soft-deleted.
	IsDeleted bool
}

// AncestorStatus summarizes an ancestor's state for readiness propagation.
type AncestorStatus struct {
	// State is the ancestor's current state.
	State State
	// IsBlocked is true when the ancestor has at least one unresolved
	// blocked_by relationship. A blocked ancestor gates readiness for
	// all descendants, mirroring the behavior of deferred ancestors.
	IsBlocked bool
}

// IsTaskReady determines whether a task is ready for work.
//
// A task is ready when:
//  1. Its state is open.
//  2. It has no unresolved blocked_by relationships (closed or deleted
//     targets count as resolved).
//  3. No ancestor is deferred or blocked.
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
		if a.State == StateDeferred || a.IsBlocked {
			return false
		}
	}

	return true
}

// IsEpicReady determines whether an epic is ready for decomposition.
//
// An epic is ready when:
//  1. Its state is open.
//  2. It has no children (needs decomposition).
//  3. It has no unresolved blocked_by relationships.
//  4. No ancestor is deferred or blocked.
func IsEpicReady(state State, hasChildren bool, blockers []BlockerStatus, ancestors []AncestorStatus) bool {
	if state != StateOpen {
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
		if a.State == StateDeferred || a.IsBlocked {
			return false
		}
	}

	return true
}

// blockerResolved reports whether a blocked_by target is resolved. A blocker
// is resolved when it has been closed or soft-deleted.
func blockerResolved(b BlockerStatus) bool {
	return b.IsClosed || b.IsDeleted
}
