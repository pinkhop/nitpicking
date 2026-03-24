package issue

// ChildStatus summarizes a child issue's state for parent-close validation.
type ChildStatus struct {
	// State is the child's current state.
	State State
}
