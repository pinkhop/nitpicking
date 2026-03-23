package root

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/gops/agent"
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/version"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// setLogLevel parses a log level name and applies it to the given LevelVar.
// Valid names are "debug", "info", "warn", and "error" (case-insensitive).
// If lv is nil (e.g., in tests that replace the Logger closure and don't
// provide a LevelVar), the call is a no-op.
func setLogLevel(lv *slog.LevelVar, name string) error {
	if lv == nil {
		return nil
	}
	switch strings.ToLower(name) {
	case "debug":
		lv.Set(slog.LevelDebug)
	case "info":
		lv.Set(slog.LevelInfo)
	case "warn":
		lv.Set(slog.LevelWarn)
	case "error":
		lv.Set(slog.LevelError)
	default:
		// Defense-in-depth: the --log-level flag's Validator currently rejects
		// unknown values at parse time, before the Before hook calls this
		// function. This branch guards against future callers that bypass flag
		// parsing and against refactorings that change or remove the Validator
		// without realizing this function silently depends on it — exactly the
		// kind of quiet upstream change that causes mysterious downstream failures.
		return cmdutil.FlagErrorf("unknown log level: %s", name)
	}
	return nil
}

// loggerKey is an unexported type used as a context key for the logger.
// Using a struct type (rather than a string) prevents collisions with keys
// from other packages — no other package can create a value of this type.
type loggerKey struct{}

// WithLogger returns a new context carrying the given logger.
// The root command's Before hook uses this to make the logger available
// to subcommands via the context, enabling command-specific logging
// without passing the Factory through every function signature.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFrom extracts the logger from the context. If no logger was stored
// (e.g., in tests that skip the Before hook), it returns the default slog logger
// as a safe fallback.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// NewRootCmd constructs the root command with all subcommands registered.
// The Factory is passed to each subcommand constructor, allowing them to
// extract only the dependencies they need. The Before hook runs before
// every subcommand and enriches the context with cross-cutting concerns;
// the After hook tears down infrastructure started in Before.
func NewRootCmd(f *cmdutil.Factory) *cli.Command {
	var env, logLevel string
	var noGops bool

	// gopsStarted tracks whether the gops agent was successfully started in
	// Before, so After knows whether to call agent.Close(). This parallels
	// the rootSpan pattern used in the OpenTelemetry integration.
	var gopsStarted bool

	// stopSignals deregisters the signal handler installed in Before.
	// Declared here so the After hook can call it during teardown.
	var stopSignals func()

	return &cli.Command{
		Name:  "np",
		Usage: "A local-only, CLI-driven issue tracker for AI agent workflows",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "env",
				Sources:     cli.EnvVars("ENV", "ENVIRONMENT"),
				Usage:       "Set the deployment environment; e.g., dev, staging, prod",
				Value:       "dev",
				Destination: &env,
				Validator: func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("environment variable is empty")
					}
					return nil
				},
			},
			&cli.StringFlag{
				Name:        "log-level",
				Sources:     cli.EnvVars("LOG_LEVEL"),
				Usage:       "Set the logging level: debug, info, warn, error",
				Value:       "info",
				Destination: &logLevel,
				Validator: func(s string) error {
					switch strings.ToLower(s) {
					case "debug", "info", "warn", "error":
						return nil
					default:
						return fmt.Errorf("unknown log level: %s", s)
					}
				},
			},
			&cli.BoolFlag{
				Name:        "no-gops",
				Sources:     cli.EnvVars("NO_GOPS"),
				Usage:       "Disable the gops diagnostics agent",
				Value:       false,
				Destination: &noGops,
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// The Before hook on the root command runs before EVERY subcommand.
			// Use it for cross-cutting concerns:
			//   - Global flag processing (log level, environment)
			//   - Context enrichment (logger, trace ID)
			//   - Authentication checks

			// Apply the parsed --log-level flag to the factory's LevelVar.
			// Because the production logger's handler references this LevelVar,
			// the change takes effect immediately for all subsequent log calls.
			if err := setLogLevel(f.LogLevel, logLevel); err != nil {
				return ctx, err
			}

			logger := f.Logger()
			ctx = WithLogger(ctx, logger)

			// Signal handling wraps the context so SIGINT/SIGTERM trigger
			// cancellation for every subcommand. This is a process-wide
			// concern — multiple NotifyContext calls on the same signals
			// interfere with each other — so it belongs here alongside
			// other process-wide lifecycle management (gops, telemetry).
			// Tests that call run() directly bypass Before entirely and
			// pass their own cancellable context.
			ctx, stopSignals = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

			// Start the gops diagnostics agent unless explicitly disabled.
			// The agent listens on a temp socket (no address configuration
			// needed) and enables runtime inspection via the `gops` CLI tool.
			// A startup failure is non-fatal — log a warning and continue,
			// because diagnostics must not block the application from running.
			if !noGops {
				if err := agent.Listen(agent.Options{}); err != nil {
					LoggerFrom(ctx).Warn("gops agent failed to start", "error", err)
				} else {
					gopsStarted = true
				}
			}

			return ctx, nil
		},
		After: func(ctx context.Context, cmd *cli.Command) error {
			// Stop the gops diagnostics agent if it was started in Before.
			if gopsStarted {
				agent.Close()
			}
			// Deregister the signal handler installed in Before.
			if stopSignals != nil {
				stopSignals()
			}
			return nil
		},
		Commands: []*cli.Command{
			version.NewCmd(f),
		},
	}
}
