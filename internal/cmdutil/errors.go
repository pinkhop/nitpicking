package cmdutil

import "errors"

// ErrSilent is a sentinel error indicating the error message has already
// been printed to the user. The top-level error handler should exit with a
// non-zero code but not print the error again. Commands use this after
// displaying a formatted error message directly to IOStreams.ErrOut.
var ErrSilent = errors.New("silent error")

// ExitCodeError signals a specific exit code without printing an error
// message. Commands use this when they have already written all output and
// need to convey a non-standard exit code — for example, np admin doctor
// uses exit 1 for warnings and exit 2 for errors (different semantics from
// the global ErrSilent → ExitError mapping).
type ExitCodeError struct {
	Code ExitCode
}

// Error returns an empty string so that ClassifyError does not print a
// redundant message: the command has already written its output.
//
// CAUTION: because Error() returns "", wrapping this error with fmt.Errorf
// would produce "context: " (trailing empty string), which is confusing.
// ExitCodeError must never be wrapped — pass it directly as the action's
// return value so ClassifyError sees it via errors.AsType.
func (e *ExitCodeError) Error() string { return "" }

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
