package init

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// initOutput is the JSON representation of the init command result.
type initOutput struct {
	Prefix string `json:"prefix"`
}

// RunInput holds the parameters for the init command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service     driving.Service
	Prefix      string
	JSON        bool
	WriteTo     io.Writer
	ColorScheme *iostreams.ColorScheme
}

// Run executes the init workflow: validates the prefix, initializes the
// database, and writes the result to the output writer.
func Run(ctx context.Context, input RunInput) error {
	prefix := strings.TrimSpace(input.Prefix)
	if prefix == "" {
		return cmdutil.FlagErrorf("prefix argument is required")
	}

	if err := input.Service.Init(ctx, prefix); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, initOutput{Prefix: prefix})
	}

	cs := input.ColorScheme
	_, err := fmt.Fprintf(input.WriteTo, "%s Initialized database with prefix %s\n",
		cs.SuccessIcon(), cs.Bold(prefix))
	return err
}

// NewCmd constructs the "init" command, which creates a new database with the
// given issue ID prefix.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "init",
		Usage:     "Initialize a new nitpicking workspace rooted at the current directory",
		ArgsUsage: "<PREFIX>",
		Description: `Creates a new .np/ directory in the current working directory, containing a
fresh SQLite database configured with the given issue ID prefix. All issues
created in this workspace will have IDs of the form "<PREFIX>-<random>" —
for example, "init PKHP" produces issue IDs like PKHP-a3bxr.

Run this once per project root. The np tool discovers its database by
walking up from the current directory looking for .np/, so every
subdirectory of the initialized root is automatically part of the workspace.
Initializing inside a directory that is already within an np workspace is an
error — np prevents nested workspaces to avoid shadowing.

Choose a short, memorable prefix (2–6 uppercase characters) that identifies
the project. The prefix is permanent for the lifetime of the workspace.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Check whether the current directory is already inside an
			// np-tracked tree. Creating a nested .np/ would shadow the
			// ancestor and confuse database discovery.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			existingDB, discoverErr := sqlite.DiscoverDatabase(cwd)
			if discoverErr == nil {
				npDir := filepath.Dir(existingDB)
				return cmdutil.FlagErrorf(
					"an existing np database was found at %s — "+
						"run commands from that directory or a subdirectory instead of initializing a new one",
					npDir)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return Run(ctx, RunInput{
				Service:     svc,
				Prefix:      cmd.Args().Get(0),
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			})
		},
	}
}
