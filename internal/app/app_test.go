package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"testing/slogtest"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// ---------------------------------------------------------------------------
// classifyError
// ---------------------------------------------------------------------------

func TestClassifyError_SilentError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer

	// When
	code := classifyError(&stderr, cmdutil.ErrSilent, false)

	// Then
	if code != ExitError {
		t.Errorf("expected ExitError (%d), got %d", ExitError, code)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr output for silent error, got %q", stderr.String())
	}
}

func TestClassifyError_ContextCanceled_DefaultFalse_ReturnsExitOK(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer

	// When
	code := classifyError(&stderr, context.Canceled, false)

	// Then
	if code != ExitOK {
		t.Errorf("expected ExitOK (%d), got %d", ExitOK, code)
	}
}

func TestClassifyError_WrappedContextCanceled_DefaultFalse_ReturnsExitOK(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	wrapped := fmt.Errorf("operation failed: %w", context.Canceled)

	// When
	code := classifyError(&stderr, wrapped, false)

	// Then
	if code != ExitOK {
		t.Errorf("expected ExitOK (%d), got %d", ExitOK, code)
	}
}

func TestClassifyError_ContextCanceled_SignalCancelIsError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer

	// When
	code := classifyError(&stderr, context.Canceled, true)

	// Then
	if code != ExitError {
		t.Errorf("expected ExitError (%d), got %d", ExitError, code)
	}
}

func TestClassifyError_WrappedContextCanceled_SignalCancelIsError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	wrapped := fmt.Errorf("operation failed: %w", context.Canceled)

	// When
	code := classifyError(&stderr, wrapped, true)

	// Then
	if code != ExitError {
		t.Errorf("expected ExitError (%d), got %d", ExitError, code)
	}
}

func TestClassifyError_FlagError_ReturnsExitValidationAndPrintsMessage(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	flagErr := cmdutil.FlagErrorf("--count must be positive")

	// When
	code := classifyError(&stderr, flagErr, false)

	// Then
	if code != ExitValidation {
		t.Errorf("expected ExitValidation (%d), got %d", ExitValidation, code)
	}
	if !strings.Contains(stderr.String(), "--count must be positive") {
		t.Errorf("expected flag error message in stderr, got %q", stderr.String())
	}
}

func TestClassifyError_GenericError_ReturnsExitErrorAndPrintsMessage(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	genericErr := errors.New("something went wrong")

	// When
	code := classifyError(&stderr, genericErr, false)

	// Then
	if code != ExitError {
		t.Errorf("expected ExitError (%d), got %d", ExitError, code)
	}
	if !strings.Contains(stderr.String(), "something went wrong") {
		t.Errorf("expected error message in stderr, got %q", stderr.String())
	}
}

// ---------------------------------------------------------------------------
// classifyError — precedence: ErrSilent wrapping FlagError
// ---------------------------------------------------------------------------

func TestClassifyError_SilentWrappingFlagError_PrefersErrSilent(t *testing.T) {
	t.Parallel()

	// Given — an error chain where ErrSilent wraps a FlagError.
	// classifyError checks ErrSilent first, so it should win.
	var stderr bytes.Buffer
	flagErr := cmdutil.FlagErrorf("bad flag")
	combined := fmt.Errorf("%w: %w", cmdutil.ErrSilent, flagErr)

	// When
	code := classifyError(&stderr, combined, false)

	// Then
	if code != ExitError {
		t.Errorf("expected ExitError (%d) from ErrSilent precedence, got %d", ExitError, code)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr for silent error, got %q", stderr.String())
	}
}

// ---------------------------------------------------------------------------
// appNameFromArgs
// ---------------------------------------------------------------------------

