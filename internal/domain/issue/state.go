package issue

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// State represents the lifecycle state of an issue. Task and epic state
// machines share some states but differ in allowed transitions and terminal
// states.
type State int

const (
	// StateOpen is the default state for new tasks. Available for work.
	StateOpen State = iota + 1

	// StateActive is the default state for new epics. Children follow their
	// own lifecycles.
	StateActive

	// StateClaimed indicates an agent or human has taken ownership.
	StateClaimed

	// StateClosed indicates a task is fully resolved. Terminal — cannot be
	// reclaimed or reopened. Only valid for tasks.
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
	StateActive:   "active",
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
// are allowed. For the issue domain, only "closed" is a terminal state
// within the state machine. "Deleted" is a separate concept checked
// independently.
func (s State) IsTerminal() bool {
	return s == StateClosed
}

// taskTransitions defines the legal state transitions for tasks.
// Key: current state → Value: set of allowed next states.
var taskTransitions = map[State]map[State]bool{
	StateOpen:     {StateClaimed: true},
	StateClaimed:  {StateOpen: true, StateClosed: true, StateDeferred: true, StateWaiting: true},
	StateDeferred: {StateClaimed: true},
	StateWaiting:  {StateClaimed: true},
	// StateClosed is terminal — no transitions out.
}

// epicTransitions defines the legal state transitions for epics.
// Epics have no closed state — completion is derived.
var epicTransitions = map[State]map[State]bool{
	StateActive:   {StateClaimed: true},
	StateClaimed:  {StateActive: true, StateDeferred: true, StateWaiting: true},
	StateDeferred: {StateClaimed: true},
	StateWaiting:  {StateClaimed: true},
}

// TransitionTask validates and returns the next state for a task transition.
// Returns ErrIllegalTransition if the transition is not allowed, or
// ErrTerminalState if the current state is terminal.
func TransitionTask(current, next State) error {
	if current.IsTerminal() {
		return fmt.Errorf("cannot transition from %s: %w", current, domain.ErrTerminalState)
	}

	allowed, ok := taskTransitions[current]
	if !ok {
		return fmt.Errorf("unknown task state %s: %w", current, domain.ErrIllegalTransition)
	}

	if !allowed[next] {
		return fmt.Errorf("cannot transition task from %s to %s: %w", current, next, domain.ErrIllegalTransition)
	}

	return nil
}

// TransitionEpic validates and returns the next state for an epic transition.
// Returns ErrIllegalTransition if the transition is not allowed.
func TransitionEpic(current, next State) error {
	allowed, ok := epicTransitions[current]
	if !ok {
		return fmt.Errorf("unknown epic state %s: %w", current, domain.ErrIllegalTransition)
	}

	if !allowed[next] {
		return fmt.Errorf("cannot transition epic from %s to %s: %w", current, next, domain.ErrIllegalTransition)
	}

	return nil
}

// DefaultStateForRole returns the initial state for a given role.
// Tasks start as open; epics start as active.
func DefaultStateForRole(r Role) State {
	if r == RoleEpic {
		return StateActive
	}
	return StateOpen
}

// ReleaseStateForRole returns the state an issue transitions to when released
// from claimed. Tasks return to open; epics return to active.
func ReleaseStateForRole(r Role) State {
	return DefaultStateForRole(r)
}
