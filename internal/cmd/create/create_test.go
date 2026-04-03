package create_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/create"
	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Test helpers ---

// setupService initialises a service backed by in-memory fakes and returns it
// ready for create routing tests.
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

// --- Routing Tests ---

func TestRun_PipedStdin_DelegatesToJSONCreate(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with stdin NOT a TTY (simulating piped input) and a
	// valid JSON payload on stdin.
	svc := setupService(t)

	ios, stdin, stdout, _ := iostreams.Test()
	// Default: stdin is not a TTY (pipe mode).

	_, _ = stdin.WriteString(`{"role": "task", "title": "Piped task"}`)

	input := create.RunInput{
		Service:   svc,
		Author:    "alice",
		IOStreams: ios,
		WriteTo:   stdout,
		FormRunner: func(_ *formcmd.CreateFormData) error {
			t.Fatal("form runner should not be called in pipe mode")
			return nil
		},
	}

	// When
	err := create.Run(t.Context(), input)
	// Then: no error, and JSON output contains expected fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["role"] != "task" {
		t.Errorf("role: got %q, want %q", result["role"], "task")
	}
	if result["title"] != "Piped task" {
		t.Errorf("title: got %q, want %q", result["title"], "Piped task")
	}
}

func TestRun_TTYStdin_DelegatesToFormCreate(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with stdin IS a TTY (simulating interactive terminal).
	svc := setupService(t)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdinTTY(true)

	formCalled := false
	input := create.RunInput{
		Service:   svc,
		Author:    "bob",
		IOStreams: ios,
		WriteTo:   stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			formCalled = true
			data.Role = "task"
			data.Title = "Interactive task"
			data.Author = "bob"
			return nil
		},
	}

	// When
	err := create.Run(t.Context(), input)
	// Then: no error, and the form runner was called.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !formCalled {
		t.Error("expected form runner to be called in TTY mode")
	}

	// Output should contain the issue title (human-readable form output).
	out := stdout.String()
	if !strings.Contains(out, "Interactive task") {
		t.Errorf("expected output to contain issue title, got: %s", out)
	}
}

func TestRun_PipedStdin_InvalidJSON_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with piped stdin and invalid JSON.
	svc := setupService(t)

	ios, stdin, stdout, _ := iostreams.Test()
	_, _ = stdin.WriteString(`not valid json`)

	input := create.RunInput{
		Service:   svc,
		Author:    "alice",
		IOStreams: ios,
		WriteTo:   stdout,
	}

	// When
	err := create.Run(t.Context(), input)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRun_TTYStdin_FormAbort_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with TTY stdin and a form runner that aborts.
	svc := setupService(t)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdinTTY(true)

	input := create.RunInput{
		Service:   svc,
		Author:    "charlie",
		IOStreams: ios,
		WriteTo:   stdout,
		FormRunner: func(_ *formcmd.CreateFormData) error {
			return formcmd.ErrUserAborted
		},
	}

	// When
	err := create.Run(t.Context(), input)

	// Then: the abort error is surfaced.
	if err == nil {
		t.Fatal("expected error for user abort, got nil")
	}
}

func TestRun_PipedStdin_EmptyStdin_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with piped stdin but empty content.
	svc := setupService(t)

	ios, _, stdout, _ := iostreams.Test()
	// stdin is empty, not a TTY

	input := create.RunInput{
		Service:   svc,
		Author:    "alice",
		IOStreams: ios,
		WriteTo:   stdout,
	}

	// When
	err := create.Run(t.Context(), input)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

func TestRun_PipedStdin_WithAllFields_CreatesIssue(t *testing.T) {
	t.Parallel()

	// Given: IOStreams with piped stdin and a complete JSON payload.
	svc := setupService(t)

	ios, stdin, stdout, _ := iostreams.Test()

	// The claim field in JSON is silently ignored — claiming requires the
	// --with-claim CLI flag on json create. This test verifies the claim field
	// does not cause an error but does not claim the issue.
	payload := `{
		"role": "task",
		"title": "Full featured task",
		"description": "A detailed description",
		"priority": "P0",
		"claim": true
	}`
	_, _ = stdin.WriteString(payload)

	var buf bytes.Buffer
	input := create.RunInput{
		Service:   svc,
		Author:    "alice",
		IOStreams: ios,
		WriteTo:   &buf,
	}

	// Redirect output to our buffer instead
	_ = stdout // unused in this test

	// When
	err := create.Run(t.Context(), input)
	// Then: no error, and JSON output contains expected fields. The claim field
	// in JSON is silently ignored so the issue is created in open state.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	if result["priority"] != "P0" {
		t.Errorf("priority: got %q, want %q", result["priority"], "P0")
	}
	if result["state"] != "open" {
		t.Errorf("state: got %q, want %q (claim in JSON should be ignored)", result["state"], "open")
	}
}
