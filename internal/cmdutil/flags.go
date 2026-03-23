package cmdutil

import "fmt"

// FlagErrorf creates a FlagError with a formatted message.
// Commands use this when flag validation fails — the top-level error handler
// will print the error along with usage text to guide the user.
func FlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}