func TestAppNameFromArgs(t *testing.T) {
	t.Parallel()

	// windowsPathWant reflects filepath.Base's intentional OS-aware behavior:
	// on Windows, backslash is a separator so the directory is stripped and
	// the result is "myapp"; on Unix, backslash is a valid filename character
	// so filepath.Base returns the whole string and only .exe is trimmed.
	windowsPathWant := `C:\bin\myapp`
	if runtime.GOOS == "windows" {
		windowsPathWant = "myapp"
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "UnixAbsolutePath_StripsDirComponent",
			args: []string{"/usr/local/bin/myapp"},
			want: "myapp",
		},
		{
			name: "WindowsPathWithExe_BehaviorIsOSDependent",
			args: []string{`C:\bin\myapp.exe`},
			want: windowsPathWant,
		},
		{
			name: "BareNameWithExeSuffix_StripsExe",
			args: []string{"myapp.exe"},
			want: "myapp",
		},
		{
			name: "BareName_NoDirectory_ReturnedAsIs",
			args: []string{"myapp"},
			want: "myapp",
		},
		{
			name: "EmptyArgs_ReturnsDefaultFallback",
			args: []string{},
			want: "app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := appNameFromArgs(tc.args)

			// Then
			if got != tc.want {
				t.Errorf("appNameFromArgs(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// newFactory
// ---------------------------------------------------------------------------
func TestNewFactory_ReturnsAppNameAndVersion(t *testing.T) {
	t.Parallel()

	// Given
	const appName = "example-app"
	const appVersion = "0.1.2"

	// When
	f := newFactory(appName, appVersion)
	actualName := f.AppName
	actualVer := f.AppVersion

	// Then
	if actualName != appName {
		t.Errorf("expected app name %q, got %q", appName, actualName)
	}
	if actualVer != appVersion {
		t.Errorf("expected app version %q, got %q", appVersion, actualVer)
	}
}

// ---------------------------------------------------------------------------
// newLogger — handler conformance via testing/slogtest
// ---------------------------------------------------------------------------

// TestNewLogger_NonTTY_JSONHandlerConformance verifies that the handler
// produced by newLogger when stderr is not a TTY passes the full slog.Handler
// contract — attributes, groups, WithAttrs, WithGroup, and level filtering
// all behave correctly with the wired-in LevelVar and writer.
func TestNewLogger_NonTTY_JSONHandlerConformance(t *testing.T) {
	t.Parallel()

	// Given — a shared buffer variable; newHandler writes to a fresh buffer
	// each subtest, and result reads back from the same buffer.
	var buf *bytes.Buffer

	slogtest.Run(t,
		func(t *testing.T) slog.Handler {
			ios, _, _, stderr := iostreams.Test()
			// Non-TTY is the default for iostreams.Test.
			_, loggerFn := newLogger(ios)
			buf = stderr
			return loggerFn().Handler()
		},
		func(t *testing.T) map[string]any {
			line := bytes.TrimSpace(buf.Bytes())
			if len(line) == 0 {
				return nil
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatalf("failed to parse JSON log output: %v\nraw: %s", err, line)
			}
			return m
		},
	)
}

// ---------------------------------------------------------------------------
// newLogger — LevelVar wiring and closure identity
// ---------------------------------------------------------------------------

func TestNewLogger_LevelVar_ControlsOutput(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, stderr := iostreams.Test()
	level, loggerFn := newLogger(ios)
	logger := loggerFn()

	// When — set level to Error, then log at Info
	level.Set(slog.LevelError)
	logger.Info("should be suppressed")

	// Then
	if stderr.Len() != 0 {
		t.Errorf("expected Info message to be suppressed at Error level, got %q", stderr.String())
	}
}

func TestNewLogger_LevelVar_DefaultsToInfo(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, stderr := iostreams.Test()
	_, loggerFn := newLogger(ios)
	logger := loggerFn()

	// When — log at Debug (below default Info level)
	logger.Debug("should be suppressed")

	// Then
	if stderr.Len() != 0 {
		t.Errorf("expected Debug message to be suppressed at default Info level, got %q", stderr.String())
	}
}

func TestNewLogger_ClosureReturnsSameInstance(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	_, loggerFn := newLogger(ios)

	// When
	a := loggerFn()
	b := loggerFn()

	// Then
	if a != b {
		t.Error("expected logger closure to return the same instance on every call")
	}
}
