package gc

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// gcOutput is the JSON representation of the gc command result.
type gcOutput struct {
	DeletedIssuesRemoved int `json:"deleted_issues_removed"`
	ClosedIssuesRemoved  int `json:"closed_issues_removed"`
}

// NewCmd constructs the "gc" command, which physically removes soft-deleted
// issue data and optionally closed issue data from the database.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		confirm       bool
		includeClosed bool
	)

	return &cli.Command{
		Name:  "gc",
		Usage: "Garbage-collect deleted (and optionally closed) issues",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.BoolFlag{
				Name:        "confirm",
				Usage:       "Confirm the garbage collection (required)",
				Destination: &confirm,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Aliases:     []string{"aggressive"},
				Usage:       "Also remove closed issues (not just deleted)",
				Destination: &includeClosed,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !confirm {
				return cmdutil.FlagErrorf("--confirm is required to run garbage collection")
			}

			input := service.GCInput{
				IncludeClosed: includeClosed,
			}

			store, err := f.Store()
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			svc := service.New(store)
			result, err := svc.GC(ctx, input)
			if err != nil {
				return fmt.Errorf("running garbage collection: %w", err)
			}

			// VACUUM must run outside a transaction to reclaim disk space.
			if err := store.Vacuum(ctx); err != nil {
				return fmt.Errorf("running vacuum: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, gcOutput{
					DeletedIssuesRemoved: result.DeletedIssuesRemoved,
					ClosedIssuesRemoved:  result.ClosedIssuesRemoved,
				})
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			_, _ = fmt.Fprintf(w, "%s Garbage collection complete.\n", cs.SuccessIcon())
			_, _ = fmt.Fprintf(w, "  Deleted issues removed: %d\n", result.DeletedIssuesRemoved)
			if includeClosed {
				_, _ = fmt.Fprintf(w, "  Closed issues removed:  %d\n", result.ClosedIssuesRemoved)
			}

			return nil
		},
	}
}
