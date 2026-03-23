package ticket

import "fmt"

// Role identifies whether a ticket is an Epic (organizer) or a Task (leaf
// work unit). Role is immutable after creation.
type Role int

const (
	// RoleTask represents an actionable work unit — the leaf node in the
	// ticket hierarchy.
	RoleTask Role = iota + 1

	// RoleEpic represents an organizing ticket whose completion is derived
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
