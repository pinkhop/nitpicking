package ticket_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestParseID_ValidID_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	raw := "NP-a3bxr"

	// When
	id, err := ticket.ParseID(raw)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "NP-a3bxr" {
		t.Errorf("expected NP-a3bxr, got %s", id.String())
	}
	if id.Prefix() != "NP" {
		t.Errorf("expected prefix NP, got %s", id.Prefix())
	}
	if id.Random() != "a3bxr" {
		t.Errorf("expected random a3bxr, got %s", id.Random())
	}
}

func TestParseID_MissingSeparator_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := ticket.ParseID("NPa3bxr")

	// Then
	if err == nil {
		t.Fatal("expected error for missing separator")
	}
}

func TestParseID_InvalidPrefix_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"lowercase prefix", "np-a3bxr"},
		{"prefix too long", "ABCDEFGHIJK-a3bxr"},
		{"empty prefix", "-a3bxr"},
		{"digits in prefix", "N1-a3bxr"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.ParseID(tc.input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", tc.input)
			}
		})
	}
}

func TestParseID_InvalidRandom_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"too short", "NP-a3bx"},
		{"too long", "NP-a3bxrr"},
		{"contains i", "NP-i3bxr"},
		{"contains l", "NP-l3bxr"},
		{"contains o", "NP-o3bxr"},
		{"contains u", "NP-u3bxr"},
		{"uppercase", "NP-A3BXR"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := ticket.ParseID(tc.input)

			// Then
			if err == nil {
				t.Errorf("expected error for %q", tc.input)
			}
		})
	}
}

func TestGenerateID_ProducesValidID(t *testing.T) {
	t.Parallel()

	// When
	id, err := ticket.GenerateID("NP", nil)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(id.String(), "NP-") {
		t.Errorf("expected NP- prefix, got %s", id.String())
	}
	if len(id.Random()) != 5 {
		t.Errorf("expected 5-char random, got %d", len(id.Random()))
	}

	// Verify round-trip
	parsed, err := ticket.ParseID(id.String())
	if err != nil {
		t.Fatalf("generated ID failed to parse: %v", err)
	}
	if parsed.String() != id.String() {
		t.Errorf("round-trip mismatch: %s != %s", parsed, id)
	}
}

func TestGenerateID_InvalidPrefix_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := ticket.GenerateID("np", nil)

	// Then
	if err == nil {
		t.Fatal("expected error for lowercase prefix")
	}
}

func TestGenerateID_CollisionRetry_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — first two calls report collision, third succeeds
	callCount := 0
	collisionCheck := func(_ ticket.ID) (bool, error) {
		callCount++
		return callCount <= 2, nil
	}

	// When
	id, err := ticket.GenerateID("NP", collisionCheck)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.IsZero() {
		t.Error("expected non-zero ID")
	}
	if callCount != 3 {
		t.Errorf("expected 3 collision checks, got %d", callCount)
	}
}

func TestGenerateID_CollisionCheckError_PropagatesError(t *testing.T) {
	t.Parallel()

	// Given
	checkErr := errors.New("db unavailable")
	collisionCheck := func(_ ticket.ID) (bool, error) {
		return false, checkErr
	}

	// When
	_, err := ticket.GenerateID("NP", collisionCheck)

	// Then
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, checkErr) {
		t.Errorf("expected wrapped db error, got %v", err)
	}
}

func TestGenerateID_AllCollisions_Fails(t *testing.T) {
	t.Parallel()

	// Given — every check reports collision
	collisionCheck := func(_ ticket.ID) (bool, error) {
		return true, nil
	}

	// When
	_, err := ticket.GenerateID("NP", collisionCheck)

	// Then
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestID_IsZero_DetectsZeroValue(t *testing.T) {
	t.Parallel()

	// Given
	var zero ticket.ID

	// Then
	if !zero.IsZero() {
		t.Error("expected zero value to report IsZero")
	}
}

func TestResolveID_FullID_ReturnsParsedID(t *testing.T) {
	t.Parallel()

	// When
	id, err := ticket.ResolveID("NP-a3bxr", "NP")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "NP-a3bxr" {
		t.Errorf("expected NP-a3bxr, got %s", id.String())
	}
}

func TestResolveID_BareRandom_PrependsPrefix(t *testing.T) {
	t.Parallel()

	// When
	id, err := ticket.ResolveID("a3bxr", "NP")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "NP-a3bxr" {
		t.Errorf("expected NP-a3bxr, got %s", id.String())
	}
}

func TestResolveID_InvalidBareRandom_ReturnsError(t *testing.T) {
	t.Parallel()

	// When: a string that isn't a valid full ID or a valid random part.
	_, err := ticket.ResolveID("not-valid", "NP")

	// Then
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestResolveID_BareRandomWithExcludedChars_ReturnsError(t *testing.T) {
	t.Parallel()

	// When: bare random with excluded Crockford chars (i, l, o, u).
	_, err := ticket.ResolveID("il0ou", "NP")

	// Then
	if err == nil {
		t.Fatal("expected error for invalid Crockford chars")
	}
}

func TestValidatePrefix_ValidPrefixes(t *testing.T) {
	t.Parallel()

	cases := []string{"A", "NP", "ABCDEFGHIJ"}
	for _, prefix := range cases {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()

			// When
			err := ticket.ValidatePrefix(prefix)
			// Then
			if err != nil {
				t.Errorf("expected valid prefix %q, got error: %v", prefix, err)
			}
		})
	}
}
