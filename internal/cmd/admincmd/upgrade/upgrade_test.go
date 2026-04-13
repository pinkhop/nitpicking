package upgrade_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/upgrade"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// --- NewCmd flag structure ---

func TestNewCmd_HasJSONFlag(t *testing.T) {
	t.Parallel()

	// Given — a command constructed with a minimal factory.
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When — the command is constructed.
	cmd := upgrade.NewCmd(f)

	// Then — the command exposes a --json flag.
	var found bool
	for _, fl := range cmd.Flags {
		for _, name := range fl.Names() {
			if name == "json" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected --json flag on admin upgrade command")
	}
}

func TestNewCmd_CommandName(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When
	cmd := upgrade.NewCmd(f)

	// Then — the command name is "upgrade".
	if cmd.Name != "upgrade" {
		t.Errorf("expected command name %q, got %q", "upgrade", cmd.Name)
	}
}

// --- NewCmd via runFn injection ---

// TestNewCmd_RunFn_UpToDate_TextOutput verifies that when Run returns "up to
// date", the text output contains the expected phrase.
func TestNewCmd_RunFn_UpToDate_TextOutput(t *testing.T) {
	t.Parallel()

	// Given — a factory with captured output streams and a stub runFn.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		cs := iostreams.NewColorScheme(false)
		_, err := input.Out.Write([]byte(cs.SuccessIcon() + " Database is up to date\n"))
		return err
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Database is up to date") {
		t.Errorf("expected 'Database is up to date' in output, got: %q", stdout.String())
	}
}

// TestNewCmd_RunFn_Migrated_JSONOutput verifies that the migrated status is
// emitted as valid JSON with the correct structure.
func TestNewCmd_RunFn_Migrated_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given — a factory with captured output streams and a stub runFn that
	// simulates a completed migration.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		return cmdutil.WriteJSON(input.Out, map[string]interface{}{
			"status":                   "migrated",
			"claimed_issues_converted": 3,
			"history_rows_removed":     7,
		})
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — no error and valid JSON with "migrated" status.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON output: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "migrated" {
		t.Errorf("status: got %v, want %q", out["status"], "migrated")
	}
}

// TestNewCmd_RunFn_UpToDate_JSONOutput verifies that up_to_date is emitted as
// valid JSON when --json is passed.
func TestNewCmd_RunFn_UpToDate_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		return cmdutil.WriteJSON(input.Out, map[string]interface{}{
			"status":                   "up_to_date",
			"claimed_issues_converted": 0,
			"history_rows_removed":     0,
		})
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON output: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "up_to_date" {
		t.Errorf("status: got %v, want %q", out["status"], "up_to_date")
	}
}
