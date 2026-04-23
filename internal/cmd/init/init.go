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
			//
			// The guard operates at the database file level rather than the
			// .np/ directory level:
			//
			//   - .np/ absent → fresh workspace; init creates both the
			//     directory and the database via f.Store()'s auto-create path.
			//
			//   - .np/ present, no nitpicking.db → partially created workspace
			//     (e.g. user created the dir manually); init creates the
			//     database file explicitly before wiring the service.
			//
			//   - .np/nitpicking.db present and non-empty → workspace is
			//     already initialised; return the "already initialized" error.
			//
			//   - .np/nitpicking.db present but empty (size == 0) → the file
			//     is corrupt or the result of a partial/interrupted init;
			//     return a user-friendly error directing the user to remove
			//     the file or run np admin doctor.
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
			existingDB, discoverErr := sqlite.DiscoverDatabase(cwd)
			if discoverErr == nil {
				// DiscoverDatabase found a .np/ directory. Check whether
				// the database file itself exists and has content.
				info, statErr := os.Stat(existingDB)
				if statErr == nil {
					// Database file is present and accessible.
					if info.Size() == 0 {
						// File exists but is empty — it is either the result of
						// a partial previous init or was created manually. This
						// is not a valid database; direct the user to remediate.
						return cmdutil.FlagErrorf(
							"database file exists at %s but is empty or corrupt — "+
								"remove the file and run 'np init' again, or run 'np admin doctor' for diagnostics",
							existingDB)
					}
					npDir := filepath.Dir(existingDB)
					return cmdutil.FlagErrorf(
						"an existing np database was found at %s — "+
							"run commands from that directory or a subdirectory instead of initializing a new one",
						npDir)
				}
				if !os.IsNotExist(statErr) {
					// Stat returned an error other than "not found" —
					// treat this as a potential access problem.
					return fmt.Errorf("checking existing database: %w", statErr)
				}

				// .np/ exists but no database file. f.Store() returns
				// ErrDatabaseNotInitialized for this state, so create the
				// schema here before NewTracker opens the file.
				created, createErr := sqlite.Create(existingDB)
				if createErr != nil {
					return fmt.Errorf("creating database: %w", createErr)
				}
				if closeErr := created.Close(); closeErr != nil {
					return fmt.Errorf("closing initial database connection: %w", closeErr)
				}
			}

			// For the "no .np/ directory" case f.Store() auto-creates both the
			// directory and the database, so NewTracker works without additional
			// setup here.
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
