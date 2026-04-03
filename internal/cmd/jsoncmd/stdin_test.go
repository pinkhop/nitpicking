package jsoncmd_test

import (
	"io"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
)

// testPayload is a simple struct used across stdin decoding tests.
type testPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestDecodeStdin_ValidJSON_DecodesIntoTarget(t *testing.T) {
	t.Parallel()

	// Given: a reader containing valid JSON matching the target struct.
	reader := strings.NewReader(`{"name": "alpha", "count": 7}`)

	// When: DecodeStdin is called with the reader.
	got, err := jsoncmd.DecodeStdin[testPayload](reader)
	// Then: the struct is populated and no error is returned.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "alpha" {
		t.Errorf("Name = %q, want %q", got.Name, "alpha")
	}
	if got.Count != 7 {
		t.Errorf("Count = %d, want %d", got.Count, 7)
	}
}

func TestDecodeStdin_EmptyReader_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an empty reader.
	reader := strings.NewReader("")

	// When: DecodeStdin is called.
	_, err := jsoncmd.DecodeStdin[testPayload](reader)

	// Then: an error is returned indicating the input was empty.
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}

func TestDecodeStdin_MalformedJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a reader containing malformed JSON.
	reader := strings.NewReader(`{"name": broken}`)

	// When: DecodeStdin is called.
	_, err := jsoncmd.DecodeStdin[testPayload](reader)

	// Then: an error is returned indicating invalid JSON.
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestDecodeStdin_RejectsExtraFields(t *testing.T) {
	t.Parallel()

	// Given: a reader containing JSON with an unknown field.
	reader := strings.NewReader(`{"name": "alpha", "unknown_field": true}`)

	// When: DecodeStdin is called.
	_, err := jsoncmd.DecodeStdin[testPayload](reader)

	// Then: an error is returned because unknown fields are rejected.
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestDecodeStdin_RejectsMultipleObjects(t *testing.T) {
	t.Parallel()

	// Given: a reader containing two JSON objects concatenated.
	reader := strings.NewReader(`{"name": "a"}{"name": "b"}`)

	// When: DecodeStdin is called.
	_, err := jsoncmd.DecodeStdin[testPayload](reader)

	// Then: an error is returned because only one object is accepted.
	if err == nil {
		t.Fatal("expected error for multiple JSON objects, got nil")
	}
}

func TestDecodeStdin_NilReader_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a nil reader.
	var reader io.Reader

	// When: DecodeStdin is called.
	_, err := jsoncmd.DecodeStdin[testPayload](reader)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for nil reader, got nil")
	}
}
