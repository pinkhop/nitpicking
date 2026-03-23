package init

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// initOutput is the JSON representation of the init command result.
type initOutput struct {
	Prefix string `json:"prefix"`
}

// NewCmd constructs the "init" command, which creates a new database with the
// given ticket ID prefix.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "init",
		Usage:     "Initialize a new nitpicking database in the current directory",
		ArgsUsage: "<PREFIX>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			prefix := strings.TrimSpace(cmd.Args().Get(0))
			if prefix == "" {
				return cmdutil.FlagErrorf("prefix argument is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			if err := svc.Init(ctx, prefix); err != nil {
				return fmt.Errorf("initializing database: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, initOutput{Prefix: prefix})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Initialized database with prefix %s\n",
				cs.SuccessIcon(), cs.Bold(prefix))
			return err
		},
	}
}
