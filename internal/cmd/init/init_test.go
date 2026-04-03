package init_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	initcmd "github.com/pinkhop/nitpicking/internal/cmd/init"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// newUninitializedService returns a service that has NOT been initialized,
// so it can be used to test the init command's actual Init call.
func newUninitializedService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	return core.New(tx)
}

func noColor() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

// --- Run Tests ---

func TestRun_EmptyPrefix_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given — empty prefix
	svc := newUninitializedService(t)

	var buf bytes.Buffer
	input := initcmd.RunInput{
		Service:     svc,
		Prefix:      "",
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := initcmd.Run(t.Context(), input)
	// Then
	if err == nil {
		t.Fatal("expected error for empty prefix")
	}
	if _, ok := errors.AsType[*cmdutil.FlagError](err); !ok {
		t.Errorf("expected FlagError, got %T: %v", err, err)
	}
}

func TestRun_WhitespaceOnlyPrefix_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given — whitespace-only prefix
	svc := newUninitializedService(t)

	var buf bytes.Buffer
	input := initcmd.RunInput{
		Service:     svc,
		Prefix:      "   ",
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := initcmd.Run(t.Context(), input)
	// Then
	if err == nil {
		t.Fatal("expected error for whitespace-only prefix")
	}
	if _, ok := errors.AsType[*cmdutil.FlagError](err); !ok {
		t.Errorf("expected FlagError, got %T: %v", err, err)
	}
}

func TestRun_ValidPrefix_InitializesAndOutputsJSON(t *testing.T) {
	t.Parallel()

	// Given — a valid prefix on an uninitialized service
	svc := newUninitializedService(t)

	var buf bytes.Buffer
	input := initcmd.RunInput{
		Service:     svc,
		Prefix:      "TST",
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := initcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if result.Prefix != "TST" {
		t.Errorf("prefix: got %q, want %q", result.Prefix, "TST")
	}
}

func TestRun_ValidPrefix_TextOutput_ShowsSuccess(t *testing.T) {
	t.Parallel()

	// Given — a valid prefix
	svc := newUninitializedService(t)

	var buf bytes.Buffer
	input := initcmd.RunInput{
		Service:     svc,
		Prefix:      "NP",
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := initcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Initialized") {
		t.Errorf("expected 'Initialized' in text output, got: %s", output)
	}
	if !strings.Contains(output, "NP") {
		t.Errorf("expected prefix 'NP' in text output, got: %s", output)
	}
}
