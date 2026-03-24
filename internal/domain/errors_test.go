package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestValidationError_SingleField_IncludesFieldAndReason(t *testing.T) {
	t.Parallel()

	// Given
	err := domain.NewValidationError("title", "must contain at least one alphanumeric character")

	// When
	msg := err.Error()

	// Then
	if !strings.Contains(msg, "title") {
		t.Errorf("expected error to mention field name, got %q", msg)
	}
	if !strings.Contains(msg, "must contain at least one alphanumeric character") {
		t.Errorf("expected error to mention reason, got %q", msg)
	}
}

func TestValidationError_MultipleFields_ReportsCount(t *testing.T) {
	t.Parallel()

	// Given
	err := domain.NewMultiValidationError(map[string]string{
		"title":  "required",
		"author": "too long",
	})

	// When
	msg := err.Error()

	// Then
	if !strings.Contains(msg, "2 fields invalid") {
		t.Errorf("expected error to report field count, got %q", msg)
	}
}

func TestValidationError_Is_MatchesValidationError(t *testing.T) {
	t.Parallel()

	// Given
	err := domain.NewValidationError("title", "required")
	wrapped := fmt.Errorf("outer: %w", err)

	// When
	matches := errors.Is(wrapped, &domain.ValidationError{})

	// Then
	if !matches {
		t.Error("expected errors.Is to match *ValidationError")
	}
}

func TestValidationError_AsType_ExtractsFields(t *testing.T) {
	t.Parallel()

	// Given
	err := domain.NewValidationError("priority", "invalid value")
	wrapped := fmt.Errorf("create failed: %w", err)

	// When
	ve, ok := errors.AsType[*domain.ValidationError](wrapped)

	// Then
	if !ok {
		t.Fatal("expected errors.AsType to succeed")
	}
	if ve.Fields["priority"] != "invalid value" {
		t.Errorf("expected priority field reason, got %v", ve.Fields)
	}
}

func TestClaimConflictError_IncludesStructuredContext(t *testing.T) {
	t.Parallel()

	// Given
	staleAt := time.Date(2026, 3, 23, 14, 0, 0, 0, time.UTC)
	err := &domain.ClaimConflictError{
		IssueID:       "NP-abc12",
		CurrentHolder: "alice",
		StaleAt:       staleAt,
	}

	// When
	msg := err.Error()

	// Then
	if !strings.Contains(msg, "NP-abc12") {
		t.Errorf("expected issue ID in message, got %q", msg)
	}
	if !strings.Contains(msg, "alice") {
		t.Errorf("expected current holder in message, got %q", msg)
	}
	if !strings.Contains(msg, "2026-03-23") {
		t.Errorf("expected stale-at date in message, got %q", msg)
	}
}

func TestClaimConflictError_Is_MatchesClaimConflictError(t *testing.T) {
	t.Parallel()

	// Given
	err := &domain.ClaimConflictError{
		IssueID:       "NP-abc12",
		CurrentHolder: "bob",
		StaleAt:       time.Now(),
	}
	wrapped := fmt.Errorf("claim: %w", err)

	// When
	matches := errors.Is(wrapped, &domain.ClaimConflictError{})

	// Then
	if !matches {
		t.Error("expected errors.Is to match *ClaimConflictError")
	}
}

func TestClaimConflictError_AsType_ExtractsContext(t *testing.T) {
	t.Parallel()

	// Given
	staleAt := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	err := &domain.ClaimConflictError{
		IssueID:       "NP-xyz99",
		CurrentHolder: "agent-7",
		StaleAt:       staleAt,
	}
	wrapped := fmt.Errorf("operation failed: %w", err)

	// When
	ce, ok := errors.AsType[*domain.ClaimConflictError](wrapped)

	// Then
	if !ok {
		t.Fatal("expected errors.AsType to succeed")
	}
	if ce.IssueID != "NP-xyz99" {
		t.Errorf("expected IssueID NP-xyz99, got %s", ce.IssueID)
	}
	if ce.CurrentHolder != "agent-7" {
		t.Errorf("expected CurrentHolder agent-7, got %s", ce.CurrentHolder)
	}
	if !ce.StaleAt.Equal(staleAt) {
		t.Errorf("expected StaleAt %v, got %v", staleAt, ce.StaleAt)
	}
}

func TestDatabaseError_WrapsUnderlyingError(t *testing.T) {
	t.Parallel()

	// Given
	underlying := errors.New("disk full")
	err := &domain.DatabaseError{Op: "create issue", Err: underlying}

	// When
	msg := err.Error()
	unwrapped := errors.Unwrap(err)

	// Then
	if !strings.Contains(msg, "create issue") {
		t.Errorf("expected op in message, got %q", msg)
	}
	if !strings.Contains(msg, "disk full") {
		t.Errorf("expected underlying error in message, got %q", msg)
	}
	if unwrapped != underlying {
		t.Errorf("expected Unwrap to return underlying error")
	}
}

func TestDatabaseError_Is_MatchesDatabaseError(t *testing.T) {
	t.Parallel()

	// Given
	err := &domain.DatabaseError{Op: "query", Err: errors.New("locked")}
	wrapped := fmt.Errorf("service: %w", err)

	// When
	matches := errors.Is(wrapped, &domain.DatabaseError{})

	// Then
	if !matches {
		t.Error("expected errors.Is to match *DatabaseError")
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	t.Parallel()

	// Given
	sentinels := []error{
		domain.ErrNotFound,
		domain.ErrIllegalTransition,
		domain.ErrCycleDetected,
		domain.ErrDeletedIssue,
		domain.ErrTerminalState,
	}

	// Then — each sentinel is distinct from every other
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("expected %v and %v to be distinct", a, b)
			}
		}
	}
}

func TestSentinelErrors_MatchWhenWrapped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		sentinel error
	}{
		{"ErrNotFound", domain.ErrNotFound},
		{"ErrIllegalTransition", domain.ErrIllegalTransition},
		{"ErrCycleDetected", domain.ErrCycleDetected},
		{"ErrDeletedIssue", domain.ErrDeletedIssue},
		{"ErrTerminalState", domain.ErrTerminalState},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			wrapped := fmt.Errorf("context: %w", tc.sentinel)

			// When
			matches := errors.Is(wrapped, tc.sentinel)

			// Then
			if !matches {
				t.Errorf("expected wrapped %s to match via errors.Is", tc.name)
			}
		})
	}
}
