package domain

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for domain failure categories. Adapters use errors.Is to
// classify these into exit codes or HTTP status codes.
var (
	// ErrNotFound indicates a requested entity (issue, comment, etc.) does not
	// exist or has been soft-deleted.
	ErrNotFound = errors.New("not found")

	// ErrIllegalTransition indicates a state machine transition that violates
	// the defined transition rules.
	ErrIllegalTransition = errors.New("illegal state transition")

	// ErrCycleDetected indicates a parent assignment or relationship would
	// create a cycle in the hierarchy.
	ErrCycleDetected = errors.New("cycle detected")

	// ErrDeletedIssue indicates an operation was attempted on a soft-deleted
	// issue, which is immutable.
	ErrDeletedIssue = errors.New("issue is deleted")

	// ErrTerminalState indicates an operation was attempted on an issue in a
	// terminal state (closed or deleted) that forbids further mutations.
	ErrTerminalState = errors.New("issue is in a terminal state")

	// ErrDepthExceeded indicates a parent assignment would exceed the maximum
	// hierarchy depth (3 levels).
	ErrDepthExceeded = errors.New("hierarchy depth exceeded")

	// ErrStaleClaim indicates an operation was attempted with a claim that has
	// passed its stale-at timestamp. The caller must re-claim the issue before
	// retrying the operation.
	ErrStaleClaim = errors.New("claim is stale")

	// ErrSchemaMigrationRequired indicates the database is at an older schema
	// version (v1) and must be upgraded before most commands can operate. The
	// caller should instruct the user to run 'np admin upgrade'.
	ErrSchemaMigrationRequired = errors.New("database schema migration required")

	// ErrCorruptDatabase indicates the database file exists but does not contain
	// the expected schema. This occurs when the file is empty, truncated, or
	// otherwise corrupt. The caller should direct the user to remove or replace
	// the file and re-run 'np init', or run 'np admin doctor' for diagnostics.
	ErrCorruptDatabase = errors.New("database is corrupt or uninitialized")

	// ErrDatabaseNotInitialized indicates that the .np/ directory exists but
	// the database file has not been created yet. The caller should direct the
	// user to run 'np init' to initialize the workspace.
	ErrDatabaseNotInitialized = errors.New("workspace not initialized")
)

// ValidationError carries structured detail about which fields failed
// validation and why, enabling self-describing error responses per §9.
type ValidationError struct {
	// Fields maps field names to human-readable descriptions of why
	// validation failed. Multiple fields may fail in a single operation.
	Fields map[string]string
}

// Error returns a summary of all validation failures.
func (e *ValidationError) Error() string {
	if len(e.Fields) == 1 {
		for field, reason := range e.Fields {
			return fmt.Sprintf("validation failed: %s: %s", field, reason)
		}
	}
	return fmt.Sprintf("validation failed: %d fields invalid", len(e.Fields))
}

// Is reports whether target is a *ValidationError, enabling errors.Is checks
// against a nil *ValidationError sentinel.
func (e *ValidationError) Is(target error) bool {
	_, ok := target.(*ValidationError)
	return ok
}

// NewValidationError creates a ValidationError for a single field.
func NewValidationError(field, reason string) *ValidationError {
	return &ValidationError{
		Fields: map[string]string{field: reason},
	}
}

// NewMultiValidationError creates a ValidationError for multiple fields.
func NewMultiValidationError(fields map[string]string) *ValidationError {
	return &ValidationError{Fields: fields}
}

// ClaimConflictError indicates an issue is already claimed and the claim is
// not stale. It carries structured context so that callers (especially AI
// agents) can decide whether to wait or steal.
type ClaimConflictError struct {
	// IssueID is the ID of the issue that could not be claimed.
	IssueID string

	// CurrentHolder is the author who holds the active claim.
	CurrentHolder string

	// StaleAt is the timestamp at which the current claim becomes stale
	// and eligible for stealing.
	StaleAt time.Time
}

// Error returns a human-readable description of the claim conflict.
func (e *ClaimConflictError) Error() string {
	return fmt.Sprintf(
		"claim conflict: issue %s is claimed by %q until %s",
		e.IssueID, e.CurrentHolder, e.StaleAt.Format(time.RFC3339),
	)
}

// Is reports whether target is a *ClaimConflictError, enabling errors.Is
// checks against a nil *ClaimConflictError sentinel.
func (e *ClaimConflictError) Is(target error) bool {
	_, ok := target.(*ClaimConflictError)
	return ok
}

// DatabaseError wraps a storage-layer error with additional context. The
// adapter layer maps this to exit code 5.
type DatabaseError struct {
	// Op describes the operation that failed (e.g., "create issue", "begin transaction").
	Op string

	// Err is the underlying storage error.
	Err error
}

// Error returns the operation and underlying error message.
func (e *DatabaseError) Error() string {
	return fmt.Sprintf("database error during %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error for use with errors.Is and errors.As.
func (e *DatabaseError) Unwrap() error { return e.Err }

// Is reports whether target is a *DatabaseError, enabling errors.Is checks
// against a nil *DatabaseError sentinel.
func (e *DatabaseError) Is(target error) bool {
	_, ok := target.(*DatabaseError)
	return ok
}
