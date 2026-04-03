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
	Until   string `json:"until,omitzero"`
}

// DeferInput holds the parameters for the defer operation, decoupled from CLI
// flag parsing so it can be tested directly.
type DeferInput struct {
	Service driving.Service
	IssueID string
	ClaimID string
	Until   string
	JSON    bool
	WriteTo io.Writer
}

// Defer shelves a claimed issue for later. The label mutation and state
// transition are delegated to a single atomic service call so the CLI adapter
// does not orchestrate ordering invariants.
func Defer(ctx context.Context, input DeferInput) error {
	deferInput := driving.DeferIssueInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
		Until:   input.Until,
	}
	if err := input.Service.DeferIssue(ctx, deferInput); err != nil {
		return fmt.Errorf("deferring issue: %w", err)
	}

	if input.JSON {
		out := deferOutput{
			IssueID: input.IssueID,
			Action:  "defer",
			Until:   input.Until,
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	if input.Until != "" {
		_, err := fmt.Fprintf(input.WriteTo, "Deferred %s until %s\n",
			input.IssueID, input.Until)
		return err
	}

	_, err := fmt.Fprintf(input.WriteTo, "Deferred %s\n", input.IssueID)
	return err
}
