// Package upgrade implements the "admin upgrade" command, which migrates
// the nitpicking database from v1 to v2 schema. The migration converts
// claimed-state issues to open, removes obsolete history event types, and
// records the new schema version — all in a single atomic transaction.
package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// upgradeOutput is the JSON representation of the upgrade command result.
type upgradeOutput struct {
	// Status is either "up_to_date" (v2 already) or "migrated" (migration applied).
	Status string `json:"status"`

	// ClaimedIssuesConverted is the number of issues whose state was changed
	// from "claimed" to "open". Zero when Status is "up_to_date".
	ClaimedIssuesConverted int `json:"claimed_issues_converted"`

	// HistoryRowsRemoved is the number of history rows deleted because they
	// carried removed event types ("claimed" or "released"). Zero when Status
	// is "up_to_date".
	HistoryRowsRemoved int `json:"history_rows_removed"`
}

// RunInput holds all parameters for the upgrade command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	// Svc is the application service through which schema migration operations
	// are invoked. The upgrade command never imports a concrete storage adapter;
	// all migration capability is accessed via this driving port interface.
	Svc driving.Service

	// JSONOutput requests machine-readable JSON output when true.
	JSONOutput bool

	// Out receives the human-readable or JSON output.
	Out io.Writer

	// ColorScheme provides terminal colour helpers for human-readable output.
	ColorScheme *iostreams.ColorScheme
}

// Run executes the upgrade workflow: checks the schema version and, when the
// database is v1, applies the v1→v2 migration atomically. On a v2 database it
// reports "up to date" without making any changes.
func Run(ctx context.Context, input RunInput) error {
	// Fast path: database is already at v2.
	checkErr := input.Svc.CheckSchemaVersion(ctx)
	if checkErr == nil {
		return writeResult(input, upgradeOutput{Status: "up_to_date"})
	}

	// Only proceed with migration when the schema version is explicitly
	// outdated. Any other error (I/O failure, pool exhaustion, missing
	// migrator) is returned directly so the caller sees the real problem
	// instead of a misleading migration failure.
	if !errors.Is(checkErr, domain.ErrSchemaMigrationRequired) {
		return fmt.Errorf("checking schema version: %w", checkErr)
	}

	// Migrate v1 to v2.
	migResult, err := input.Svc.MigrateV1ToV2(ctx)
	if err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	return writeResult(input, upgradeOutput{
		Status:                 "migrated",
		ClaimedIssuesConverted: migResult.ClaimedIssuesConverted,
		HistoryRowsRemoved:     migResult.HistoryRowsRemoved,
	})
}

// writeResult emits the upgrade result as JSON or human-readable text.
func writeResult(input RunInput, out upgradeOutput) error {
	if input.JSONOutput {
		return cmdutil.WriteJSON(input.Out, out)
	}

	switch out.Status {
	case "up_to_date":
		_, err := fmt.Fprintf(input.Out, "%s Database is up to date\n", input.ColorScheme.SuccessIcon())
		return err
	default:
		_, err := fmt.Fprintf(input.Out,
			"%s Database migrated to v2 (claimed issues converted: %d, history rows removed: %d)\n",
			input.ColorScheme.SuccessIcon(),
			out.ClaimedIssuesConverted,
			out.HistoryRowsRemoved,
		)
		return err
	}
}

// NewCmd constructs the "admin upgrade" command, which checks the database
// schema version and applies the v1→v2 migration when needed. An optional
// runFn parameter replaces the default Run for testing; when injected, the
// service is not constructed and the runFn receives only the IOStreams fields
// of RunInput (Svc is nil).
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, RunInput) error) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "upgrade",
		Usage: "Check for and apply database schema upgrades",
		Description: `Checks whether the database schema is current and applies any pending
upgrades.

Currently this migrates v1 databases to v2: it converts any issues
whose primary state column was set to "claimed" back to "open" (the
claims table retains the active claim), removes history rows for event
types that no longer exist in v2 ("claimed" and "released"), and
records schema_version=2 in the metadata table. All changes are applied
in a single atomic transaction — the migration either fully succeeds or
leaves the database unchanged.

Run this after updating the np binary to a new version to ensure the
database schema matches the expectations of the new code.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			input := RunInput{
				JSONOutput:  jsonOutput,
				Out:         f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			}

			// When a test runFn is injected, skip service construction so tests
			// do not require a real database. The injected function takes full
			// responsibility for the Svc field.
			if len(runFn) > 0 {
				return runFn[0](ctx, input)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			input.Svc = svc
			return Run(ctx, input)
		},
	}
}
