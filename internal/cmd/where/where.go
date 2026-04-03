// Package where provides the "where" command, which prints the absolute path
// of the discovered .np/ directory.
package where

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// whereOutput is the JSON representation of the where command result.
type whereOutput struct {
	Path string `json:"path"`
}

// RunInput holds the parameters for the where command's core logic, decoupled
// from CLI flag parsing so it can be tested directly. The DiscoverFunc allows
// tests to inject a stub instead of hitting the real filesystem.
type RunInput struct {
	// DiscoverFunc locates the database file, returning its absolute path.
	// In production this is Factory.DatabasePath (workspace-aware); tests
	// provide a stub.
	DiscoverFunc func() (string, error)

	// JSON enables machine-readable JSON output.
	JSON bool

	// WriteTo receives the output.
	WriteTo io.Writer
}

// Run executes the where workflow: discovers the .np/ directory and prints
// its path.
func Run(_ context.Context, input RunInput) error {
	dbPath, err := input.DiscoverFunc()
	if err != nil {
		return fmt.Errorf("no .np/ directory found: %w", err)
	}

	// DiscoverDatabase returns the full DB file path (e.g. /foo/.np/np.db).
	// We want just the .np/ directory.
	npDir := filepath.Dir(dbPath)

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, whereOutput{Path: npDir})
	}

	_, err = fmt.Fprintln(input.WriteTo, npDir)
	return err
}

// NewCmd constructs the "where" command which prints the path of the .np/
// directory discovered by walking up from the current working directory.
// Useful for scripting and debugging.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "where",
		Usage: "Print the path of the .np/ directory",
		Description: `Prints the absolute path of the .np/ directory that np discovered by
walking up from the current working directory. This is the same
discovery logic every other np command uses, so "admin where" tells
you exactly which database np would operate on.

Use this for scripting (e.g., to locate backup files or the database),
for debugging when you have multiple .np/ directories in nested
projects, or simply to confirm that np has found the right database
before running a destructive command.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			return Run(ctx, RunInput{
				DiscoverFunc: f.DatabasePath,
				JSON:         jsonOutput,
				WriteTo:      f.IOStreams.Out,
			})
		},
	}
}
