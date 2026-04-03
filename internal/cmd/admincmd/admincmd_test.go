package admincmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

func TestNewResetCmd_HasResetKeyFlag(t *testing.T) {
	// Given
	ios, _, _, _ := iostreams.Test()
	cmd := newResetCmd(&cmdutil.Factory{IOStreams: ios})

	// Then — the command has a --reset-key flag.
	var hasResetKey bool
	for _, fl := range cmd.Flags {
		for _, name := range fl.Names() {
			if name == "reset-key" {
				hasResetKey = true
			}
		}
	}
	if !hasResetKey {
		t.Error("expected --reset-key flag on admin reset command")
	}
}

func TestNewResetCmd_ConfirmFlagRemoved(t *testing.T) {
	// Given
	ios, _, _, _ := iostreams.Test()
	cmd := newResetCmd(&cmdutil.Factory{IOStreams: ios})

	// Then — the command does NOT have a --confirm flag.
	for _, fl := range cmd.Flags {
		for _, name := range fl.Names() {
			if name == "confirm" {
				t.Error("--confirm flag should be removed from admin reset")
			}
		}
	}
}

func TestNewUpgradeCmd_JSONSuccessPath(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	original := validateDatabase
	validateDatabase = func(_ *cmdutil.Factory) error { return nil }
	t.Cleanup(func() { validateDatabase = original })

	// When
	err := newUpgradeCmd(f).Run(t.Context(), []string{"upgrade", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if out["status"] != "up_to_date" {
		t.Fatalf("status: got %q, want %q", out["status"], "up_to_date")
	}
}

func TestNewUpgradeCmd_TextSuccessPath(t *testing.T) {
	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}
	original := validateDatabase
	validateDatabase = func(_ *cmdutil.Factory) error { return nil }
	t.Cleanup(func() { validateDatabase = original })

	// When
	err := newUpgradeCmd(f).Run(t.Context(), []string{"upgrade"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Database is up to date") {
		t.Fatalf("expected success output, got %q", stdout.String())
	}
}

// Verify FlagError is still detected for old tests' compatibility.
var _ = errors.AsType[*cmdutil.FlagError]
