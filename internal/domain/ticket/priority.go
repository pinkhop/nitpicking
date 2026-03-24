package ticket

import (
	"fmt"
	"strings"
)

// Priority represents the urgency of a ticket. Lower numbers indicate higher
// urgency. The default priority is P2.
type Priority int

const (
	// P0 is the highest urgency — critical, drop-everything priority.
	// The enum starts at iota+1 so that the zero value of Priority is not a
	// valid priority, allowing constructors to distinguish "not set" from P0.
	P0 Priority = iota + 1

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
// Indexed by Priority value directly; index 0 is unused because the zero
// value is reserved as the "unset" sentinel.
var priorityStrings = [...]string{
	P0: "P0",
	P1: "P1",
	P2: "P2",
	P3: "P3",
	P4: "P4",
}

// String returns the canonical string representation (e.g., "P2").
func (p Priority) String() string {
	if p >= P0 && p <= P4 {
		return priorityStrings[p]
	}
	return fmt.Sprintf("Priority(%d)", int(p))
}

// ParsePriority parses a priority string into a Priority. Accepts canonical
// form ("P0"–"P4"), lowercase ("p0"–"p4"), and bare numeric ("0"–"4").
func ParsePriority(s string) (Priority, error) {
	normalized := strings.ToUpper(s)

	// Accept bare numeric: "0" → "P0", "4" → "P4".
	if len(normalized) == 1 && normalized[0] >= '0' && normalized[0] <= '9' {
		normalized = "P" + normalized
	}

	for p := P0; p <= P4; p++ {
		if normalized == priorityStrings[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("invalid priority %q: must be P0–P4", s)
}

// IsHigherThan reports whether p has higher urgency than other. Lower
// numeric value means higher urgency.
func (p Priority) IsHigherThan(other Priority) bool {
	return p < other
}
