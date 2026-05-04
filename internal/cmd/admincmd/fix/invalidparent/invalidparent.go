// Package invalidparent provides the "admin fix invalid-parent-reference"
// subcommand, which removes dangling parent references from issues whose
// parent no longer exists in the database. Each repaired issue becomes a
// top-level issue and receives an audit comment recording the original parent.
package invalidparent

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// service is the consuming interface for the repair operation. Defined here
// (in the consuming package) per hexagonal conventions so the command does not
// depend on the full driving.Service interface.
type service interface {
	// RepairInvalidParentReferences scans for dangling parent references and
	// either removes them (non-dry-run) or reports what would be removed
	// (dry-run). Author is recorded on audit comments.
	RepairInvalidParentReferences(ctx context.Context, input driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error)
}

// RunInput holds all parameters for the invalid-parent-reference fix logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	// Author is the name recorded on audit comments for each repaired issue.
	// The service validates the value; an empty author produces an error.
	Author string

	// DryRun, when true, identifies affected issues without mutating the
	// database. The service receives DryRun=true so no writes occur at any
	// layer.
	DryRun bool

	// JSON enables machine-readable JSON output instead of human-readable text.
	JSON bool

	// Out receives all command output.
	Out io.Writer

	// Svc is the service that performs the repair operation. NewCmd sets this
	// from the factory's store; tests inject a stub.
	Svc service
}

// fixedRecord is the JSON representation of a single repaired issue.
type fixedRecord struct {
	// Issue is the string ID of the issue whose parent was cleared.
	Issue string `json:"issue"`
	// RemovedParentID is the ID of the dangling parent that was removed.
	RemovedParentID string `json:"removed_parent_id"`
}

// wouldFixRecord is the JSON representation of a single issue that would be
// repaired in dry-run mode.
type wouldFixRecord struct {
	// Issue is the string ID of the affected issue.
	Issue string `json:"issue"`
	// MissingParentID is the ID of the parent that does not exist.
	MissingParentID string `json:"missing_parent_id"`
}

// successOutput is the JSON envelope for a successful (non-dry-run) repair.
type successOutput struct {
	// Fixed lists each repaired issue. The slice is non-nil so that the JSON
	// serialises as [] rather than null when no issues were affected.
	Fixed []fixedRecord `json:"fixed"`
	// Count is the number of issues repaired.
	Count int `json:"count"`
}

// dryRunOutput is the JSON envelope for a dry-run preview.
type dryRunOutput struct {
	// WouldFix lists each issue that would be repaired. Non-nil for the same
	// reason as successOutput.Fixed.
	WouldFix []wouldFixRecord `json:"would_fix"`
	// Count is the number of issues that would be repaired.
	Count int `json:"count"`
}

// Run executes the invalid-parent-reference fix. It calls the service to scan
// (and optionally repair) dangling parent references, then renders the result
// as text or JSON according to input.JSON and input.DryRun.
func Run(ctx context.Context, input RunInput) error {
	result, err := input.Svc.RepairInvalidParentReferences(ctx, driving.RepairInvalidParentsInput{
		Author: input.Author,
		DryRun: input.DryRun,
	})
	if err != nil {
		return fmt.Errorf("repairing invalid parent references: %w", err)
	}

	if input.DryRun {
		return renderDryRun(input.Out, result.Repaired, input.JSON)
	}
	return renderSuccess(input.Out, result.Repaired, input.JSON)
}

// renderSuccess writes the success output (text or JSON) for a completed
// non-dry-run repair. Both the success-with-items and no-op (zero items) cases
// are handled here.
func renderSuccess(w io.Writer, repaired []driving.RepairedParentRecord, jsonOutput bool) error {
	if jsonOutput {
		records := make([]fixedRecord, len(repaired))
		for i, r := range repaired {
			records[i] = fixedRecord{Issue: r.IssueID, RemovedParentID: r.RemovedParentID}
		}
		return cmdutil.WriteJSON(w, successOutput{Fixed: records, Count: len(records)})
	}

	if len(repaired) == 0 {
		_, err := fmt.Fprintln(w, "No invalid parent references found. Nothing to fix.")
		return err
	}

	_, err := fmt.Fprintln(w, "Removing dangling parent references...")
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintln(w); err != nil {
		return err
	}
	for _, r := range repaired {
		if _, err = fmt.Fprintf(w, "Cleaned %s (was → %s)\n", r.IssueID, r.RemovedParentID); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintln(w); err != nil {
		return err
	}
	noun := "issues"
	if len(repaired) == 1 {
		noun = "issue"
	}
	_, err = fmt.Fprintf(w, "%d %s fixed.\n", len(repaired), noun)
	return err
}

// renderDryRun writes the dry-run preview output (text or JSON). In text mode
// the output uses "Would clean" lines and the re-run hint; in JSON mode the
// "would_fix" key and "missing_parent_id" per-item key distinguish dry-run
// output from success output.
func renderDryRun(w io.Writer, repaired []driving.RepairedParentRecord, jsonOutput bool) error {
	if jsonOutput {
		records := make([]wouldFixRecord, len(repaired))
		for i, r := range repaired {
			records[i] = wouldFixRecord{Issue: r.IssueID, MissingParentID: r.RemovedParentID}
		}
		return cmdutil.WriteJSON(w, dryRunOutput{WouldFix: records, Count: len(records)})
	}

	if len(repaired) == 0 {
		_, err := fmt.Fprintln(w, "No invalid parent references found. Nothing to fix.")
		return err
	}

	for _, r := range repaired {
		if _, err := fmt.Fprintf(w, "Would clean %s (parent %s does not exist)\n", r.IssueID, r.RemovedParentID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	noun := "issues"
	if len(repaired) == 1 {
		noun = "issue"
	}
	_, err := fmt.Fprintf(w, "Would fix %d %s. Re-run without --dry-run to apply.\n", len(repaired), noun)
	return err
}

// NewCmd constructs the "admin fix invalid-parent-reference" subcommand. An
// optional runFn replaces Run for testing; when provided, the Factory is used
// only to resolve the database store for the Svc field.
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, RunInput) error) *cli.Command {
	var (
		author     string
		dryRun     bool
		jsonOutput bool
	)

	return &cli.Command{
		Name:  "invalid-parent-reference",
		Usage: "Remove dangling parent references from issues",
		Description: `Scans every non-deleted issue for a parent_id that refers to an absent
or soft-deleted parent. For each affected issue the parent reference is
cleared (making it a top-level issue) and an audit comment is recorded.

The fix is idempotent: re-running after a successful repair reports no
issues found. Use --dry-run to preview what would change before applying.

--author is required because every repair records an audit comment.

Exit codes: 0 success or no-op; 2 flag error or unrecoverable database error.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Usage:       "Author name recorded on audit comments for each repaired issue",
				Destination: &author,
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Sources:     cli.EnvVars("NP_AUTHOR"),
			},
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "Preview what would change without modifying the database",
				Destination: &dryRun,
				Category:    cmdutil.FlagCategorySupplemental,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
				Category:    cmdutil.FlagCategorySupplemental,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			input := RunInput{
				Author: author,
				DryRun: dryRun,
				JSON:   jsonOutput,
				Out:    f.IOStreams.Out,
			}
			if len(runFn) > 0 {
				return runFn[0](ctx, input)
			}

			store, err := f.Store()
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			input.Svc = core.New(store, store)
			return Run(ctx, input)
		},
	}
}
