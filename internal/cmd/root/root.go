package root

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd"
	"github.com/pinkhop/nitpicking/internal/cmd/agent"
	"github.com/pinkhop/nitpicking/internal/cmd/blocked"
	"github.com/pinkhop/nitpicking/internal/cmd/claim"
	"github.com/pinkhop/nitpicking/internal/cmd/closecmd"
	"github.com/pinkhop/nitpicking/internal/cmd/comment"
	"github.com/pinkhop/nitpicking/internal/cmd/create"
	"github.com/pinkhop/nitpicking/internal/cmd/epiccmd"
	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/importcmd"
	cmdinit "github.com/pinkhop/nitpicking/internal/cmd/init"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
	"github.com/pinkhop/nitpicking/internal/cmd/labelcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/list"
	"github.com/pinkhop/nitpicking/internal/cmd/ready"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/show"
	"github.com/pinkhop/nitpicking/internal/cmd/version"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// categoryOrder defines the workflow-first ordering of command groups in
// help output. urfave/cli sorts categories alphabetically; we override the
// root help template to render them in this sequence instead.
var categoryOrder = []string{
	"Setup",
	"Core Workflow",
	"Issues",
	"Agent Toolkit",
	"Admin",
	"Info",
}

func init() {
	// Inject the orderedCategories template function into urfave/cli's
	// help renderer so our custom root help template can iterate
	// categories in workflow order instead of alphabetical order.
	original := cli.HelpPrinterCustom
	cli.HelpPrinterCustom = func(w io.Writer, templ string, data any, customFuncs map[string]any) {
		if customFuncs == nil {
			customFuncs = map[string]any{}
		}
		customFuncs["orderedCategories"] = func() []string { return categoryOrder }
		original(w, templ, data, customFuncs)
	}
}

// schemaCheckExemptArgs identifies the two-part command paths that must operate
// on pre-migration (v1 or v2) databases and are therefore exempt from the
// startup schema version check. All other commands that find an existing
// database have the schema version verified before proceeding.
//
// "admin upgrade" performs the migration itself; it must accept v1 and v2
// databases.
// "admin doctor" diagnoses workspace health; it explicitly reports the
// schema_migration_required finding and must operate on pre-migration databases
// to do so.
// "admin where" reports the database path on disk; it does not read or write
// issue data, so its result is independent of schema version. Exempting it lets
// users locate their database (e.g., before backing it up) without first
// running "np admin upgrade".
var schemaCheckExemptArgs = map[[2]string]bool{
	{"admin", "upgrade"}: true,
	{"admin", "doctor"}:  true,
	{"admin", "where"}:   true,
}

// schemaCheckExemptSingleArgs identifies single-word command names that are
// exempt from the startup schema version check. These are commands that
// either create or manage the database itself, or do not require a database
// at all, and therefore cannot be gated on a pre-existing, fully-migrated
// schema.
//
// "init" creates the database file explicitly before opening it, so the
// schema check would find either no database (skipped automatically via the
// DatabasePath guard) or the freshly created file (no schema_version row,
// which CheckSchemaVersion would misread as a v1 database requiring
// migration). Exempting init avoids that false-positive block.
//
// "agent" covers the entire agent subcommand group (agent name, agent prime).
// These utilities are explicitly documented as not requiring an initialized
// workspace — they operate on pure domain logic (PRNG, static text) without
// any storage access, so the schema version is irrelevant.
var schemaCheckExemptSingleArgs = map[string]bool{
	"init":  true,
	"agent": true,
}

// isSchemaCheckExempt reports whether the command described by args is exempt
// from the startup schema version check. args contains the positional arguments
// remaining after the root command's own flags have been parsed — equivalent to
// cmd.Args().Slice() in the root Before hook.
//
// It extracts the first two non-flag positional arguments to identify the
// command path and checks them against both schemaCheckExemptSingleArgs (for
// single-word commands like "init") and schemaCheckExemptArgs (for two-part
// commands like "admin upgrade"). Flags (arguments starting with '-') are
// skipped so that interleaved flags such as "np admin --json doctor" resolve
// correctly.
func isSchemaCheckExempt(args []string) bool {
	// Collect the first two non-flag positional arguments to identify the
	// command path (e.g., ["admin", "upgrade"] for "np admin upgrade").
	var parts [2]string
	n := 0
	for _, a := range args {
		if len(a) == 0 || a[0] == '-' {
			continue
		}
		parts[n] = a
		n++
		if n == 2 {
			break
		}
	}

	if n == 0 {
		return false
	}

	// Check single-word exemptions first (e.g., "init").
	if schemaCheckExemptSingleArgs[parts[0]] {
		return true
	}

	if n < 2 {
		// Single-part invocation that is not in the single-word exempt list —
		// e.g., "np admin" showing help. Run the check.
		return false
	}

	return schemaCheckExemptArgs[parts]
}

