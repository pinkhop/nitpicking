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
	DeletedTicketsRemoved int `json:"deleted_tickets_removed"`
	ClosedTicketsRemoved  int `json:"closed_tickets_removed"`
}

// NewCmd constructs the "gc" command, which physically removes soft-deleted
// ticket data and optionally closed ticket data from the database.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		confirm       bool
		includeClosed bool
	)

	return &cli.Command{
		Name:  "gc",
		Usage: "Garbage-collect deleted (and optionally closed) tickets",
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
				Usage:       "Also remove closed tickets",
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

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.GC(ctx, input)
			if err != nil {
				return fmt.Errorf("running garbage collection: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, gcOutput{
					DeletedTicketsRemoved: result.DeletedTicketsRemoved,
					ClosedTicketsRemoved:  result.ClosedTicketsRemoved,
				})
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			_, _ = fmt.Fprintf(w, "%s Garbage collection complete.\n", cs.SuccessIcon())
			_, _ = fmt.Fprintf(w, "  Deleted tickets removed: %d\n", result.DeletedTicketsRemoved)
			if includeClosed {
				_, _ = fmt.Fprintf(w, "  Closed tickets removed:  %d\n", result.ClosedTicketsRemoved)
			}

			return nil
		},
	}
}
