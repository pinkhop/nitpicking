package issuecmd

import (
	"context"
	"fmt"
	"io"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// releaseOutput is the JSON representation of a release result.
type releaseOutput struct {
	IssueID string `json:"issue_id"`
	Action  string `json:"action"`
}

// ReleaseInput holds the parameters for the release operation, decoupled from
// CLI flag parsing so it can be tested directly.
type ReleaseInput struct {
	Service driving.Service
	IssueID string
	ClaimID string
	JSON    bool
	WriteTo io.Writer
}

// Release returns a claimed issue to its default unclaimed state without
// closing it, making the issue available for other agents to claim.
func Release(ctx context.Context, input ReleaseInput) error {
	transInput := driving.TransitionInput{
		IssueID: input.IssueID,
		ClaimID: input.ClaimID,
		Action:  driving.ActionRelease,
	}
	if err := input.Service.TransitionState(ctx, transInput); err != nil {
		return fmt.Errorf("releasing issue: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, releaseOutput{
			IssueID: input.IssueID,
			Action:  "release",
		})
	}

	_, err := fmt.Fprintf(input.WriteTo, "Released %s\n", input.IssueID)
	return err
}
