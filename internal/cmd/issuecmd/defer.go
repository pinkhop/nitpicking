package issuecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// deferOutput is the JSON representation of a defer result.
type deferOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// DeferInput holds the parameters for the defer operation, decoupled from CLI
// flag parsing so it can be tested directly.
type DeferInput struct {
	Service driving.Service
	IssueID string
	ClaimID string
	JSON    bool
	WriteTo io.Writer
}

// Defer shelves a claimed issue for later. The state transition is delegated
// to a single atomic service call so the CLI adapter does not orchestrate
// ordering invariants.
func Defer(ctx context.Context, input DeferInput) error {
	deferInput := driving.DeferIssueInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
	}
	if err := input.Service.DeferIssue(ctx, deferInput); err != nil {
		return fmt.Errorf("deferring issue: %w", err)
	}

	if input.JSON {
		out := deferOutput{
			IssueID: input.IssueID,
			Action:  "defer",
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	_, err := fmt.Fprintf(input.WriteTo, "Deferred %s\n", input.IssueID)
	return err
}
