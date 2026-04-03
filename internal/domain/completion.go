package domain

// ChildStatus summarizes a child issue's state for parent-close validation
// and epic progress computation.
type ChildStatus struct {
	// State is the child's current state.
	State State
	// IsBlocked is true when the child has at least one unresolved
	// blocked_by relationship. Used by epic progress bars to distinguish
	// open-and-blocked from open-and-ready.
	IsBlocked bool
}
