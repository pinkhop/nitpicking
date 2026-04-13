package cmdutil_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// TestResolveLimit_NoLimitTrue_ReturnsUnlimited verifies that passing
// noLimit=true causes ResolveLimit to return -1 (unlimited), regardless of
// the limit flag value.
func TestResolveLimit_NoLimitTrue_ReturnsUnlimited(t *testing.T) {
	t.Parallel()

	// Given
	noLimit := true
	limitFlag := 10

	// When
	result, err := cmdutil.ResolveLimit(limitFlag, noLimit)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != -1 {
		t.Errorf("expected -1 (unlimited) when noLimit is true, got %d", result)
	}
}

// TestResolveLimit_ExplicitLimit_ReturnsExplicitValue verifies that an explicit
// positive limit flag value is returned as-is.
func TestResolveLimit_ExplicitLimit_ReturnsExplicitValue(t *testing.T) {
	t.Parallel()

	// Given
	noLimit := false
	limitFlag := 25

	// When
	result, err := cmdutil.ResolveLimit(limitFlag, noLimit)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 25 {
		t.Errorf("expected 25, got %d", result)
	}
}

// TestResolveLimit_NegativeLimit_ReturnsError verifies that a negative --limit
// flag value is rejected with ErrInvalidLimit.
func TestResolveLimit_NegativeLimit_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	noLimit := false
	limitFlag := -1

	// When
	_, err := cmdutil.ResolveLimit(limitFlag, noLimit)

	// Then
	if err == nil {
		t.Fatal("expected error for negative limit, got nil")
	}
	if !errors.Is(err, cmdutil.ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}
}

// TestResolveLimit_ZeroLimit_ReturnsError verifies that a zero --limit flag
// value is rejected with ErrInvalidLimit.
func TestResolveLimit_ZeroLimit_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	noLimit := false
	limitFlag := 0

	// When
	_, err := cmdutil.ResolveLimit(limitFlag, noLimit)

	// Then
	if err == nil {
		t.Fatal("expected error for zero limit, got nil")
	}
	if !errors.Is(err, cmdutil.ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}
}
