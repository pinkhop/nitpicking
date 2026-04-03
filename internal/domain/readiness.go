package domain

// BlockerStatus summarizes a blocked_by target's state for readiness checks.
type BlockerStatus struct {
	// IsClosed is true if the blocker is closed (resolved).
	IsClosed bool
	// IsDeleted is true if the blocker has been soft-deleted.
	IsDeleted bool
}

// AncestorStatus summarizes an ancestor's state for readiness propagation.
type AncestorStatus struct {
	// ID identifies the ancestor issue.
	ID ID
	// State is the ancestor's current state.
	State State
	// IsBlocked is true when the ancestor has at least one unresolved
	// blocked_by relationship. A blocked ancestor gates readiness for
	// all descendants, mirroring the behavior of deferred ancestors.
	IsBlocked bool
}
