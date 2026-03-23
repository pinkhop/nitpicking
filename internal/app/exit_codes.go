package app

// ExitCode represents the process exit code returned to the operating system.
// Each value has a specific semantic meaning, allowing scripts and CI systems
// to distinguish between different failure modes.
type ExitCode int

const (
	// ExitOK indicates the command completed successfully.
	ExitOK ExitCode = 0

	// ExitError indicates a general error — the default for unclassified failures.
	ExitError ExitCode = 1

	// ExitCancel indicates the user cancelled the operation (Ctrl+C, prompt
	// cancellation, or context cancellation).
	ExitCancel ExitCode = 2

	// ExitPending indicates the operation was accepted but is not yet complete.
	// Used for asynchronous workflows where the result will be available later.
	ExitPending ExitCode = 8
)
