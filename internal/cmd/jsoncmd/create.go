package jsoncmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// createInput is the JSON object read from stdin for the "json create"
// subcommand. It contains only content fields — the identity flag (--author)
// and the --with-claim flag remain on the command line.
type createInput struct {
	Role               string   `json:"role"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	Priority           string   `json:"priority"`
	Parent             string   `json:"parent"`
	Labels             []string `json:"labels"`
}

// createOutput is the JSON representation of a created domain.
type createOutput struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Title     string `json:"title"`
	Priority  string `json:"priority"`
	State     string `json:"state"`
	ClaimID   string `json:"claim_id,omitzero"`
	CreatedAt string `json:"created_at"`
}

// RunCreateInput holds the parameters for the json create operation, decoupled
// from CLI flag parsing so it can be tested directly.
type RunCreateInput struct {
	Service   driving.Service
	Author    string
	Stdin     io.Reader
	WriteTo   io.Writer
	WithClaim bool
	Deferred  bool
}

// RunCreate reads a JSON object from stdin, validates it, and creates an issue
// via the service layer. Output is always JSON.
//
// The role field defaults to "task" when omitted. Unknown fields are rejected
// by the JSON decoder.
//
// When Deferred is true, the issue is created with an internal claim, deferred
// using that claim, and the claim is consumed. The output reflects the deferred
// state with no claim_id. The --deferred and --with-claim flags are mutually
// exclusive.
func RunCreate(ctx context.Context, input RunCreateInput) error {
	if input.Deferred && input.WithClaim {
		return cmdutil.FlagErrorf("--deferred and --with-claim are mutually exclusive")
	}

	payload, err := DecodeStdin[createInput](input.Stdin)
	if err != nil {
		return fmt.Errorf("reading create JSON from stdin: %w", err)
	}

	// Default role to task when omitted.
	role := domain.RoleTask
	if payload.Role != "" {
		var roleErr error
		role, roleErr = domain.ParseRole(payload.Role)
		if roleErr != nil {
			return fmt.Errorf("invalid role %q: must be task or epic", payload.Role)
		}
	}

	if payload.Title == "" {
		return fmt.Errorf("\"title\" field is required and must be non-empty")
	}

	// Parse labels from key:value strings into service-layer DTOs.
	labels, err := cmdutil.ParseLabels(payload.Labels)
	if err != nil {
		return fmt.Errorf("invalid label: %w", err)
	}

	// Resolve the parent ID if provided.
	var parentIDStr string
	if payload.Parent != "" {
		resolver := cmdutil.NewIDResolver(input.Service)
		parentID, resolveErr := resolver.Resolve(ctx, payload.Parent)
		if resolveErr != nil {
			return fmt.Errorf("invalid parent ID: %w", resolveErr)
		}
		parentIDStr = parentID.String()
	}

	var priority domain.Priority
	if payload.Priority != "" {
		var priErr error
		priority, priErr = domain.ParsePriority(payload.Priority)
		if priErr != nil {
			return fmt.Errorf("invalid priority %q: %v", payload.Priority, priErr)
		}
	}

	// When --deferred is set, the issue must be created with a claim so that
	// it can be immediately deferred. The claim is consumed by the deferral.
	shouldClaim := input.WithClaim || input.Deferred

	result, err := input.Service.CreateIssue(ctx, driving.CreateIssueInput{
		Role:               role,
		Title:              payload.Title,
		Description:        payload.Description,
		AcceptanceCriteria: payload.AcceptanceCriteria,
		Priority:           priority,
		ParentID:           parentIDStr,
		Labels:             labels,
		Author:             input.Author,
		Claim:              shouldClaim,
	})
	if err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	// Defer the issue if requested, consuming the internal claim.
	if input.Deferred {
		if err := input.Service.DeferIssue(ctx, driving.DeferIssueInput{
			IssueID: result.Issue.ID().String(),
			ClaimID: result.ClaimID,
		}); err != nil {
			return fmt.Errorf("deferring newly created issue: %w", err)
		}
	}

	t := result.Issue
	out := createOutput{
		ID:        t.ID().String(),
		Role:      t.Role().String(),
		Title:     t.Title(),
		Priority:  t.Priority().String(),
		CreatedAt: cmdutil.FormatJSONTimestamp(t.CreatedAt()),
	}

	// Reflect the correct state: deferred if the flag was set, otherwise the
	// state from the create result. Only include the claim_id when
	// --with-claim was explicitly requested (not the internal claim used for
	// deferral).
	if input.Deferred {
		out.State = domain.StateDeferred.String()
	} else {
		out.State = t.State().String()
		out.ClaimID = result.ClaimID
	}

	return cmdutil.WriteJSON(input.WriteTo, out)
}

// newCreateCmd constructs the "json create" subcommand, which creates an issue
// using structured JSON input from stdin. The --author flag identifies the
// actor; the --with-claim flag controls whether the new issue is immediately
// claimed. The JSON object on stdin provides all content fields (role, title,
// description, etc.).
//
// Output is always JSON — there is no --json flag.
func newCreateCmd(f *cmdutil.Factory) *cli.Command {
	var (
		author    string
		withClaim bool
		deferred  bool
	)

	return &cli.Command{
		Name:  "create",
		Usage: "Create an issue (JSON stdin)",
		Description: `Creates a new issue from a JSON object piped to stdin. The JSON object must
include "title" at minimum. The "role" field defaults to "task" when omitted.
Optional fields include description, acceptance_criteria, priority, parent
(issue ID), and labels (array of "key:value" strings). Unknown fields are
rejected.

Use --with-claim to immediately claim the new issue. The output will include
the claim_id.

Use --deferred to create the issue in the deferred state. This internally
claims the issue, defers it, and consumes the claim in a single operation.
The output reflects state "deferred" with no claim_id.

The --deferred and --with-claim flags are mutually exclusive.

The --author flag identifies who is creating the issue. Output is a JSON
object containing the new issue's ID, role, title, priority, state, and
creation timestamp. If --with-claim was set, the output also includes a
claim_id. This is the primary command for agents and scripts that need to
create issues programmatically.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &author,
			},
			&cli.BoolFlag{
				Name:        "with-claim",
				Usage:       "Immediately claim the new issue",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &withClaim,
			},
			&cli.BoolFlag{
				Name:        "deferred",
				Usage:       "Create the issue in the deferred state",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &deferred,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return RunCreate(ctx, RunCreateInput{
				Service:   svc,
				Author:    author,
				Stdin:     f.IOStreams.In,
				WriteTo:   f.IOStreams.Out,
				WithClaim: withClaim,
				Deferred:  deferred,
			})
		},
	}
}
