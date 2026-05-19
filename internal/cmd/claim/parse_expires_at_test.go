package claim

import (
	"strings"
	"testing"
	"time"
)

// TestParseExpiresAt_ValidFutureUTC_Succeeds asserts that a well-formed UTC
// RFC3339 timestamp within the 24h window is accepted and parsed.
func TestParseExpiresAt_ValidFutureUTC_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a UTC timestamp 3 hours in the future.
	future := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	raw := future.Format(time.RFC3339)

	// When
	got, err := parseExpiresAt(raw)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(future) {
		t.Errorf("parsed time: got %v, want %v", got, future)
	}
}

// TestParseExpiresAt_EmptyString_Fails asserts that an empty value is rejected
// with a UTC-related message rather than a confusing parse error.
func TestParseExpiresAt_EmptyString_Fails(t *testing.T) {
	t.Parallel()

	// Given — empty string

	// When
	_, err := parseExpiresAt("")

	// Then — error mentions UTC since the empty value cannot be a UTC suffix.
	if err == nil {
		t.Fatal("expected error for empty value, got nil")
	}
	if !strings.Contains(err.Error(), "UTC") {
		t.Errorf("error should mention UTC, got: %v", err)
	}
}

// TestParseExpiresAt_NonUTCOffset_Fails asserts that timestamps with non-Z
// offsets are rejected so the wire format remains canonical UTC.
func TestParseExpiresAt_NonUTCOffset_Fails(t *testing.T) {
	t.Parallel()

	// Given — a valid RFC3339 timestamp with a +05:00 offset.
	raw := "2099-01-01T00:00:00+05:00"

	// When
	_, err := parseExpiresAt(raw)

	// Then — error mentions UTC and the "Z" suffix.
	if err == nil {
		t.Fatal("expected error for non-UTC offset, got nil")
	}
	if !strings.Contains(err.Error(), "UTC") {
		t.Errorf("error should mention UTC, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Z") {
		t.Errorf("error should mention Z suffix, got: %v", err)
	}
}

// TestParseExpiresAt_InvalidFormat_Fails asserts that values that are not
// RFC3339 timestamps return a parse error after the UTC-suffix check passes.
func TestParseExpiresAt_InvalidFormat_Fails(t *testing.T) {
	t.Parallel()

	// Given — a string that ends in 'Z' but is not a valid timestamp.
	raw := "not-a-timestampZ"

	// When
	_, err := parseExpiresAt(raw)

	// Then
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "RFC3339") {
		t.Errorf("error should mention RFC3339, got: %v", err)
	}
}

// TestParseExpiresAt_InThePast_Fails asserts that past timestamps are rejected
// because an expiry that has already occurred is never useful.
func TestParseExpiresAt_InThePast_Fails(t *testing.T) {
	t.Parallel()

	// Given — a timestamp in the year 2020.
	raw := "2020-01-01T00:00:00Z"

	// When
	_, err := parseExpiresAt(raw)

	// Then — error mentions "future".
	if err == nil {
		t.Fatal("expected error for past timestamp, got nil")
	}
	if !strings.Contains(err.Error(), "future") {
		t.Errorf("error should mention 'future', got: %v", err)
	}
}

// TestParseExpiresAt_BeyondTwentyFourHours_Fails asserts the 24h cap is
// enforced so callers cannot lock issues for arbitrarily long.
func TestParseExpiresAt_BeyondTwentyFourHours_Fails(t *testing.T) {
	t.Parallel()

	// Given — a timestamp well beyond 24h from now (year 2099).
	raw := "2099-01-01T00:00:00Z"

	// When
	_, err := parseExpiresAt(raw)

	// Then — error mentions 24h.
	if err == nil {
		t.Fatal("expected error for too-distant timestamp, got nil")
	}
	if !strings.Contains(err.Error(), "24h") {
		t.Errorf("error should mention 24h, got: %v", err)
	}
}
