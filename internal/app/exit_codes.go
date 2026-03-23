package app

// ExitCode represents the process exit code returned to the operating system.
// Each value has a specific semantic meaning per §9 of the specification,
// allowing scripts, CI systems, and AI agents to distinguish between
// different failure modes.
type ExitCode int

const (
	// ExitOK indicates the command completed successfully.
	ExitOK ExitCode = 0

	// ExitError indicates a general or unexpected error.
	ExitError ExitCode = 1

	// ExitNotFound indicates the requested entity (ticket, note, etc.)
	// does not exist.
	ExitNotFound ExitCode = 2

	// ExitClaimConflict indicates the ticket is already claimed and the
	// claim is not stale, or the claim ID does not match.
	ExitClaimConflict ExitCode = 3

	// ExitValidation indicates invalid input (bad flags, missing required
	// fields, constraint violations).
	ExitValidation ExitCode = 4

	// ExitDatabase indicates a database error (corruption, locked, etc.).
	ExitDatabase ExitCode = 5

	// ExitPending indicates the operation was accepted but is not yet
	// complete. Used for asynchronous workflows.
	ExitPending ExitCode = 8
)
