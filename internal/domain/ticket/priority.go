package ticket

import "fmt"

// Priority represents the urgency of a ticket. Lower numbers indicate higher
// urgency. The default priority is P2.
type Priority int

const (
	// P0 is the highest urgency — critical, drop-everything priority.
	P0 Priority = iota

	// P1 is high urgency — should be addressed soon.
	P1

	// P2 is normal urgency — the default for new tickets.
	P2

	// P3 is low urgency — address when convenient.
	P3

	// P4 is the lowest urgency — nice-to-have.
	P4
)

// DefaultPriority is the priority assigned to tickets that do not specify one.
const DefaultPriority = P2

// priorityStrings maps each Priority to its canonical string representation.
var priorityStrings = [...]string{"P0", "P1", "P2", "P3", "P4"}

// String returns the canonical string representation (e.g., "P2").
func (p Priority) String() string {
	if p >= P0 && p <= P4 {
		return priorityStrings[p]
	}
	return fmt.Sprintf("Priority(%d)", int(p))
}

// ParsePriority parses a priority string (e.g., "P0", "P2") into a Priority.
// Parsing is case-sensitive — "p0" is not valid.
func ParsePriority(s string) (Priority, error) {
	for i, ps := range priorityStrings {
		if s == ps {
			return Priority(i), nil
		}
	}
	return 0, fmt.Errorf("invalid priority %q: must be P0–P4", s)
}

// IsHigherThan reports whether p has higher urgency than other. Lower
// numeric value means higher urgency.
func (p Priority) IsHigherThan(other Priority) bool {
	return p < other
}
