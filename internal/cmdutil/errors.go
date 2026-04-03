package cmdutil

import "errors"

// ErrSilent is a sentinel error indicating the error message has already
// been printed to the user. The top-level error handler should exit with a
// non-zero code but not print the error again. Commands use this after
// displaying a formatted error message directly to IOStreams.ErrOut.
var ErrSilent = errors.New("silent error")

// FlagError indicates invalid flag usage or flag combination.
// The top-level error handler should print the error along with the
// command's usage text to help the user correct their invocation.
type FlagError struct {
	Err error
}

// Error returns the underlying error message.
func (e *FlagError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error for use with errors.Is and errors.As.
func (e *FlagError) Unwrap() error { return e.Err }
