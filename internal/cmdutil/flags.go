package cmdutil

import "fmt"

const (
	// FlagCategoryRequired is the flag category for mandatory flags. The user
	// must supply these for the command to succeed.
	FlagCategoryRequired = "Required"

	// FlagCategorySupplemental is the flag category for optional flags that
	// modify command behavior but are not strictly necessary.
	FlagCategorySupplemental = "Supplemental"
)

// FlagErrorf creates a FlagError with a formatted message.
// Commands use this when flag validation fails — the top-level error handler
// will print the error along with usage text to guide the user.
func FlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{Err: fmt.Errorf(format, args...)}
}
