package domain

import "fmt"

// ParseError represents a JSON parsing error on a specific line of the import
// file. Line is the 1-based line number.
type ParseError struct {
	Line int
	Err  error
}

// Error implements the error interface.
func (e ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Err)
}

// Unwrap returns the underlying error for use with errors.Is/As.
func (e ParseError) Unwrap() error {
	return e.Err
}

// LineError represents a validation error on a specific line of the import
// file. Line is the zero-based index into the input slice.
type LineError struct {
	Line    int
	Message string
}

// Error implements the error interface for display purposes.
func (e LineError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line+1, e.Message)
}
