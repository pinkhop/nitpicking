package root

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/gops/agent"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// ---------------------------------------------------------------------------
// setLogLevel
// ---------------------------------------------------------------------------

func TestSetLogLevel_ValidLevels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected slog.Level
	}{
		{"debug lowercase", "debug", slog.LevelDebug},
		{"debug mixed case", "Debug", slog.LevelDebug},
		{"info lowercase", "info", slog.LevelInfo},
		{"info uppercase", "INFO", slog.LevelInfo},
		{"warn lowercase", "warn", slog.LevelWarn},
		{"error lowercase", "error", slog.LevelError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			lv := &slog.LevelVar{}

			// When
			err := setLogLevel(lv, tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := lv.Level(); got != tc.expected {
				t.Errorf("expected level %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestSetLogLevel_InvalidLevel_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given
	lv := &slog.LevelVar{}

	// When
	err := setLogLevel(lv, "verbose")

	// Then
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if _, ok := err.(*cmdutil.FlagError); !ok {
		t.Errorf("expected *cmdutil.FlagError, got %T", err)
	}
}

func TestSetLogLevel_NilLevelVar_IsNoOp(t *testing.T) {
	t.Parallel()

	// When
	err := setLogLevel(nil, "debug")
	// Then
	if err != nil {
		t.Fatalf("expected nil error for nil LevelVar, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// WithLogger / LoggerFrom
// ---------------------------------------------------------------------------

func TestLoggerRoundTrip_StoredLoggerIsRetrieved(t *testing.T) {
	t.Parallel()

	// Given
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	// When
	ctx = WithLogger(ctx, logger)
	got := LoggerFrom(ctx)

	// Then
	if got != logger {
		t.Error("expected LoggerFrom to return the logger stored by WithLogger")
	}
}

func TestLoggerFrom_NoLogger_ReturnsSlogDefault(t *testing.T) {
	t.Parallel()

	// Given — a bare context with no logger stored
	ctx := context.Background()

	// When
	got := LoggerFrom(ctx)

	// Then
	if got != slog.Default() {
		t.Error("expected LoggerFrom to return slog.Default() when no logger is stored")
	}
}

// ---------------------------------------------------------------------------
// NewRootCmd — Before hook behavior
// ---------------------------------------------------------------------------

func newTestFactory() *cmdutil.Factory {
	ios, _, _, _ := iostreams.Test()
	level := &slog.LevelVar{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	return &cmdutil.Factory{
		AppName:    "app-name",
		AppVersion: "test",
		IOStreams:  ios,
		LogLevel:   level,
		Logger:     func() *slog.Logger { return logger },
	}
}

// ---------------------------------------------------------------------------
// NewRootCmd — gops agent behavior
// ---------------------------------------------------------------------------

// TestNewRootCmd_NoGopsFlag_AgentNotStarted verifies that passing --no-gops
// prevents the gops agent from starting, leaving the agent slot free for a
// subsequent Listen call in the same test process.
func TestNewRootCmd_NoGopsFlag_AgentNotStarted(t *testing.T) {
	t.Parallel()

	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When
	err := cmd.Run(context.Background(), []string{"np", "--no-gops", "version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The agent was never started, so a fresh Listen call must succeed.
	// If the agent were already running, Listen would return an error.
	if listenErr := agent.Listen(agent.Options{}); listenErr != nil {
		t.Errorf("expected gops agent to not be running after --no-gops, got: %v", listenErr)
	}
	t.Cleanup(agent.Close)
}

// TestNewRootCmd_NoGopsEnvVar_AgentNotStarted verifies that NO_GOPS=1 has the
// same effect as --no-gops via urfave/cli's flag-layering mechanism.
// Not parallel: t.Setenv and t.Parallel cannot be combined.
func TestNewRootCmd_NoGopsEnvVar_AgentNotStarted(t *testing.T) {
	t.Setenv("NO_GOPS", "1")

	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When
	err := cmd.Run(context.Background(), []string{"np", "version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listenErr := agent.Listen(agent.Options{}); listenErr != nil {
		t.Errorf("expected gops agent to not be running after NO_GOPS=1, got: %v", listenErr)
	}
	t.Cleanup(agent.Close)
}

// TestNewRootCmd_GopsAgent_LifecycleStartsAndCleans verifies that the gops
// agent is started by the Before hook and cleaned up by the After hook when
// --no-gops is absent. After cmd.Run returns, agent.Listen must succeed again,
// confirming the full start-and-close lifecycle executed correctly.
//
// Not parallel: agent.Listen is process-wide state. Running this concurrently
// with other tests that call Listen would cause non-deterministic failures.
// Any new root command test that exercises the full Before+After path without
// --no-gops must also skip t.Parallel() for the same reason.
func TestNewRootCmd_GopsAgent_LifecycleStartsAndCleans(t *testing.T) {
	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When — Before starts the agent, After closes it
	err := cmd.Run(context.Background(), []string{"np", "version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After hook called agent.Close(), so a fresh Listen must succeed.
	if listenErr := agent.Listen(agent.Options{}); listenErr != nil {
		t.Errorf("expected gops agent to be closed after After hook ran, but Listen returned: %v", listenErr)
	}
	t.Cleanup(agent.Close)
}

// ---------------------------------------------------------------------------
// NewRootCmd — Before hook behavior
// ---------------------------------------------------------------------------

func TestNewRootCmd_BeforeHook_SetsLogLevel(t *testing.T) {
	t.Parallel()

	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When — run with --log-level=debug and "version" subcommand.
	// --no-gops prevents the process-wide gops agent from starting, keeping
	// this test isolated from other parallel tests that also call Run.
	err := cmd.Run(context.Background(), []string{"np", "--no-gops", "--log-level", "debug", "version"})
	// Then — the Before hook should have set the level to Debug
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := f.LogLevel.Level(); got != slog.LevelDebug {
		t.Errorf("expected LevelDebug, got %v", got)
	}
}
