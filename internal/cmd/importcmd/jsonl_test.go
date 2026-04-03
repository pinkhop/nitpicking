package importcmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/importcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// testAuthor is the author name used across JSONL import tests.
const testAuthor = "test-agent"

// --- Helpers ---

// setupService creates an in-memory service initialised with prefix "NP".
func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// noColorScheme returns a ColorScheme with colour output disabled, suitable
// for deterministic assertion on human-readable output.
func noColorScheme() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

// --- Tests ---

func TestJSONLRun_EmptyInput_PrintsNoLines(t *testing.T) {
	t.Parallel()

	// Given: a reader containing no JSONL lines.
	svc := setupService(t)
	var out bytes.Buffer

	// When: JSONLRun is called with an empty reader.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(""),
		FilePath:    "empty.jsonl",
		Author:      testAuthor,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})
	// Then: no error and the output indicates no lines.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); got != "No lines to import.\n" {
		t.Errorf("output = %q, want %q", got, "No lines to import.\n")
	}
}

func TestJSONLRun_EmptyInput_JSON_ReturnsEmptyResult(t *testing.T) {
	t.Parallel()

	// Given: a reader containing no JSONL lines and JSON output enabled.
	svc := setupService(t)
	var out bytes.Buffer

	// When: JSONLRun is called with an empty reader and JSON=true.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(""),
		FilePath:    "empty.jsonl",
		Author:      testAuthor,
		JSON:        true,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})
	// Then: no error and the JSON output has action=import with zero counts.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["action"] != "import" {
		t.Errorf("action = %v, want %q", result["action"], "import")
	}
	if result["source"] != "empty.jsonl" {
		t.Errorf("source = %v, want %q", result["source"], "empty.jsonl")
	}
}

func TestJSONLRun_SingleTask_ImportsSuccessfully(t *testing.T) {
	t.Parallel()

	// Given: a JSONL reader with one valid task line.
	svc := setupService(t)
	var out bytes.Buffer
	input := `{"role":"task","title":"Test task","idempotency_key":"key-1"}` + "\n"

	// When: JSONLRun is called.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(input),
		FilePath:    "tasks.jsonl",
		Author:      testAuthor,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})
	// Then: no error and output mentions 1 issue imported.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "1 issues") {
		t.Errorf("output = %q, want to contain %q", got, "1 issues")
	}
}

func TestJSONLRun_SingleTask_JSON_ReturnsCreatedCount(t *testing.T) {
	t.Parallel()

	// Given: a JSONL reader with one valid task line and JSON output enabled.
	svc := setupService(t)
	var out bytes.Buffer
	input := `{"role":"task","title":"Test task","idempotency_key":"key-2"}` + "\n"

	// When: JSONLRun is called with JSON=true.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(input),
		FilePath:    "tasks.jsonl",
		Author:      testAuthor,
		JSON:        true,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})
	// Then: no error and JSON output shows created=1.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["action"] != "imported" {
		t.Errorf("action = %v, want %q", result["action"], "imported")
	}
	if result["created"] != float64(1) {
		t.Errorf("created = %v, want 1", result["created"])
	}
}

func TestJSONLRun_ValidationError_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a JSONL reader with an invalid line (missing required title).
	svc := setupService(t)
	var stdout, stderr bytes.Buffer
	input := `{"role":"task"}` + "\n"

	// When: JSONLRun is called.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(input),
		FilePath:    "bad.jsonl",
		Author:      testAuthor,
		WriteTo:     &stdout,
		ErrWriteTo:  &stderr,
		ColorScheme: noColorScheme,
	})

	// Then: an error is returned indicating validation failure.
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "validation failed")
	}
}

func TestJSONLRun_ValidationError_JSON_ReturnsStructuredErrors(t *testing.T) {
	t.Parallel()

	// Given: a JSONL reader with an invalid line and JSON output enabled.
	svc := setupService(t)
	var out bytes.Buffer
	input := `{"role":"task"}` + "\n"

	// When: JSONLRun is called with JSON=true.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader(input),
		FilePath:    "bad.jsonl",
		Author:      testAuthor,
		JSON:        true,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})
	// Then: no error (JSON validation errors are written to output, not returned).
	// The JSON output should indicate validation_failed.
	if err != nil {
		t.Fatalf("unexpected error for JSON validation output: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if result["action"] != "validation_failed" {
		t.Errorf("action = %v, want %q", result["action"], "validation_failed")
	}
}

func TestJSONLRun_MalformedJSON_ReturnsParseError(t *testing.T) {
	t.Parallel()

	// Given: a reader containing malformed JSON.
	svc := setupService(t)
	var out bytes.Buffer

	// When: JSONLRun is called with malformed input.
	err := importcmd.JSONLRun(t.Context(), importcmd.JSONLRunInput{
		Service:     svc,
		Reader:      strings.NewReader("{not json}\n"),
		FilePath:    "broken.jsonl",
		Author:      testAuthor,
		WriteTo:     &out,
		ErrWriteTo:  &out,
		ColorScheme: noColorScheme,
	})

	// Then: a parse error is returned.
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing JSONL") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "parsing JSONL")
	}
}
