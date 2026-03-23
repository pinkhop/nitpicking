package ticket

// ChildStatus summarizes a child ticket's completion state for the purpose
// of deriving epic completion.
type ChildStatus struct {
	// Role is the child's role (task or epic).
	Role Role
	// State is the child's current state (relevant for tasks — closed means complete).
	State State
	// IsComplete is true if the child is complete. For tasks, this means
	// closed. For epics, this is recursively derived.
	IsComplete bool
}

// IsEpicComplete derives whether an epic is complete per §6.2.
//
// An epic is complete when it has children AND all of them are closed (tasks)
// or complete (sub-epics). An epic with no children is incomplete.
func IsEpicComplete(children []ChildStatus) bool {
	if len(children) == 0 {
		return false
	}

	for _, child := range children {
		switch child.Role {
		case RoleTask:
			if child.State != StateClosed {
				return false
			}
		case RoleEpic:
			if !child.IsComplete {
				return false
			}
		}
	}

	return true
}
