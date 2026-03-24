package issue

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// State represents the lifecycle state of an issue. All issue roles (task
// and epic) share the same state machine.
type State int

const (
	// StateOpen is the default state for new issues. Available for work.
	StateOpen State = iota + 1

	// StateClaimed indicates an agent or human has taken ownership.
	StateClaimed

	// StateClosed indicates the issue is fully resolved. Terminal — cannot
	// be reclaimed or reopened.
	StateClosed

	// StateDeferred indicates the issue should not be worked on now.
	StateDeferred

	// StateWaiting indicates the issue cannot proceed until something
	// external happens.
	StateWaiting
)

// stateStrings maps each State to its canonical lowercase string.
var stateStrings = map[State]string{
	StateOpen:     "open",
	StateClaimed:  "claimed",
	StateClosed:   "closed",
	StateDeferred: "deferred",
	StateWaiting:  "waiting",
}

// String returns the canonical lowercase string representation.
func (s State) String() string {
	if str, ok := stateStrings[s]; ok {
		return str
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// ParseState parses a state string into a State. Parsing is case-sensitive.
func ParseState(s string) (State, error) {
	for state, str := range stateStrings {
		if s == str {
			return state, nil
		}
	}
	return 0, fmt.Errorf("invalid state %q", s)
}

// IsTerminal reports whether the state is terminal — no further transitions
// are allowed. Only "closed" is terminal within the state machine. "Deleted"
// is a separate concept checked independently.
func (s State) IsTerminal() bool {
	return s == StateClosed
}

// transitions defines the legal state transitions for all issues.
// Key: current state → Value: set of allowed next states.
var transitions = map[State]map[State]bool{
	StateOpen:     {StateClaimed: true},
	StateClaimed:  {StateOpen: true, StateClosed: true, StateDeferred: true, StateWaiting: true},
	StateDeferred: {StateClaimed: true},
	StateWaiting:  {StateClaimed: true},
	// StateClosed is terminal — no transitions out.
}

// Transition validates a state transition for any issue role. Returns
// ErrIllegalTransition if the transition is not allowed, or
// ErrTerminalState if the current state is terminal.
func Transition(current, next State) error {
	if current.IsTerminal() {
		return fmt.Errorf("cannot transition from %s: %w", current, domain.ErrTerminalState)
	}

	allowed, ok := transitions[current]
	if !ok {
		return fmt.Errorf("unknown state %s: %w", current, domain.ErrIllegalTransition)
	}

	if !allowed[next] {
		return fmt.Errorf("cannot transition from %s to %s: %w", current, next, domain.ErrIllegalTransition)
	}

	return nil
}

// DefaultState returns the initial state for any newly created issue.
func DefaultState() State {
	return StateOpen
}

// ReleaseState returns the state an issue transitions to when released
// from claimed.
func ReleaseState() State {
	return StateOpen
}
