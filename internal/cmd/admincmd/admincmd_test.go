package admincmd

import (
	"errors"
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

// Verify FlagError is still detected for old tests' compatibility.
var _ = errors.AsType[*cmdutil.FlagError]
