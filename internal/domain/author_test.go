package domain_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestNewAuthor_ValidAuthor_Succeeds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"simple", "alice"},
		{"with digits", "agent-7"},
		{"unicode", "böb"},
		{"single char", "a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			author, err := domain.NewAuthor(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if author.IsZero() {
				t.Error("expected non-zero author")
			}
		})
	}
}

func TestNewAuthor_NFCNormalization(t *testing.T) {
	t.Parallel()

	// Given — e + combining acute accent (NFD form)
	nfd := "e\u0301"

	// When
	author, err := domain.NewAuthor(nfd)
	// Then — should normalize to NFC (single codepoint é)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if author.String() != "\u00e9" {
		t.Errorf("expected NFC normalized é, got %q", author.String())
	}
}

func TestNewAuthor_ContainsWhitespace_Fails(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"space", "alice bob"},
		{"tab", "alice\tbob"},
		{"newline", "alice\nbob"},
		{"nbsp", "alice\u00a0bob"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.NewAuthor(tc.input)

			// Then
			if err == nil {
				t.Error("expected error for whitespace in author")
			}
		})
	}
}

func TestNewAuthor_NoAlphanumeric_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewAuthor("---")

	// Then
	if err == nil {
		t.Error("expected error for no alphanumeric characters")
	}
}

func TestNewAuthor_TooLong_Fails(t *testing.T) {
	t.Parallel()

	// Given — 65 runes
	long := strings.Repeat("a", 65)

	// When
	_, err := domain.NewAuthor(long)

	// Then
	if err == nil {
		t.Error("expected error for author exceeding 64 runes")
	}
}

func TestNewAuthor_ExactlyMaxLength_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — exactly 64 runes
	exact := strings.Repeat("a", 64)

	// When
	author, err := domain.NewAuthor(exact)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if author.IsZero() {
		t.Error("expected non-zero author")
	}
}

func TestNewAuthor_Empty_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewAuthor("")

	// Then
	if err == nil {
		t.Error("expected error for empty author")
	}
}

func TestAuthor_CaseSensitiveEquality(t *testing.T) {
	t.Parallel()

	// Given
	alice, _ := domain.NewAuthor("alice")
	Alice, _ := domain.NewAuthor("Alice")

	// Then
	if alice.Equal(Alice) {
		t.Error("expected alice and Alice to be distinct")
	}
}

func TestAuthor_EqualAuthorsMatch(t *testing.T) {
	t.Parallel()

	// Given
	a1, _ := domain.NewAuthor("agent-7")
	a2, _ := domain.NewAuthor("agent-7")

	// Then
	if !a1.Equal(a2) {
		t.Error("expected identical authors to match")
	}
}
