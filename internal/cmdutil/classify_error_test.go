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
