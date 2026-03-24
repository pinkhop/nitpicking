package issuecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// reopenOutput is the JSON representation of the reopen result.
type reopenOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// ReopenInput holds the parameters for the reopen operation, decoupled from
// CLI flag parsing so it can be tested directly.
type ReopenInput struct {
	Service service.Service
	IssueID issue.ID
	Author  identity.Author
	JSON    bool
	WriteTo io.Writer
}

// Reopen transitions a closed or deferred issue back to the open state. It
// claims the issue and immediately releases it (→ open), making the issue
// available for work again. Returns an error if the issue is already open or
// claimed.
func Reopen(ctx context.Context, input ReopenInput) error {
	// Verify the issue is in a reopenable state.
	shown, err := input.Service.ShowIssue(ctx, input.IssueID)
	if err != nil {
		return fmt.Errorf("looking up issue: %w", err)
	}
	state := shown.Issue.State()
	if state != issue.StateClosed && state != issue.StateDeferred {
		return fmt.Errorf("issue %s is %s: only closed or deferred issues can be reopened",
			input.IssueID, state)
	}

	// Step 1: Claim the issue.
	claimOut, err := input.Service.ClaimByID(ctx, service.ClaimInput{
		IssueID: input.IssueID,
		Author:  input.Author,
	})
	if err != nil {
		return fmt.Errorf("claiming issue: %w", err)
	}

	// Step 2: Release immediately to return to open.
	err = input.Service.TransitionState(ctx, service.TransitionInput{
		IssueID: input.IssueID,
		ClaimID: claimOut.ClaimID,
		Action:  service.ActionRelease,
	})
	if err != nil {
		return fmt.Errorf("releasing issue to open: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, reopenOutput{
			IssueID: input.IssueID.String(),
			Action:  "reopen",
		})
	}

	_, err = fmt.Fprintf(input.WriteTo, "Reopened %s\n", input.IssueID)
	return err
}
