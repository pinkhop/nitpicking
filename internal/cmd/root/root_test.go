package root

import (
	"context"
	"io"
	"log/slog"
	"testing"

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

func TestNewRootCmd_BeforeHook_SetsLogLevel(t *testing.T) {
	t.Parallel()

	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When
	err := cmd.Run(context.Background(), []string{"np", "--log-level", "debug", "version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := f.LogLevel.Level(); got != slog.LevelDebug {
		t.Errorf("expected LevelDebug, got %v", got)
	}
}
