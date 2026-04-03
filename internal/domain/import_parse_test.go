package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestParse_ValidJSONL_ReturnsLines(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"idempotency_key":"k1","role":"task","title":"First"}
{"idempotency_key":"k2","role":"epic","title":"Second"}`

	// When
	lines, err := domain.Parse(strings.NewReader(input))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].IdempotencyKey != "k1" {
		t.Errorf("line[0].IdempotencyKey: got %q, want %q", lines[0].IdempotencyKey, "k1")
	}
	if lines[1].Role != "epic" {
		t.Errorf("line[1].Role: got %q, want %q", lines[1].Role, "epic")
	}
}

func TestParse_BlankLines_AreSkipped(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"idempotency_key":"k1","role":"task","title":"First"}

{"idempotency_key":"k2","role":"task","title":"Second"}`

	// When
	lines, err := domain.Parse(strings.NewReader(input))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (blank skipped), got %d", len(lines))
	}
}

func TestParse_InvalidJSON_ReturnsParseError(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"idempotency_key":"k1","role":"task","title":"Valid"}
not valid json`

	// When
	_, err := domain.Parse(strings.NewReader(input))

	// Then
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	var parseErr domain.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %T: %v", err, err)
	}
	if parseErr.Line != 2 {
		t.Errorf("expected error on line 2, got line %d", parseErr.Line)
	}
}

func TestParse_EmptyInput_ReturnsNilSlice(t *testing.T) {
	t.Parallel()

	// When
	lines, err := domain.Parse(strings.NewReader(""))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines != nil {
		t.Errorf("expected nil slice for empty input, got %v", lines)
	}
}

func TestParse_ExtraFields_AreIgnored(t *testing.T) {
	t.Parallel()

	// Given — line contains a field not in RawLine.
	input := `{"idempotency_key":"k1","role":"task","title":"Task","unknown_field":"ignored"}`

	// When
	lines, err := domain.Parse(strings.NewReader(input))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Title != "Task" {
		t.Errorf("title: got %q, want %q", lines[0].Title, "Task")
	}
}

func TestParse_LabelsMap_ParsedCorrectly(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"idempotency_key":"k1","role":"task","title":"Task","labels":{"kind":"bug","area":"auth"}}`

	// When
	lines, err := domain.Parse(strings.NewReader(input))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0].Labels["kind"] != "bug" {
		t.Errorf("labels[kind]: got %q, want %q", lines[0].Labels["kind"], "bug")
	}
	if lines[0].Labels["area"] != "auth" {
		t.Errorf("labels[area]: got %q, want %q", lines[0].Labels["area"], "auth")
	}
}
