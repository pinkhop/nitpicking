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

func TestIsSchemaCheckExempt_AdminWhere_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args for "np admin where". The "where" command reports the database
	// path on disk and must not be blocked by a schema version check — its purpose
	// is independent of schema state and it is the command a user would run before
	// upgrading to find out where their database lives.
	args := []string{"admin", "where"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for admin where")
	}
}

func TestIsSchemaCheckExempt_AdminWhereWithLeadingFlag_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — "np admin --json where": a flag interleaved before the subcommand
	// name must not prevent the exemption from being recognised.
	args := []string{"admin", "--json", "where"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for admin --json where")
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

	// Then — single-arg invocations that are not in the single-word exempt
	// list (schemaCheckExemptSingleArgs) are never exempt.
	if got {
		t.Error("expected isSchemaCheckExempt to return false for single-arg invocation")
	}
}

func TestIsSchemaCheckExempt_Init_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args remaining after root flag parsing for "np init FOO".
	// The init command creates a fresh database from scratch and must never be
	// gated by a schema version check — before init runs, the schema has not
	// been created yet.
	args := []string{"init", "FOO"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for init")
	}
}

func TestIsSchemaCheckExempt_InitWithFlag_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — "np init --json FOO": a flag interleaved before the prefix arg
	// must not prevent the single-word exemption from being recognised.
	args := []string{"init", "--json", "FOO"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for init --json FOO")
	}
}

func TestIsSchemaCheckExempt_InitNoPrefix_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — "np init" with no prefix argument. The command will fail with a
	// usage error, but the schema check must still be skipped so the error
	// message comes from init's own validation, not from a spurious schema check.
	args := []string{"init"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for bare init")
	}
}

func TestIsSchemaCheckExempt_AgentName_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args for "np agent name". The agent subcommands are explicitly
	// documented as not requiring a database; they use pure domain logic and
	// must work in any directory, even one without an np workspace.
	args := []string{"agent", "name"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for agent name")
	}
}

func TestIsSchemaCheckExempt_AgentPrime_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args for "np agent prime". Like agent name, this subcommand
	// returns static text and never accesses the database.
	args := []string{"agent", "prime"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for agent prime")
	}
}

func TestIsSchemaCheckExempt_AgentNameWithSeedFlag_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given — args for "np agent name --seed=foo". The flag must not prevent
	// the single-word exemption from being recognised.
	args := []string{"agent", "--seed=foo", "name"}

	// When
	got := isSchemaCheckExempt(args)

	// Then
	if !got {
		t.Error("expected isSchemaCheckExempt to return true for agent --seed=foo name")
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

func TestNewRootCmd_AdminWhere_SucceedsWhenSchemaMigrationRequired(t *testing.T) {
	t.Parallel()

	// Given — a factory whose DatabasePath succeeds but whose Store returns an
	// error simulating a pre-migration (v1 or v2) database. Because "admin where"
	// is exempt from the schema version check, the command must succeed regardless
	// of schema state. "admin where" may attempt to open the store to read the
	// issue prefix, but it must absorb any store failure and omit the prefix
	// rather than surfacing it as a command error.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return "/project/.np/nitpicking.db", nil },
		Store: func() (*sqlite.Store, error) {
			return nil, errors.New("pre-migration database: schema upgrade required")
		},
	}
	cmd := NewRootCmd(f)

	// When — run "np admin where".
	err := cmd.Run(t.Context(), []string{"np", "admin", "where"})
	// Then — the command succeeds even though the store is unavailable: the
	// schema check is skipped, the .np/ directory path is printed, and the
	// prefix is simply omitted (store failure is absorbed).
	if err != nil {
		t.Fatalf("expected np admin where to succeed on pre-migration database, got: %v", err)
	}
	output := strings.TrimSpace(stdout.String())
	if output != "/project/.np" {
		t.Errorf("expected output %q, got %q", "/project/.np", output)
	}
}

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

func TestNewRootCmd_Init_SchemaCheckSkipped_EvenWhenDatabasePathSucceeds(t *testing.T) {
	t.Parallel()

	// Given — a factory where DatabasePath succeeds (simulating a .np/ directory
	// that exists) but CheckSchemaVersion would fail if called (e.g., because the
	// database file has no schema). If the Before hook called checkSchemaVersion
	// for init, it would open the Store, call CheckSchemaVersion, and return an
	// error that does not originate from the init action itself. We verify that
	// the error returned from "np init" is NOT a schema-version error.
	ios, _, _, _ := iostreams.Test()
	schemaCheckErr := errors.New("sentinel: schema check must not be called for init")
	f := &cmdutil.Factory{
		IOStreams: ios,
		// DatabasePath succeeds as if .np/ already exists.
		DatabasePath: func() (string, error) { return "/project/.np/nitpicking.db", nil },
		// Store returns a specific sentinel error so we can tell if it was the
		// Before hook calling it (via checkSchemaVersion) rather than the init action.
		Store: func() (*sqlite.Store, error) {
			return nil, schemaCheckErr
		},
	}
	cmd := NewRootCmd(f)

	// When — run "np init FOO".
	err := cmd.Run(t.Context(), []string{"np", "init", "FOO"})

	// Then — the error is from the init action's store call (or the already-
	// initialised guard), NOT from the Before hook's schema check. Specifically,
	// the error must not be an "opening database for schema check" error, which
	// would indicate that checkSchemaVersion was called before init's own logic.
	if err != nil && strings.Contains(err.Error(), "opening database for schema check") {
		t.Errorf("schema check must be skipped for np init, but got schema-check error: %v", err)
	}
}

func TestNewRootCmd_AgentName_SchemaCheckSkipped_EvenWhenDatabasePathSucceeds(t *testing.T) {
	t.Parallel()

	// Given — a factory where DatabasePath succeeds (simulating a .np/ directory
	// with a pre-migration database). If the Before hook called checkSchemaVersion
	// for agent name, it would open the Store, call CheckSchemaVersion, and return
	// a schema-version error. We verify that "np agent name" is exempt.
	ios, _, _, _ := iostreams.Test()
	schemaCheckErr := errors.New("sentinel: schema check must not be called for agent name")
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return "/project/.np/nitpicking.db", nil },
		// Store returns a sentinel error so we can detect if checkSchemaVersion
		// called it via the Before hook.
		Store: func() (*sqlite.Store, error) {
			return nil, schemaCheckErr
		},
	}
	cmd := NewRootCmd(f)

	// When — run "np agent name".
	err := cmd.Run(t.Context(), []string{"np", "agent", "name"})

	// Then — any error must not originate from the schema check in the Before hook.
	if err != nil && strings.Contains(err.Error(), "opening database for schema check") {
		t.Errorf("schema check must be skipped for np agent name, but got schema-check error: %v", err)
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
