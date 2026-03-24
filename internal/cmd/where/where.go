// Package where provides the "where" command, which prints the absolute path
// of the discovered .np/ directory.
package where

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

// NewCmd constructs the "where" command which prints the path of the .np/
// directory discovered by walking up from the current working directory.
// Useful for scripting and debugging.
func NewCmd(_ *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "where",
		Usage: "Print the path of the .np/ directory",
		Action: func(_ context.Context, cmd *cli.Command) error {
			dbPath, err := sqlite.DiscoverDatabase(".")
			if err != nil {
				return fmt.Errorf("no .np/ directory found: %w", err)
			}

			// DiscoverDatabase returns the full DB file path (e.g. /foo/.np/np.db).
			// We want just the .np/ directory.
			npDir := filepath.Dir(dbPath)
			fmt.Println(npDir)
			return nil
		},
	}
}
