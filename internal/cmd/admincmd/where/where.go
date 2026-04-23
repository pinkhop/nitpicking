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
	// Path is the absolute path to the discovered .np/ directory.
	Path string `json:"path"`

	// Prefix is the database's configured issue ID prefix. Omitted from JSON
	// when the prefix cannot be determined (e.g., the database has not yet been
	// initialised or a pending schema migration prevents reading it).
	Prefix string `json:"prefix,omitempty"`
}

// RunInput holds the parameters for the where command's core logic, decoupled
// from CLI flag parsing so it can be tested directly. The DiscoverFunc allows
// tests to inject a stub instead of hitting the real filesystem.
type RunInput struct {
	// DiscoverFunc locates the database file, returning its absolute path.
	// In production this is Factory.DatabasePath (workspace-aware); tests
	// provide a stub.
	DiscoverFunc func() (string, error)

	// PrefixFunc returns the database's configured issue ID prefix. When nil
	// or when the function returns an error, the prefix is treated as
	// unavailable and silently omitted from the output. The command still
	// succeeds in that case.
	PrefixFunc func(context.Context) (string, error)

	// JSON enables machine-readable JSON output.
	JSON bool

	// WriteTo receives the output.
	WriteTo io.Writer
}

// Run executes the where workflow: discovers the .np/ directory, optionally
// resolves the issue prefix, and prints both. When the prefix is unavailable
// (PrefixFunc is nil or returns an error), the command succeeds and simply
// omits the prefix from the output.
func Run(ctx context.Context, input RunInput) error {
	dbPath, err := input.DiscoverFunc()
	if err != nil {
		return fmt.Errorf("no .np/ directory found: %w", err)
	}

	// DiscoverDatabase returns the full DB file path (e.g. /foo/.np/np.db).
	// We want just the .np/ directory.
	npDir := filepath.Dir(dbPath)

	// Resolve the prefix. A missing or erroring PrefixFunc is treated as
	// "prefix unavailable" — the command still succeeds.
	var prefix string
	if input.PrefixFunc != nil {
		p, prefixErr := input.PrefixFunc(ctx)
		if prefixErr == nil {
			prefix = p
		}
		// Silently ignore prefixErr: an unavailable prefix is not a fatal
		// condition for this informational command.
	}

	if input.JSON {
		out := whereOutput{Path: npDir, Prefix: prefix}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	if prefix != "" {
		_, err = fmt.Fprintf(input.WriteTo, "%s (prefix: %s)\n", npDir, prefix)
	} else {
		_, err = fmt.Fprintln(input.WriteTo, npDir)
	}
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

When the database has been initialised, the output also includes the
issue ID prefix configured at init time (e.g. PKHP). The prefix is
omitted when it cannot be determined, such as before the database has
been initialised or when a schema migration is pending.

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
			// Attempt to resolve the prefix from the database. A failure here
			// is not fatal — the store may be unavailable (pre-migration schema,
			// uninitialised database) and the command must still succeed. The
			// PrefixFunc wrapper below absorbs any service-construction or
			// prefix-lookup errors so that Run can silently omit the prefix.
			//
			// The DatabasePath guard prevents NewTracker from being called when
			// no .np/ directory exists. Without it, f.Store() would create the
			// directory and a database as a side effect — inappropriate for a
			// read-only diagnostic command.
			var prefixFunc func(context.Context) (string, error)
			if _, pathErr := f.DatabasePath(); pathErr == nil {
				if svc, svcErr := cmdutil.NewTracker(f); svcErr == nil {
					prefixFunc = svc.GetPrefix
				}
			}
			return Run(ctx, RunInput{
				DiscoverFunc: f.DatabasePath,
				PrefixFunc:   prefixFunc,
				JSON:         jsonOutput,
				WriteTo:      f.IOStreams.Out,
			})
		},
	}
}
