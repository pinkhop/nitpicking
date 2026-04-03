package completion

import (
	"errors"
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
