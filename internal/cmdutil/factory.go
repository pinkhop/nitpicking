package cmdutil

import (
	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// Factory provides lazy-loaded, substitutable dependencies to commands.
// Function fields are not called until a command actually needs the dependency,
// and they can be replaced in tests with trivial stubs or panicking functions
// to catch accidental dependency usage.
//
// Eager fields (like IOStreams) are cheap to construct and used by virtually
// every command. As the application grows, expensive dependencies
// (HTTP clients, database pools) are added as function-typed fields whose
// cost is deferred until actual use.
//
// Function fields also serve as live configuration providers for long-running
// services. When a Factory function closes over a mutable configuration source
// (file watcher, K8s ConfigMap, Vault lease), callers that invoke the function
// per unit of work — per HTTP request, per batch iteration, per queue message —
// always receive resources built from the current configuration. This enables
// credential rotation, feature flag updates, and connection pool recycling
// without process restarts. Short-lived CLI commands can safely call Factory
// functions once; the pattern adds no overhead but does not provide its full
// benefit in that context.
type Factory struct {
	// AppName is the application binary name, used in version output and
	// user-facing messages.
	AppName string

	// AppVersion is the build version, injected at compile time via -ldflags
	// or set to "dev" during development.
	AppVersion string

	// BuildInfo holds version-control metadata (VCS name, commit, timestamp)
	// embedded by the Go toolchain at build time.
	BuildInfo BuildInfo

	// IOStreams provides abstracted I/O with TTY awareness and color control.
	// Constructed eagerly because it is needed by almost every command and
	// has no expensive initialization.
	IOStreams *iostreams.IOStreams

	// Workspace is the explicit workspace directory path, set via the
	// --workspace global flag or the NP_WORKSPACE environment variable. When
	// non-empty, database discovery uses this directory directly instead of
	// walking up from the current working directory. The flag takes
	// precedence over the environment variable.
	Workspace string

	// DatabasePath returns the absolute path to the database file, using
	// workspace-aware discovery. When Workspace is set, it checks that
	// directory directly via LookupDatabase; otherwise it walks up from the
	// current working directory via DiscoverDatabase. Commands that need the
	// .np/ directory path (for backup files, reset-key hashes, or path
	// reporting) should call this rather than invoking DiscoverDatabase
	// directly, which would bypass the --workspace override.
	DatabasePath func() (string, error)

	// Store returns the SQLite database connection. Constructed lazily on
	// first access — database discovery and opening happen at that point.
	// Commands that need the application service construct it from this
	// connection via cmdutil.NewTracker.
	Store func() (*sqlite.Store, error)

	// SignalCancelIsError controls whether signal-triggered context
	// cancellation (SIGINT, SIGTERM) produces a non-zero exit code.
	//
	// Long-running processes — HTTP servers, Kafka consumers, queue workers —
	// receive SIGTERM as a routine part of their lifecycle (e.g., Kubernetes
	// pod eviction, rolling deploys). For these, signal cancellation is
	// expected and should exit cleanly with code 0, so leave this false (the
	// default).
	//
	// Short-lived processes — CLI tools, Kubernetes Jobs, cron jobs — run to
	// completion under normal conditions. A signal interrupting one of these
	// typically means something went wrong (user abort, preemption, timeout),
	// so set this to true to surface the interruption as a non-zero exit code.
	SignalCancelIsError bool
}
