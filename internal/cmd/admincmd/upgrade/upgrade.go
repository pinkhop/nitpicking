// Package upgrade implements the "admin upgrade" command, which migrates
// the nitpicking database from its current schema version to v3. The command
// chains the v1→v2 and v2→v3 migrations in sequence so that a single
// invocation carries a database from any supported source version to the
// current version.
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
	// Status is either "up_to_date" (already at v3) or "migrated" (one or more
	// migration steps were applied). When "migrated", the counter fields report
	// the number of rows affected by each step; fields for steps that were
	// skipped (e.g., v1→v2 counters on a v2 database) are zero.
	Status string `json:"status"`

	// ClaimedIssuesConverted is the number of issues whose state was changed
	// from "claimed" to "open" during the v1→v2 migration step. Zero when the
	// database was already at v2 or v3.
	ClaimedIssuesConverted int `json:"claimed_issues_converted"`

	// HistoryRowsRemoved is the number of history rows deleted because they
	// carried removed event types ("claimed" or "released") during v1→v2.
	// Zero when the database was already at v2 or v3.
	HistoryRowsRemoved int `json:"history_rows_removed"`

	// LegacyRelationshipsTranslated is the number of relationship rows that
	// were translated or removed during the v1→v2 migration step: "cites" rows
	// are renamed to "refs" and "cited_by" rows are dropped. Zero when the
	// database was already at v2 or v3.
	LegacyRelationshipsTranslated int `json:"legacy_relationships_translated"`

	// IdempotencyKeysMigrated is the number of non-NULL idempotency_key column
	// values successfully written as idempotency:<value> label rows during the
	// v2→v3 migration step. Zero when the database was already at v3 or had no
	// rows with a non-NULL idempotency_key.
	IdempotencyKeysMigrated int `json:"idempotency_keys_migrated"`

	// IdempotencyKeysSkipped is the number of idempotency_key column values not
	// written during v2→v3 because the issue already carried an idempotency
	// label (skip-on-conflict policy). Zero when no collisions occurred.
	IdempotencyKeysSkipped int `json:"idempotency_keys_skipped"`

	// InvalidLabelValuesSkipped is the number of idempotency_key column values
	// dropped during v2→v3 because they failed domain.NewLabel validation (for
	// example, values containing control characters or whitespace that were
	// tolerated by the legacy column but rejected by the label value rules).
	// Surfaced in the upgrade result so operators can detect when a migration
	// silently discarded dedupe metadata for some rows. Zero when every legacy
	// value was a valid label value.
	InvalidLabelValuesSkipped int `json:"invalid_label_values_skipped"`
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
// database is below v3, applies the pending migrations in sequence. A v1
// database receives both the v1→v2 and v2→v3 steps; a v2 database receives
// only the v2→v3 step; a v3 database is reported as up to date immediately.
//
// Both migration functions are safe to call on an already-migrated database
// (v1→v2 is a no-op on a v2 database because the SQL changes are idempotent,
// and v2→v3 has an explicit PRAGMA table_info guard). The chained call
// therefore always applies the v1→v2 step first, followed by v2→v3, so that a
// v1 database reaches v3 in a single invocation without requiring the caller
// to know the source version.
func Run(ctx context.Context, input RunInput) error {
	// Fast path: database is already at v3 — no migration needed.
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

	// Migrate v1→v2. This is a no-op if the database is already at v2 because
	// the SQL statements (UPDATE issues, DELETE history, translate rels) match
	// zero rows, and SetSchemaVersion(2) is idempotent.
	v12Result, err := input.Svc.MigrateV1ToV2(ctx)
	if err != nil {
		return fmt.Errorf("migrating v1→v2: %w", err)
	}

	// Migrate v2→v3. MigrateV2ToV3 contains a PRAGMA table_info guard that
	// skips the idempotency_key carry-forward and DROP COLUMN steps when the
	// column no longer exists, so calling it on an already-v3 database is safe.
	v23Result, err := input.Svc.MigrateV2ToV3(ctx)
	if err != nil {
		return fmt.Errorf("migrating v2→v3: %w", err)
	}

	return writeResult(input, upgradeOutput{
		Status:                        "migrated",
		ClaimedIssuesConverted:        v12Result.ClaimedIssuesConverted,
		HistoryRowsRemoved:            v12Result.HistoryRowsRemoved,
		LegacyRelationshipsTranslated: v12Result.LegacyRelationshipsTranslated,
		IdempotencyKeysMigrated:       v23Result.IdempotencyKeysMigrated,
		IdempotencyKeysSkipped:        v23Result.IdempotencyKeysSkipped,
		InvalidLabelValuesSkipped:     v23Result.InvalidLabelValuesSkipped,
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
		// Build a human-readable summary that mentions only the migration steps
		// that produced non-zero counts, to avoid confusing output when a v2
		// database is upgraded (v1→v2 counters are all zero).
		_, err := fmt.Fprintf(input.Out,
			"%s Database migrated to v3 (claimed issues converted: %d, history rows removed: %d, legacy relationships translated: %d, idempotency keys migrated: %d, idempotency keys skipped: %d, invalid label values skipped: %d)\n",
			input.ColorScheme.SuccessIcon(),
			out.ClaimedIssuesConverted,
			out.HistoryRowsRemoved,
			out.LegacyRelationshipsTranslated,
			out.IdempotencyKeysMigrated,
			out.IdempotencyKeysSkipped,
			out.InvalidLabelValuesSkipped,
		)
		return err
	}
}

// NewCmd constructs the "admin upgrade" command, which checks the database
// schema version and applies any pending migrations to bring it to v3. An
// optional runFn parameter replaces the default Run for testing; when injected,
// the service is not constructed and the runFn receives only the IOStreams
// fields of RunInput (Svc is nil).
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, RunInput) error) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "upgrade",
		Usage: "Check for and apply database schema upgrades",
		Description: `Checks whether the database schema is current and applies any pending
upgrades to bring it to v3 (the current version).

The command chains migrations in sequence so that a single invocation carries
a database from any supported source version to v3:

  v1 → v2 → v3: converts "claimed" issue states back to "open", removes
    obsolete history event types, translates legacy v0.2.0 relationship names,
    then carries non-NULL idempotency_key column values forward as
    idempotency:<value> label rows and drops the column.

  v2 → v3: carries non-NULL idempotency_key column values forward as
    idempotency:<value> label rows and drops the idempotency_key column and
    its unique partial index.

  v3 (current): reports "up to date" without making any changes.

Each migration step runs in a single atomic transaction — the entire upgrade
either fully succeeds or leaves the database unchanged.

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
