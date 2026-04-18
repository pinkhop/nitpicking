package completion

import (
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

func TestNewCmd_MissingShellArg_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmd(&cmdutil.Factory{IOStreams: ios})

	// When
	err := cmd.Run(t.Context(), []string{"completion"})

	// Then
	if err == nil {
		t.Fatal("expected error when shell arg is missing")
	}
	if _, ok := errors.AsType[*cmdutil.FlagError](err); !ok {
		t.Fatalf("expected FlagError, got %T: %v", err, err)
	}
}

func TestNewCmd_UnsupportedShell_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	cmd := NewCmd(&cmdutil.Factory{IOStreams: ios})

	// When
	err := cmd.Run(t.Context(), []string{"completion", "powershell"})

	// Then
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}
	if _, ok := errors.AsType[*cmdutil.FlagError](err); !ok {
		t.Fatalf("expected FlagError, got %T: %v", err, err)
	}
}

// TestBashCompletion_RelSubcommands_IncludesCurrentSubcommandSet asserts that the
// bash completion script lists all current "rel" subcommands. This test exists
// to prevent the completion script from drifting away from the actual command
// tree when rel subcommands are added or renamed.
func TestBashCompletion_RelSubcommands_IncludesCurrentSubcommandSet(t *testing.T) {
	t.Parallel()

	// Given: the current rel subcommands defined in relcmd.NewCmd.
	wantSubcmds := []string{"add", "remove", "blocks", "refs", "parent", "issue", "tree", "graph"}

	// When/Then: the bash script contains each expected subcommand.
	for _, sub := range wantSubcmds {
		if !strings.Contains(bashCompletion, sub) {
			t.Errorf("bashCompletion missing rel subcommand %q", sub)
		}
	}
	// The "list" rel subcommand was renamed to "issue" in commit 9b90a39; verify
	// the obsolete token string is no longer present. We match the full old token
	// list rather than the bare word "list" to avoid false positives from other
	// commands that use "list" as a subcommand (e.g., label, comment).
	if strings.Contains(bashCompletion, `add blocks refs parent list tree graph`) {
		t.Errorf("bashCompletion still contains obsolete rel subcommand list; update the script to match the current command tree")
	}
}

// TestFishCompletion_RelSubcommands_IncludesCurrentSubcommandSet asserts that
// the fish completion script lists all current "rel" subcommands. This test
// exists to prevent the completion script from drifting away from the actual
// command tree when rel subcommands are added or renamed.
func TestFishCompletion_RelSubcommands_IncludesCurrentSubcommandSet(t *testing.T) {
	t.Parallel()

	// Given: the current rel subcommands defined in relcmd.NewCmd.
	wantSubcmds := []string{"add", "remove", "blocks", "refs", "parent", "issue", "tree", "graph"}

	// When/Then: the fish script contains each expected subcommand.
	for _, sub := range wantSubcmds {
		if !strings.Contains(fishCompletion, sub) {
			t.Errorf("fishCompletion missing rel subcommand %q", sub)
		}
	}
	if strings.Contains(fishCompletion, `add blocks refs parent list tree graph`) {
		t.Errorf("fishCompletion still contains obsolete 'list' rel subcommand in old token string")
	}
}

func TestNewCmd_SelectsStableScriptForSupportedShells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		shell string
		want  string
	}{
		{name: "bash", shell: "bash", want: bashCompletion},
		{name: "zsh", shell: "zsh", want: zshCompletion},
		{name: "fish", shell: "fish", want: fishCompletion},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			ios, _, stdout, _ := iostreams.Test()
			cmd := NewCmd(&cmdutil.Factory{IOStreams: ios})

			// When
			err := cmd.Run(t.Context(), []string{"completion", tc.shell})
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stdout.String() != tc.want {
				t.Fatalf("script mismatch for %s\nwant:\n%s\ngot:\n%s", tc.shell, tc.want, stdout.String())
			}
		})
	}
}
