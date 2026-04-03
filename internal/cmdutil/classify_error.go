package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// ClassifyError maps a command error to an exit code and prints appropriate
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
func ClassifyError(stderr io.Writer, err error, signalCancelIsError bool) ExitCode {
	switch {
	case errors.Is(err, ErrSilent):
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

		if fe, ok := errors.AsType[*FlagError](err); ok {
			_, _ = fmt.Fprintln(stderr, fe) // #nosec G705 -- CLI stderr output, not rendered in a browser
			return ExitValidation
		}

		_, _ = fmt.Fprintln(stderr, err)
		return ExitError
	}
}
