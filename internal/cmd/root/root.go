package root

import (
	"context"
	"io"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/agent"
	"github.com/pinkhop/nitpicking/internal/cmd/blocked"
	"github.com/pinkhop/nitpicking/internal/cmd/claim"
	"github.com/pinkhop/nitpicking/internal/cmd/comment"
	"github.com/pinkhop/nitpicking/internal/cmd/create"
	cmddelete "github.com/pinkhop/nitpicking/internal/cmd/delete"
	"github.com/pinkhop/nitpicking/internal/cmd/doctor"
	"github.com/pinkhop/nitpicking/internal/cmd/done"
	"github.com/pinkhop/nitpicking/internal/cmd/edit"
	"github.com/pinkhop/nitpicking/internal/cmd/extend"
	"github.com/pinkhop/nitpicking/internal/cmd/gc"
	"github.com/pinkhop/nitpicking/internal/cmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/historyview"
	cmdinit "github.com/pinkhop/nitpicking/internal/cmd/init"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd"
	"github.com/pinkhop/nitpicking/internal/cmd/list"
	"github.com/pinkhop/nitpicking/internal/cmd/ready"
	"github.com/pinkhop/nitpicking/internal/cmd/relate"
	"github.com/pinkhop/nitpicking/internal/cmd/search"
	"github.com/pinkhop/nitpicking/internal/cmd/show"
	"github.com/pinkhop/nitpicking/internal/cmd/status"
	"github.com/pinkhop/nitpicking/internal/cmd/transition"
	"github.com/pinkhop/nitpicking/internal/cmd/update"
	"github.com/pinkhop/nitpicking/internal/cmd/version"
	"github.com/pinkhop/nitpicking/internal/cmd/welcome"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// categoryOrder defines the workflow-first ordering of command groups in
// help output. urfave/cli sorts categories alphabetically; we override the
// root help template to render them in this sequence instead.
var categoryOrder = []string{
	"Setup",
	"Issue Lifecycle",
	"Workflow",
	"Query",
	"Annotations",
	"Maintenance",
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

// NewRootCmd constructs the root command with all subcommands registered.
// The Factory is passed to each subcommand constructor, allowing them to
// extract only the dependencies they need. The Before hook runs before
// every subcommand and enriches the context with cross-cutting concerns;
// the After hook tears down infrastructure started in Before.
func NewRootCmd(f *cmdutil.Factory) *cli.Command {
	// stopSignals deregisters the signal handler installed in Before.
	// Declared here so the After hook can call it during teardown.
	var stopSignals func()

	return &cli.Command{
		Name:                          "np",
		Usage:                         "A local-only, CLI-driven issue tracker for AI agent workflows",
		CustomRootCommandHelpTemplate: rootHelpTemplate,
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Signal handling wraps the context so SIGINT/SIGTERM trigger
			// cancellation for every subcommand. This is a process-wide
			// concern — multiple NotifyContext calls on the same signals
			// interfere with each other — so it belongs here.
			// Tests that call run() directly bypass Before entirely and
			// pass their own cancellable context.
			ctx, stopSignals = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

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
				welcome.NewCmd(f),
				cmdinit.NewCmd(f),
				agent.NewCmd(f),
				version.NewCmd(f),
			}},
			{"Issue Lifecycle", []*cli.Command{
				issuecmd.NewCmd(f),
				create.NewCmd(f),
				claim.NewCmd(f),
				update.NewCmd(f),
				edit.NewCmd(f),
				extend.NewCmd(f),
				cmddelete.NewCmd(f),
				transition.NewReleaseCmd(f),
				transition.NewStateCmd(f),
			}},
			{"Workflow", []*cli.Command{
				ready.NewCmd(f),
				blocked.NewCmd(f),
				status.NewCmd(f),
				done.NewCmd(f),
			}},
			{"Query", []*cli.Command{
				show.NewCmd(f),
				list.NewCmd(f),
				search.NewCmd(f),
				historyview.NewCmd(f),
			}},
			{"Annotations", []*cli.Command{
				relate.NewCmd(f),
				comment.NewCmd(f),
			}},
			{"Maintenance", []*cli.Command{
				graphcmd.NewCmd(f),
				doctor.NewCmd(f),
				gc.NewCmd(f),
			}},
		}),
	}
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
