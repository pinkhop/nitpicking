package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/root"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

// appNameFromArgs derives the application name from the executable path in
// args (typically os.Args). It strips the directory component and, on Windows,
// the .exe extension, so the name matches the binary filename regardless of
// how it was invoked. Falls back to "app" if args is empty, which indicates
// an abnormal process launch.
func appNameFromArgs(args []string) string {
	if len(args) == 0 {
		return "app"
	}
	// filepath.Base is OS-aware: on Unix it treats only / as a separator;
	// on Windows it treats both / and \ as separators. This means the function
	// is correct for any path produced by the current platform's os.Args[0].
	name := filepath.Base(args[0])
	name = strings.TrimSuffix(name, ".exe")
	return name
}

// Main is the application's true entry point, called from cmd/np/main.go.
// It constructs all dependencies, runs the root command, and classifies the
// result into an exit code. This function never calls os.Exit — that
// responsibility belongs exclusively to cmd/np/main.go.
func Main() ExitCode {
	f := newFactory(appNameFromArgs(os.Args), version)

	rootCmd := root.NewRootCmd(f)

	err := rootCmd.Run(context.Background(), os.Args)
	if err == nil {
		return ExitOK
	}

	exitCode := classifyError(f.IOStreams.ErrOut, err, f.SignalCancelIsError)
	return exitCode
}

// classifyError maps a command error to an exit code and prints appropriate
// messages to stderr. This is the single place where error types are translated
// into user-visible behavior and exit codes.
//
// The signalCancelIsError parameter controls whether context.Canceled (typically
// from SIGINT/SIGTERM via signal.NotifyContext) produces a non-zero exit code.
// When false, signal cancellation is treated as graceful shutdown (exit 0) —
// the correct default for Kubernetes-deployed services where SIGTERM is a normal
// lifecycle event.
//
// Uses Go 1.26's errors.AsType for type-safe, generic error classification —
// no need for a target variable declaration before the call.
func classifyError(stderr io.Writer, err error, signalCancelIsError bool) ExitCode {
	switch {
	case errors.Is(err, cmdutil.ErrSilent):
		// Error message already printed by the command.
		return ExitError

	case errors.Is(err, context.Canceled):
		if signalCancelIsError {
			return ExitError
		}
		return ExitOK

	default:
		// Map domain errors to specific exit codes per §9.
		if errors.Is(err, domain.ErrNotFound) {
			_, _ = fmt.Fprintln(stderr, err)
			return ExitNotFound
		}
		if errors.Is(err, &domain.ClaimConflictError{}) {
			_, _ = fmt.Fprintln(stderr, err)
			return ExitClaimConflict
		}
		if errors.Is(err, &domain.ValidationError{}) {
			_, _ = fmt.Fprintln(stderr, err)
			return ExitValidation
		}
		if errors.Is(err, &domain.DatabaseError{}) {
			_, _ = fmt.Fprintln(stderr, err)
			return ExitDatabase
		}

		if fe, ok := errors.AsType[*cmdutil.FlagError](err); ok {
			_, _ = fmt.Fprintln(stderr, fe) // #nosec G705 -- CLI stderr output, not rendered in a browser
			return ExitValidation
		}

		_, _ = fmt.Fprintln(stderr, err)
		return ExitError
	}
}

// newFactory constructs the Factory with all dependencies wired in the correct
// order. Both IOStreams and Logger are eager — cheap to construct and needed by
// virtually every command. As the application grows, expensive dependencies
// (HTTP clients, database pools) are added as function-typed fields whose
// construction is deferred until first use.
func newFactory(appName, appVersion string) *cmdutil.Factory {
	f := &cmdutil.Factory{
		AppName:    appName,
		AppVersion: appVersion,
		BuildInfo:  cmdutil.ReadBuildInfo(),
	}

	// Phase 1: IOStreams — constructed eagerly because it is cheap and
	// needed by virtually every command.
	f.IOStreams = iostreams.System()

	// Phase 2: Logger — constructed eagerly with a mutable LevelVar so it is
	// usable from the first log line. The level defaults to Info and is updated
	// by the root command's Before hook after flag parsing.
	f.LogLevel, f.Logger = newLogger(f.IOStreams)

	// Phase 3: Service — constructed lazily on first access. Database
	// discovery runs once; the Store is opened once and reused. Commands
	// that don't need the database (agent-name, agent-instructions) still
	// call through the service; the service handles non-database operations
	// without touching storage.
	var svc service.Service
	f.Service = func() service.Service {
		if svc != nil {
			return svc
		}

		// Try to discover an existing database. If not found, create a
		// service backed by a nil transactor — init will create the db.
		cwd, _ := os.Getwd()
		dbPath, err := sqlite.DiscoverDatabase(cwd)
		if err != nil {
			// No database found — return a service that can handle
			// non-database operations. Database commands will fail
			// with appropriate errors.
			dbPath, _ = sqlite.InitDatabaseDir(cwd)
		}

		store, err := sqlite.Open(dbPath)
		if err != nil {
			// Return a service with a nil-safe transactor — operations
			// will fail with database errors at call time.
			svc = service.New(nil)
			return svc
		}
		svc = service.New(store)
		return svc
	}

	return f
}

// newLogger constructs the application logger eagerly and returns both the
// mutable LevelVar and a closure that returns the logger. The logger starts
// at Info level; the root command's Before hook adjusts it after flag parsing
// by calling LevelVar.Set.
//
// The closure form exists solely as a testing seam — production callers always
// receive the same *slog.Logger instance. Using a LevelVar means the handler
// checks the current level on every log call, so updating the level after
// construction retroactively affects all logging.
//
// Output is always JSON, suitable for structured log ingestion.
func newLogger(ios *iostreams.IOStreams) (*slog.LevelVar, func() *slog.Logger) {
	level := &slog.LevelVar{} // defaults to Info

	handler := slog.NewJSONHandler(ios.ErrOut, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)
	return level, func() *slog.Logger { return logger }
}
