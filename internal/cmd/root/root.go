package root

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/agent"
	"github.com/pinkhop/nitpicking/internal/cmd/claim"
	"github.com/pinkhop/nitpicking/internal/cmd/create"
	cmddelete "github.com/pinkhop/nitpicking/internal/cmd/delete"
	"github.com/pinkhop/nitpicking/internal/cmd/doctor"
	"github.com/pinkhop/nitpicking/internal/cmd/edit"
	"github.com/pinkhop/nitpicking/internal/cmd/extend"
	"github.com/pinkhop/nitpicking/internal/cmd/gc"
	"github.com/pinkhop/nitpicking/internal/cmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/historyview"
	cmdinit "github.com/pinkhop/nitpicking/internal/cmd/init"
	"github.com/pinkhop/nitpicking/internal/cmd/list"
	"github.com/pinkhop/nitpicking/internal/cmd/note"
	"github.com/pinkhop/nitpicking/internal/cmd/relate"
	"github.com/pinkhop/nitpicking/internal/cmd/search"
	"github.com/pinkhop/nitpicking/internal/cmd/show"
	"github.com/pinkhop/nitpicking/internal/cmd/transition"
	"github.com/pinkhop/nitpicking/internal/cmd/update"
	"github.com/pinkhop/nitpicking/internal/cmd/version"
	"github.com/pinkhop/nitpicking/internal/cmd/welcome"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

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
		Name:  "np",
		Usage: "A local-only, CLI-driven issue tracker for AI agent workflows",
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
		Commands: []*cli.Command{
			// Global operations — do not require database discovery.
			version.NewCmd(f),
			cmdinit.NewCmd(f),
			agent.NewCmd(f),
			welcome.NewCmd(f),

			// Ticket lifecycle.
			create.NewCmd(f),
			claim.NewCmd(f),
			update.NewCmd(f),
			edit.NewCmd(f),
			extend.NewCmd(f),
			cmddelete.NewCmd(f),

			// State transitions.
			transition.NewReleaseCmd(f),
			transition.NewStateCmd(f),

			// Queries.
			show.NewCmd(f),
			list.NewCmd(f),
			search.NewCmd(f),
			historyview.NewCmd(f),

			// Relationships and notes.
			relate.NewCmd(f),
			note.NewCmd(f),

			// Visualization.
			graphcmd.NewCmd(f),

			// Diagnostics and maintenance.
			doctor.NewCmd(f),
			gc.NewCmd(f),
		},
	}
}
