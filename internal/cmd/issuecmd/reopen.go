package issuecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// reopenOutput is the JSON representation of the reopen result.
type reopenOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// ReopenInput holds the parameters for the reopen operation, decoupled from
// CLI flag parsing so it can be tested directly.
type ReopenInput struct {
	Service driving.Service
	IssueID string
	Author  string
	JSON    bool
	WriteTo io.Writer
}

// Reopen transitions a closed or deferred issue back to the open state. The
// service handles validation, claim lifecycle, and history recording
// atomically.
func Reopen(ctx context.Context, input ReopenInput) error {
	err := input.Service.ReopenIssue(ctx, driving.ReopenInput{
		IssueID: input.IssueID,
		Author:  input.Author,
	})
	if err != nil {
		return fmt.Errorf("reopening issue: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, reopenOutput{
			IssueID: input.IssueID,
			Action:  "reopen",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Reopened %s\n", input.IssueID)
	return err
}