// checkSchemaVersion opens the existing database (if any) and verifies that
// the schema version meets the minimum required version. It is a no-op when no
// database is found in the workspace — commands that require a database will
// report the missing-database error themselves.
//
// Returns an error (wrapping domain.ErrSchemaMigrationRequired) when the
// database is at v1 schema, directing the user to run "np admin upgrade".
func checkSchemaVersion(ctx context.Context, f *cmdutil.Factory) error {
	// Probe for a database without creating one. Commands that need a
	// database — but find none — will report their own "no database found"
	// error when they later call f.Store() or f.DatabasePath().
	if _, err := f.DatabasePath(); err != nil {
		return nil
	}

	// Open the existing database (memoised; subsequent f.Store() calls in the
	// command's Action return the same connection).
	store, err := f.Store()
	if err != nil {
		// .np/ exists but the database file is absent: the workspace has not
		// been initialized yet. Skip the check — np init will create the file,
		// and any other command that needs a database will surface its own
		// "run np init" error when it calls f.Store() in its Action.
		if errors.Is(err, domain.ErrDatabaseNotInitialized) {
			return nil
		}
		return fmt.Errorf("opening database for schema check: %w", err)
	}

	return store.CheckSchemaVersion(ctx)
}

// NewRootCmd constructs the root command with all subcommands registered.
// The Factory is passed to each subcommand constructor, allowing them to
// extract only the dependencies they need. The Before hook runs before
// every subcommand and enriches the context with cross-cutting concerns
// (signal handling and schema version gating); the After hook tears down
// infrastructure started in Before.
func NewRootCmd(f *cmdutil.Factory) *cli.Command {
	// stopSignals deregisters the signal handler installed in Before.
	// Declared here so the After hook can call it during teardown.
	var stopSignals func()

	rootCmd := &cli.Command{
		Name:                          "np",
		Usage:                         "A local-only, CLI-driven issue tracker for AI agent workflows",
		CustomRootCommandHelpTemplate: rootHelpTemplate,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "workspace",
				Usage:       "directory containing the .np/ workspace (skips parent traversal)",
				Sources:     cli.EnvVars("NP_WORKSPACE"),
				Destination: &f.Workspace,
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Signal handling wraps the context so SIGINT/SIGTERM trigger
			// cancellation for every subcommand. This is a process-wide
			// concern — multiple NotifyContext calls on the same signals
			// interfere with each other — so it belongs here.
			// Tests that call run() directly bypass Before entirely and
			// pass their own cancellable context.
			ctx, stopSignals = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

			// Schema version check: every command that reads or writes the
			// database must operate on a v2-or-later schema. Commands that
			// manage schema (admin upgrade, admin doctor) are exempt.
			// When no database is found, the check is skipped — the command
			// itself will surface a "no database found" error if it needs one.
			if !isSchemaCheckExempt(cmd.Args().Slice()) {
				if err := checkSchemaVersion(ctx, f); err != nil {
					return ctx, err
				}
			}

			return ctx, nil
		},
		After: func(ctx context.Context, cmd *cli.Command) error {
			// Deregister the signal handler installed in Before.
			if stopSignals != nil {
				stopSignals()
			}
			return nil
		},
		Commands: categorize([]commandGroup{
			{"Setup", []*cli.Command{
				cmdinit.NewCmd(f),
			}},
			{"Core Workflow", []*cli.Command{
				create.NewCmd(f),
				claim.NewCmd(f),
				closecmd.NewCmd(f),
				show.NewCmd(f),
				list.NewCmd(f),
				ready.NewCmd(f),
				blocked.NewCmd(f),
			}},
			{"Issues", []*cli.Command{
				issuecmd.NewCmd(f),
				epiccmd.NewCmd(f),
				relcmd.NewCmd(f),
				labelcmd.NewCmd(f),
				comment.NewCmd(f),
				formcmd.NewCmd(f),
			}},
			{"Agent Toolkit", []*cli.Command{
				jsoncmd.NewCmd(f),
				agent.NewCmd(f),
			}},
			{"Admin", []*cli.Command{
				admincmd.NewCmd(f),
				importcmd.NewCmd(f),
			}},
			{"Info", []*cli.Command{
				version.NewCmd(f),
			}},
		}),
	}

	return rootCmd
}

// commandGroup pairs a category label with the commands that belong to it.
type commandGroup struct {
	category string
	commands []*cli.Command
}

// categorize assigns each command its group's category and returns the
// flattened, ordered slice for the root command.
func categorize(groups []commandGroup) []*cli.Command {
	var all []*cli.Command
	for _, g := range groups {
		for _, cmd := range g.commands {
			cmd.Category = g.category
			all = append(all, cmd)
		}
	}
	return all
}

// rootHelpTemplate overrides the default urfave/cli root help template to
// render command categories in workflow-first order (defined by
// categoryOrder) instead of the framework's default alphabetical order.
//
// The template iterates orderedCategories (injected via init) and looks
// up matching categories from VisibleCategories, preserving the
// within-group command order set by categorize. Uncategorized commands
// (like "help") render at the end.
var rootHelpTemplate = `NAME:
   {{template "helpNameTemplate" .}}

USAGE:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} {{if .VisibleFlags}}[global options]{{end}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{template "descriptionTemplate" .}}{{end}}{{if .VisibleCommands}}

COMMANDS:{{range $_, $name := orderedCategories}}{{range $.VisibleCategories}}{{if eq .Name $name}}

   {{.Name}}:{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{range .VisibleCategories}}{{if not .Name}}{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{end}}{{end}}{{end}}{{if .VisibleFlagCategories}}

GLOBAL OPTIONS:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

GLOBAL OPTIONS:{{template "visibleFlagTemplate" .}}{{end}}
`
