package domain

import "fmt"

// SecondaryState qualifies the primary state with additional context about an
// issue's readiness, progress, or blocking status. The zero value
// (SecondaryNone) indicates no secondary qualifier — used for closed issues or
// when the primary state does not warrant a qualifier.
type SecondaryState int

const (
	// SecondaryNone indicates no secondary state. Used for closed issues or
	// when the primary state does not warrant a qualifier.
	SecondaryNone SecondaryState = iota

	// SecondaryClaimed indicates an open issue has an active (non-stale) claim.
	// Claimed takes precedence over ready and blocked in display priority.
	SecondaryClaimed

	// SecondaryReady indicates an issue is available for work (tasks) or
	// decomposition (epics with no children).
	SecondaryReady

	// SecondaryBlocked indicates the issue has unresolved blockers or a
	// blocked/deferred ancestor.
	SecondaryBlocked

	// SecondaryUnplanned indicates an epic has no children and needs
	// decomposition. Used in detail views alongside SecondaryBlocked when
	// an unplanned epic is also blocked.
	SecondaryUnplanned

	// SecondaryActive indicates an epic has children but not all are closed.
	SecondaryActive

	// SecondaryCompleted indicates an epic has children and all are closed.
	SecondaryCompleted
)

// secondaryStateStrings maps each SecondaryState to its canonical string.
// SecondaryNone maps to the empty string.
var secondaryStateStrings = map[SecondaryState]string{
	SecondaryNone:      "",
	SecondaryClaimed:   "claimed",
	SecondaryReady:     "ready",
	SecondaryBlocked:   "blocked",
	SecondaryUnplanned: "unplanned",
	SecondaryActive:    "active",
	SecondaryCompleted: "completed",
}

// String returns the canonical string representation. Returns an empty string
// for SecondaryNone.
func (s SecondaryState) String() string {
	if str, ok := secondaryStateStrings[s]; ok {
		return str
	}
	return fmt.Sprintf("SecondaryState(%d)", int(s))
}

// ParseSecondaryState parses a secondary state string. Parsing is
// case-sensitive. The empty string is not accepted — callers should check for
// SecondaryNone explicitly rather than parsing it.
func ParseSecondaryState(s string) (SecondaryState, error) {
	for ss, str := range secondaryStateStrings {
		if ss == SecondaryNone {
			continue
		}
		if s == str {
			return ss, nil
		}
	}
	return SecondaryNone, fmt.Errorf("invalid secondary state %q", s)
}

// SecondaryStateResult carries the computed secondary state for both list and
// detail views. ListState is a single secondary state chosen by priority rules
// for compact list displays. DetailStates is the full set of applicable
// secondary conditions for rich detail views.
type SecondaryStateResult struct {
	// ListState is the single secondary state for list views, chosen by
	// priority: completed > claimed > blocked > ready > active.
	ListState SecondaryState

	// DetailStates is the ordered set of secondary conditions applicable in
	// detail views. May contain multiple entries (e.g., [blocked, active]
	// for a blocked epic with children).
	DetailStates []SecondaryState
}

// HasSecondary reports whether this result carries any secondary state.
func (r SecondaryStateResult) HasSecondary() bool {
	return r.ListState != SecondaryNone
}
