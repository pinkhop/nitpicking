package issue

import "fmt"

// Role identifies whether an issue is an Epic (organizer) or a Task (leaf
// work unit). Role is immutable after creation.
type Role int

const (
	// RoleTask represents an actionable work unit — the leaf node in the
	// issue hierarchy.
	RoleTask Role = iota + 1

	// RoleEpic represents an organizing issue whose completion is derived
	// from its children.
	RoleEpic
)

// String returns the canonical lowercase string representation.
func (r Role) String() string {
	switch r {
	case RoleTask:
		return "task"
	case RoleEpic:
		return "epic"
	default:
		return fmt.Sprintf("Role(%d)", int(r))
	}
}

// ParseRole parses a role string ("task" or "epic") into a Role. Parsing is
// case-sensitive.
func ParseRole(s string) (Role, error) {
	switch s {
	case "task":
		return RoleTask, nil
	case "epic":
		return RoleEpic, nil
	default:
		return 0, fmt.Errorf("invalid role %q: must be \"task\" or \"epic\"", s)
	}
}
