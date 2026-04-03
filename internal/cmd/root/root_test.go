package root

import (
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

func newTestFactory() *cmdutil.Factory {
	ios, _, _, _ := iostreams.Test()
	return &cmdutil.Factory{IOStreams: ios}
}

func TestCategorize_AssignsCategoryAndPreservesOrder(t *testing.T) {
	t.Parallel()

	// Given
	groups := []commandGroup{
		{
			category: "Core Workflow",
			commands: []*cli.Command{
				{Name: "ready"},
				{Name: "blocked"},
			},
		},
		{
			category: "Admin",
			commands: []*cli.Command{
				{Name: "admin"},
			},
		},
	}

	// When
	got := categorize(groups)

	// Then
	if len(got) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(got))
	}
	if got[0].Name != "ready" || got[0].Category != "Core Workflow" {
		t.Errorf("got[0] = %s in %s, want ready in Core Workflow", got[0].Name, got[0].Category)
	}
	if got[1].Name != "blocked" || got[1].Category != "Core Workflow" {
		t.Errorf("got[1] = %s in %s, want blocked in Core Workflow", got[1].Name, got[1].Category)
	}
	if got[2].Name != "admin" || got[2].Category != "Admin" {
		t.Errorf("got[2] = %s in %s, want admin in Admin", got[2].Name, got[2].Category)
	}
}

func TestNewRootCmd_ClaimCommand_VisibleInCoreWorkflow(t *testing.T) {
	t.Parallel()

	// When
	cmd := NewRootCmd(newTestFactory())

	// Then — claim is registered, visible, and categorized as Core Workflow.
	for _, sub := range cmd.Commands {
		if sub.Name == "claim" {
			if sub.Hidden {
				t.Fatal("claim command must not be hidden")
			}
			if sub.Category != "Core Workflow" {
				t.Fatalf("claim category = %q, want %q", sub.Category, "Core Workflow")
			}
			return
		}
	}
	t.Fatal("expected root command to register claim subcommand")
}

func TestNewRootCmd_NoHiddenCommands(t *testing.T) {
	t.Parallel()

	// When
	cmd := NewRootCmd(newTestFactory())

	// Then — no command anywhere in the tree has Hidden set.
	var walk func(prefix string, cmds []*cli.Command)
	walk = func(prefix string, cmds []*cli.Command) {
		for _, sub := range cmds {
			path := prefix + " " + sub.Name
			if sub.Hidden {
				t.Errorf("command %q has Hidden = true", path)
			}
			walk(path, sub.Commands)
		}
	}
	walk("np", cmd.Commands)
}

func TestNewRootCmd_RegistersWorkspaceGlobalFlag(t *testing.T) {
	t.Parallel()

	// When
	cmd := NewRootCmd(newTestFactory())

	// Then — the root command has a --workspace flag.
	found := false
	for _, flag := range cmd.Flags {
		if sf, ok := flag.(*cli.StringFlag); ok && sf.Name == "workspace" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected root command to have a --workspace flag")
	}
}

func TestNewRootCmd_WorkspaceFlag_SetsFactoryWorkspace(t *testing.T) {
	t.Parallel()

	// Given
	f := newTestFactory()
	cmd := NewRootCmd(f)

	// When — run with --workspace flag.
	err := cmd.Run(t.Context(), []string{"np", "--workspace", "/some/dir", "version"})
	// Then — Factory.Workspace is set.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Workspace != "/some/dir" {
		t.Errorf("expected Workspace %q, got %q", "/some/dir", f.Workspace)
	}
}

func TestRootHelpTemplate_UsesOrderedCategoriesFunction(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(newTestFactory())

	if cmd.CustomRootCommandHelpTemplate != rootHelpTemplate {
		t.Fatal("expected root command to use custom root help template")
	}
	if !strings.Contains(rootHelpTemplate, "orderedCategories") {
		t.Fatalf("expected root help template to iterate orderedCategories, got:\n%s", rootHelpTemplate)
	}

	want := []string{"Setup", "Core Workflow", "Issues", "Agent Toolkit", "Admin", "Info"}
	if len(categoryOrder) != len(want) {
		t.Fatalf("categoryOrder length = %d, want %d", len(categoryOrder), len(want))
	}
	for i, name := range want {
		if categoryOrder[i] != name {
			t.Fatalf("categoryOrder[%d] = %q, want %q", i, categoryOrder[i], name)
		}
	}
}
