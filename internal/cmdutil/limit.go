package cmdutil

import "errors"

// ErrInvalidLimit indicates that a non-positive value was provided for the
// --limit flag. The value must be a positive integer. Callers should use
// --no-limit to request unbounded results instead.
var ErrInvalidLimit = errors.New("--limit must be a positive integer; use --no-limit for unbounded results")

// DefaultLimit is the default result limit for all list operations. Commands
// set this as the Value on their --limit IntFlag so the help text displays the
// correct default and the flag destination is pre-populated.
const DefaultLimit = 20

// ResolveLimit determines the effective limit for list operations based on
// the --limit flag and the --no-limit flag.
//
// The --limit flag's default value should be set to DefaultLimit at command
// construction time, so the limitFlag always carries a meaningful positive value
// when the user does not override it.
//
// Precedence: --no-limit (unlimited) > --limit N (explicit positive).
// A non-positive limitFlag value is rejected with ErrInvalidLimit.
func ResolveLimit(limitFlag int, noLimitFlag bool) (int, error) {
	if noLimitFlag {
		return -1, nil
	}
	if limitFlag < 1 {
		return 0, ErrInvalidLimit
	}
	return limitFlag, nil
}
