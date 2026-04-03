package cmdutil

// InteractiveLimit is the default result limit when stdout is a terminal.
// Larger than piped mode because human users benefit from seeing more context.
const InteractiveLimit = 50

// PipedLimit is the default result limit when stdout is piped (agent mode).
// Smaller because agents typically parse structured output and can request more.
const PipedLimit = 20

// ResolveLimit determines the effective limit for list operations based on
// the --limit flag, the --all flag, and whether stdout is a TTY.
//
// Precedence: --all (unlimited) > --limit N (explicit) > TTY-aware default.
func ResolveLimit(limitFlag int, allFlag bool, isTTY bool) int {
	if allFlag {
		return -1
	}
	if limitFlag != 0 {
		return limitFlag
	}
	if isTTY {
		return InteractiveLimit
	}
	return PipedLimit
}
