package cmdutil_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// ---------------------------------------------------------------------------
// ClassifyError
// ---------------------------------------------------------------------------

func TestClassifyError_SilentError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer

	// When
	code := cmdutil.ClassifyError(&stderr, cmdutil.ErrSilent, false)

	// Then
	if code != cmdutil.ExitError {
		t.Errorf("expected ExitError (%d), got %d", cmdutil.ExitError, code)
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
	code := cmdutil.ClassifyError(&stderr, context.Canceled, false)

	// Then
	if code != cmdutil.ExitOK {
		t.Errorf("expected ExitOK (%d), got %d", cmdutil.ExitOK, code)
	}
}

func TestClassifyError_WrappedContextCanceled_DefaultFalse_ReturnsExitOK(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	wrapped := fmt.Errorf("operation failed: %w", context.Canceled)

	// When
	code := cmdutil.ClassifyError(&stderr, wrapped, false)

	// Then
	if code != cmdutil.ExitOK {
		t.Errorf("expected ExitOK (%d), got %d", cmdutil.ExitOK, code)
	}
}

func TestClassifyError_ContextCanceled_SignalCancelIsError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer

	// When
	code := cmdutil.ClassifyError(&stderr, context.Canceled, true)

	// Then
	if code != cmdutil.ExitError {
		t.Errorf("expected ExitError (%d), got %d", cmdutil.ExitError, code)
	}
}

func TestClassifyError_WrappedContextCanceled_SignalCancelIsError_ReturnsExitError(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	wrapped := fmt.Errorf("operation failed: %w", context.Canceled)

	// When
	code := cmdutil.ClassifyError(&stderr, wrapped, true)

	// Then
	if code != cmdutil.ExitError {
		t.Errorf("expected ExitError (%d), got %d", cmdutil.ExitError, code)
	}
}

func TestClassifyError_FlagError_ReturnsExitValidationAndPrintsMessage(t *testing.T) {
	t.Parallel()

	// Given
	var stderr bytes.Buffer
	flagErr := cmdutil.FlagErrorf("--count must be positive")

	// When
	code := cmdutil.ClassifyError(&stderr, flagErr, false)

	// Then
	if code != cmdutil.ExitValidation {
		t.Errorf("expected ExitValidation (%d), got %d", cmdutil.ExitValidation, code)
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
	code := cmdutil.ClassifyError(&stderr, genericErr, false)

	// Then
	if code != cmdutil.ExitError {
		t.Errorf("expected ExitError (%d), got %d", cmdutil.ExitError, code)
	}
	if !strings.Contains(stderr.String(), "something went wrong") {
		t.Errorf("expected error message in stderr, got %q", stderr.String())
	}
}

// ---------------------------------------------------------------------------
// ClassifyError — precedence: ErrSilent wrapping FlagError
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// ClassifyError — ExitCodeError: caller-chosen exit code, no message printed
// ---------------------------------------------------------------------------

// TestClassifyError_ExitCodeError_ReturnsSpecifiedCode verifies that an
// *ExitCodeError is mapped to its embedded Code without printing a message.
// This is the path used by np admin doctor (exit 1 for warnings, exit 2 for
// errors) where the command has already written all output.
func TestClassifyError_ExitCodeError_ReturnsSpecifiedCode(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		code cmdutil.ExitCode
	}{
		{cmdutil.ExitOK},      // 0 — unusual but must not panic
		{cmdutil.ExitError},   // 1
		{cmdutil.ExitCode(2)}, // 2 — doctor "errors present"
		{cmdutil.ExitCode(7)}, // arbitrary code
	} {
		var stderr bytes.Buffer
		err := &cmdutil.ExitCodeError{Code: tt.code}

		// When
		got := cmdutil.ClassifyError(&stderr, err, false)

		// Then
		if got != tt.code {
			t.Errorf("ExitCodeError{%d}: got %d, want %d", tt.code, got, tt.code)
		}
		if stderr.Len() != 0 {
			t.Errorf("ExitCodeError must not print a message; got %q", stderr.String())
		}
	}
}

// TestClassifyError_ExitCodeError_TakesPrecedenceOverErrSilent verifies that
// the ExitCodeError branch fires before the ErrSilent branch, so a code other
// than 1 (ExitError) can be signalled even when ErrSilent is in the chain.
func TestClassifyError_ExitCodeError_TakesPrecedenceOverErrSilent(t *testing.T) {
	t.Parallel()

	// Given — ExitCodeError{Code:2}
	var stderr bytes.Buffer
	err := &cmdutil.ExitCodeError{Code: cmdutil.ExitCode(2)}

	// When
	code := cmdutil.ClassifyError(&stderr, err, false)

	// Then — ExitCodeError wins; code is 2, not ExitError (1).
	if code != cmdutil.ExitCode(2) {
		t.Errorf("expected ExitCode 2, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// ClassifyError — precedence: ErrSilent wrapping FlagError
// ---------------------------------------------------------------------------

func TestClassifyError_SilentWrappingFlagError_PrefersErrSilent(t *testing.T) {
	t.Parallel()

	// Given — an error chain where ErrSilent wraps a FlagError.
	// ClassifyError checks ErrSilent first, so it should win.
	var stderr bytes.Buffer
	flagErr := cmdutil.FlagErrorf("bad flag")
	combined := fmt.Errorf("%w: %w", cmdutil.ErrSilent, flagErr)

	// When
	code := cmdutil.ClassifyError(&stderr, combined, false)

	// Then
	if code != cmdutil.ExitError {
		t.Errorf("expected ExitError (%d) from ErrSilent precedence, got %d", cmdutil.ExitError, code)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr for silent error, got %q", stderr.String())
	}
}
