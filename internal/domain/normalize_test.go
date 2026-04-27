package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestNormalizeCrockford_LowercasesUppercaseLetters(t *testing.T) {
	t.Parallel()

	// When
	result := domain.NormalizeCrockford("ABCDE")

	// Then
	if result != "abcde" {
		t.Errorf("expected %q, got %q", "abcde", result)
	}
}

func TestNormalizeCrockford_SubstitutesI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"uppercase I", "I"},
		{"lowercase i", "i"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			result := domain.NormalizeCrockford(tc.input)

			// Then — I/i maps to 1
			if result != "1" {
				t.Errorf("expected %q, got %q", "1", result)
			}
		})
	}
}

func TestNormalizeCrockford_SubstitutesL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"uppercase L", "L"},
		{"lowercase l", "l"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			result := domain.NormalizeCrockford(tc.input)

			// Then — L/l maps to 1
			if result != "1" {
				t.Errorf("expected %q, got %q", "1", result)
			}
		})
	}
}

func TestNormalizeCrockford_SubstitutesO(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"uppercase O", "O"},
		{"lowercase o", "o"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			result := domain.NormalizeCrockford(tc.input)

			// Then — O/o maps to 0
			if result != "0" {
				t.Errorf("expected %q, got %q", "0", result)
			}
		})
	}
}

func TestNormalizeCrockford_MixedInput(t *testing.T) {
	t.Parallel()

	// Given — a string with uppercase, confusable chars, and digits
	input := "LOIoA"

	// When
	result := domain.NormalizeCrockford(input)

	// Then — L→1, O→0, I→1, o→0, A→a
	if result != "1010a" {
		t.Errorf("expected %q, got %q", "1010a", result)
	}
}

func TestNormalizeCrockford_DigitsUnchanged(t *testing.T) {
	t.Parallel()

	// When
	result := domain.NormalizeCrockford("01234")

	// Then
	if result != "01234" {
		t.Errorf("expected %q, got %q", "01234", result)
	}
}

func TestNormalizeCrockford_AlreadyCanonical(t *testing.T) {
	t.Parallel()

	// When — input is already lowercase Crockford
	result := domain.NormalizeCrockford("a3bxr")

	// Then — unchanged
	if result != "a3bxr" {
		t.Errorf("expected %q, got %q", "a3bxr", result)
	}
}

func TestParseID_UppercaseRandom_NormalizesToLowercase(t *testing.T) {
	t.Parallel()

	// When
	id, err := domain.ParseID("FOO-A3BXR")
	// Then — succeeds and normalizes to lowercase
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "FOO-a3bxr" {
		t.Errorf("expected %q, got %q", "FOO-a3bxr", id.String())
	}
}

func TestParseID_ConfusableI_NormalizesTo1(t *testing.T) {
	t.Parallel()

	// When — 'i' in random portion normalizes to '1'
	id, err := domain.ParseID("FOO-i3bxr")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Random() != "13bxr" {
		t.Errorf("expected random %q, got %q", "13bxr", id.Random())
	}
}

func TestParseID_ConfusableL_NormalizesTo1(t *testing.T) {
	t.Parallel()

	// When — 'l' in random portion normalizes to '1'
	id, err := domain.ParseID("FOO-l3bxr")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Random() != "13bxr" {
		t.Errorf("expected random %q, got %q", "13bxr", id.Random())
	}
}

func TestParseID_ConfusableO_NormalizesTo0(t *testing.T) {
	t.Parallel()

	// When — 'o' in random portion normalizes to '0'
	id, err := domain.ParseID("FOO-o3bxr")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Random() != "03bxr" {
		t.Errorf("expected random %q, got %q", "03bxr", id.Random())
	}
}

func TestParseID_MixedConfusables_NormalizesAll(t *testing.T) {
	t.Parallel()

	// When — multiple confusable chars: L→1, O→0, I→1, o→0, A→a
	id, err := domain.ParseID("FOO-LOIoA")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "FOO-1010a" {
		t.Errorf("expected %q, got %q", "FOO-1010a", id.String())
	}
}

func TestParseID_ExcludedU_StillFails(t *testing.T) {
	t.Parallel()

	// When — 'u' is excluded from Crockford and has no substitution
	_, err := domain.ParseID("FOO-u3bxr")

	// Then
	if err == nil {
		t.Fatal("expected error for excluded character 'u'")
	}
}

func TestResolveID_ConfusableBarePart_Normalizes(t *testing.T) {
	t.Parallel()

	// When — bare random part with confusable characters
	id, err := domain.ResolveID("LOIoA", "FOO")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "FOO-1010a" {
		t.Errorf("expected %q, got %q", "FOO-1010a", id.String())
	}
}
