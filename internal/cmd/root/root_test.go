package root

import (
	"errors"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

func newTestFactory() *cmdutil.Factory {
	ios, _, _, _ := iostreams.Test()
	return &cmdutil.Factory{
		IOStreams: ios,
		// DatabasePath returns an error so that tests exercising command
		// registration (not database logic) skip the schema version check.
		DatabasePath: func() (string, error) { return "", errors.New("no database in test") },
		Store: func() (*sqlite.Store, error) {
			return nil, errors.New("no database in test")
		},
	}
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

// ---------------------------------------------------------------------------
// isSchemaCheckExempt
// ---------------------------------------------------------------------------

func TestIsSchemaCheckExempt_AdminUpgrade_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args remaining after root flag parsing for "np admin upgrade".
	args := []string{"admin", "upgrade"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for admin upgrade")
	}
}

func TestIsSchemaCheckExempt_AdminDoctor_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args for "np admin doctor".
	args := []string{"admin", "doctor"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for admin doctor")
	}
}

func TestIsSchemaCheckExempt_AdminUpgradeWithLeadingFlag_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — "np admin --json upgrade": a boolean flag before the subcommand
	// name that must be skipped when identifying the command path.
	args := []string{"admin", "--json", "upgrade"}

	// When
	got := isSchemaCheckExempt(args)

	// Then — "--json" starts with '-' and is skipped; the first two non-flag
	// positional args are "admin" and "upgrade", which is in the exempt list.
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for admin --json upgrade")
	}
}

func TestIsSchemaCheckExempt_ClaimReady_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given — args for "np claim ready".
	args := []string{"claim", "ready"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if got {
		t.Error("expected isSchemaCheckExempt to return false for claim ready")
	}
}

func TestIsSchemaCheckExempt_AdminBackup_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given — args for "np admin backup".
	args := []string{"admin", "backup"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if got {
		t.Error("expected isSchemaCheckExempt to return false for admin backup")
	}
}

func TestIsSchemaCheckExempt_NoArgs_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given — no args (bare "np" invocation).
	args := []string{}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if got {
		t.Error("expected isSchemaCheckExempt to return false for no-arg invocation")
	}
}

func TestIsSchemaCheckExempt_SingleArg_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given — a single positional arg (e.g., "np version").
	args := []string{"version"}

	// When
	got := isSchemaCheckExempt(args)

	// Then — single-arg invocations have fewer than two non-flag positional
	// args and are never exempt.
	if got {
		t.Error("expected isSchemaCheckExempt to return false for single-arg invocation")
	}
}

// ---------------------------------------------------------------------------
// checkSchemaVersion
// ---------------------------------------------------------------------------

func TestCheckSchemaVersion_NoDatabaseFound_ReturnsNil(t *testing.T) {
	t.Parallel()

	// Given — a factory whose DatabasePath reports no database found.
	ios, _, _, _ := iostreams.Test()
	storeCalled := false
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return "", errors.New("no .np directory found") },
		Store: func() (*sqlite.Store, error) {
			storeCalled = true
			return nil, errors.New("store must not be opened when no database found")
		},
	}

	// When
	err := checkSchemaVersion(t.Context(), f)
	// Then — no error; the check is a no-op when there is no database.
	if err != nil {
		t.Errorf("expected nil from checkSchemaVersion when no database found, got: %v", err)
	}
	if storeCalled {
		t.Error("expected Store not to be called when no database found")
	}
}

func TestCheckSchemaVersion_StoreOpenFails_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a factory where DatabasePath succeeds but Store fails to open.
	ios, _, _, _ := iostreams.Test()
	openErr := errors.New("disk full")
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return "/np/nitpicking.db", nil },
		Store: func() (*sqlite.Store, error) {
			return nil, openErr
		},
	}

	// When
	err := checkSchemaVersion(t.Context(), f)

	// Then — the open error is wrapped and returned.
	if err == nil {
		t.Fatal("expected error from checkSchemaVersion when Store fails, got nil")
	}
	if !errors.Is(err, openErr) {
		t.Errorf("expected wrapped openErr, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Root Before schema gating
// ---------------------------------------------------------------------------

func TestNewRootCmd_NoDatabase_SkipsSchemaCheck(t *testing.T) {
	t.Parallel()

	// Given — a factory with no database found; DatabasePath returns an error.
	ios, _, _, _ := iostreams.Test()
	storeCalled := false
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return "", errors.New("no .np directory found") },
		Store: func() (*sqlite.Store, error) {
			storeCalled = true
			return nil, errors.New("store must not be opened for schema check when no database found")
		},
	}
	cmd := NewRootCmd(f)

	// When — run a command that does not require a database.
	// "np version" does not call f.Store() in its Action, so any Store call
	// would be caused by the schema check itself.
	_ = cmd.Run(t.Context(), []string{"np", "version"})

	// Then — the Store was not opened for the schema check.
	if storeCalled {
		t.Error("expected Store not to be called when no database is found during schema check")
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
