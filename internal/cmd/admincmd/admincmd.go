// Package admincmd provides the "admin" parent command, which groups
// maintenance and administrative operations under a single namespace.
package admincmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/doctor"
	"github.com/pinkhop/nitpicking/internal/cmd/gc"
	"github.com/pinkhop/nitpicking/internal/cmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

// NewCmd constructs the "admin" parent command with maintenance subcommands.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "admin",
		Usage: "Administrative and maintenance commands",
		Commands: []*cli.Command{
			doctor.NewCmd(f),
			gc.NewCmd(f),
			graphcmd.NewCmd(f),
			newResetCmd(f),
			newUpgradeCmd(f),
		},
	}
}

// newResetCmd constructs "admin reset" which deletes the .np/ database
// directory, requiring a fresh "np init" afterwards. This is a destructive
// operation that requires --confirm.
func newResetCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		confirm    bool
	)

	return &cli.Command{
		Name:  "reset",
		Usage: "Delete the .np/ database (destructive — requires --confirm)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "confirm",
				Usage:       "Confirm the reset (required)",
				Category:    "Options",
				Destination: &confirm,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if !confirm {
				return cmdutil.FlagErrorf("--confirm is required to reset the database")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			dbPath, err := sqlite.DiscoverDatabase(cwd)
			if err != nil {
				return fmt.Errorf("no database found: %w", err)
			}

			// The database lives in .np/nitpicking.db — remove the .np/ directory.
			npDir := filepath.Dir(dbPath)
			if err := os.RemoveAll(npDir); err != nil {
				return fmt.Errorf("removing database directory: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"action": "reset",
					"path":   npDir,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed %s — run %s to reinitialize\n",
				cs.SuccessIcon(), npDir, cs.Cyan("np init <PREFIX>"))
			return err
		},
	}
}

// newUpgradeCmd constructs "admin upgrade" which checks for and applies
// database schema upgrades. Currently the schema has no versioning, so
// this always reports the database is up to date.
func newUpgradeCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "upgrade",
		Usage: "Check for and apply database schema upgrades",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Verify the database exists and is accessible.
			_, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"status": "up_to_date",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Database is up to date\n",
				cs.SuccessIcon())
			return err
		},
	}
}
