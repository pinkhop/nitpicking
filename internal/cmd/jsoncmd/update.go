package jsoncmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// patchString is a JSON field type that distinguishes three states:
//   - Absent from the JSON object: Present is false, Value is nil → no change.
//   - Explicitly set to null: Present is true, Value is nil → unset/clear.
//   - Set to a string value: Present is true, Value is non-nil → update.
//
// This enables JSON PATCH semantics where missing fields are left untouched and
// null fields are cleared.
type patchString struct {
	// Present indicates whether the field appeared in the JSON object at all,
	// regardless of whether its value was null or a string.
	Present bool

	// Value holds the decoded string when the field was present and non-null.
	// A nil Value with Present=true means the field was explicitly set to null.
	Value *string
}

// UnmarshalJSON implements json.Unmarshaler. It sets Present to true whenever
// the field appears in the JSON object. A JSON null yields Value=nil; a JSON
// string yields Value pointing to the decoded string.
func (p *patchString) UnmarshalJSON(data []byte) error {
	p.Present = true

	if string(data) == "null" {
		p.Value = nil
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	p.Value = &s
	return nil
}

// updateInput is the JSON object read from stdin for the "json update"
// subcommand. It contains only content fields — the claim ID is provided on
// the command line.
//
// Scalar string fields use patchString to support three-state semantics:
// absent (no change), null (unset/clear), and present-with-value (update).
// Array fields use standard slices where nil means no change.
//
// Fields shared with createInput (role, claim) are accepted silently so that
// the same JSON object can be piped to both json create and json update
// without triggering DisallowUnknownFields.
type updateInput struct {
	Title              patchString `json:"title"`
	Description        patchString `json:"description"`
	AcceptanceCriteria patchString `json:"acceptance_criteria"`
	Priority           patchString `json:"priority"`
	Parent             patchString `json:"parent"`
	Labels             []string    `json:"labels"`
	LabelRemove        []string    `json:"label_remove"`
	Comment            string      `json:"comment"`

	// Role is accepted for schema compatibility with json create but validated:
	// if present and different from the issue's current role, an error is
	// returned.
	Role string `json:"role"`

	// Claim is accepted for schema compatibility with json create but silently
	// ignored. Claiming is managed through the --claim CLI flag.
	Claim bool `json:"claim"`

	// State is accepted in JSON so that callers receive a clear error message
	// rather than a generic "unknown field" rejection. State transitions are
	// managed through dedicated lifecycle commands (claim, close, defer, etc.)
	// and cannot be set via json update.
	State string `json:"state"`
}

// updateOutput is the JSON representation of the update command result.
type updateOutput struct {
	IssueID string `json:"issue_id"`
	Updated bool   `json:"updated"`
}

// RunUpdateInput holds the parameters for the json update operation, decoupled
// from CLI flag parsing so it can be tested directly.
type RunUpdateInput struct {
	Service driving.Service
	ClaimID string
	Stdin   io.Reader
	WriteTo io.Writer
}

// RunUpdate reads a JSON object from stdin, translates it into a
// driving.UpdateIssueInput, and calls the service directly. The issue ID is
// resolved from the claim ID — no explicit issue ID is needed.
//
// Field semantics follow JSON PATCH conventions:
//   - Absent fields: no change.
//   - Null fields: unset/clear the value.
//   - Present fields: update to the provided value.
//
// The role and claim fields are accepted for schema compatibility with json
// create. If role is present and differs from the issue's current role, an
// error is returned. The claim field is silently ignored. The state field is
// accepted so callers receive a clear error message; passing any state value
// (including "claimed") is rejected with an explicit error because state
// transitions are managed through dedicated lifecycle commands.
//
// Output is always JSON.
func RunUpdate(ctx context.Context, input RunUpdateInput) error {
	payload, err := DecodeStdin[updateInput](input.Stdin)
	if err != nil {
		return fmt.Errorf("reading update JSON from stdin: %w", err)
	}

	// Reject any attempt to set state directly. State transitions — including
	// claiming — are managed through dedicated lifecycle commands. Accepting the
	// field with an explicit error produces a clearer message than the generic
	// "unknown field" rejection that would result if the field were absent from
	// the schema.
	if payload.State != "" {
		return fmt.Errorf("\"state\" is not a writable field: state transitions are managed through dedicated commands (claim, close, defer, release); \"claimed\" is not a valid primary state")
	}

	// Resolve the issue ID from the claim.
	issueIDStr, err := input.Service.LookupClaimIssueID(ctx, input.ClaimID)
	if err != nil {
		return fmt.Errorf("looking up claim: %w", err)
	}

	// Validate role if provided: it must match the issue's current role.
	if payload.Role != "" {
		shown, showErr := input.Service.ShowIssue(ctx, issueIDStr)
		if showErr != nil {
			return fmt.Errorf("looking up issue for role validation: %w", showErr)
		}
		payloadRole, parseErr := domain.ParseRole(payload.Role)
		if parseErr != nil {
			return fmt.Errorf("invalid role %q: must be task or epic", payload.Role)
		}
		if payloadRole != shown.Role {
			return fmt.Errorf("role mismatch: issue is %s but input specifies %s", shown.Role, payloadRole)
		}
	}

	svcInput := driving.UpdateIssueInput{
		IssueID: issueIDStr,
		ClaimID: input.ClaimID,
	}

	// Translate patchString fields into pointer fields on the service input.
	// Present+nil → pointer to empty string (clear); Present+non-nil → pointer
	// to the value; not present → nil pointer (no change).
	if payload.Title.Present {
		svcInput.Title = patchToPtr(payload.Title)
	}
	if payload.Description.Present {
		svcInput.Description = patchToPtr(payload.Description)
	}
	if payload.AcceptanceCriteria.Present {
		svcInput.AcceptanceCriteria = patchToPtr(payload.AcceptanceCriteria)
	}
	if payload.Priority.Present {
		if payload.Priority.Value == nil {
			// JSON null — clear the priority by setting to zero value.
			zero := domain.Priority(0)
			svcInput.Priority = &zero
		} else {
			p, priErr := domain.ParsePriority(*payload.Priority.Value)
			if priErr != nil {
				return fmt.Errorf("invalid priority %q: %v", *payload.Priority.Value, priErr)
			}
			svcInput.Priority = &p
		}
	}
	if payload.Parent.Present {
		svcInput.ParentID = patchToPtr(payload.Parent)
	}

	// Parse labels from key:value strings into service-layer DTOs.
	if payload.Labels != nil {
		labels, labelErr := cmdutil.ParseLabels(payload.Labels)
		if labelErr != nil {
			return fmt.Errorf("invalid label: %w", labelErr)
		}
		svcInput.LabelSet = labels
	}

	svcInput.LabelRemove = payload.LabelRemove
	svcInput.CommentBody = payload.Comment

	if err := input.Service.UpdateIssue(ctx, svcInput); err != nil {
		return fmt.Errorf("updating issue: %w", err)
	}

	return cmdutil.WriteJSON(input.WriteTo, updateOutput{
		IssueID: svcInput.IssueID,
		Updated: true,
	})
}

// patchToPtr converts a patchString to a *string suitable for
// driving.UpdateIssueInput. A null JSON value (Present=true, Value=nil) maps
// to a pointer to the empty string, signalling "clear this field". A non-null
// value maps to a pointer to that value.
func patchToPtr(p patchString) *string {
	if p.Value == nil {
		empty := ""
		return &empty
	}
	return p.Value
}

// newUpdateCmd constructs the "json update" subcommand, which updates fields
// on a claimed issue using structured JSON input from stdin. The --claim flag
// identifies the active claim (and by extension, the issue); the JSON object
// on stdin provides content fields only.
//
// Output is always JSON — there is no --json flag.
func newUpdateCmd(f *cmdutil.Factory) *cli.Command {
	var claimID string

	return &cli.Command{
		Name:  "update",
		Usage: "Update a claimed issue (JSON stdin)",
		Description: `Updates fields on a claimed issue using a JSON object piped to stdin. The
JSON follows PATCH semantics: fields absent from the object are left
unchanged, fields set to null are cleared, and fields set to a value are
updated. Supported fields include title, description, acceptance_criteria,
priority, parent, labels (array of "key:value" strings), label_remove
(array of key strings to remove), and comment (string to add as a comment
alongside the update).

The role and claim fields are accepted for schema compatibility with json
create. If role is present and differs from the issue's current role, an
error is returned; if it matches, it is silently accepted. The claim field
is silently ignored.

The "state" field is not writable. State transitions are managed through
dedicated lifecycle commands (claim, close, defer, release). Passing any
"state" value — including "claimed" — returns an error.

The --claim flag identifies the active claim and, by extension, the issue
being updated — no explicit issue ID is needed. Use this command when you
have already claimed an issue and need to modify its fields mid-flight.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &claimID,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return RunUpdate(ctx, RunUpdateInput{
				Service: svc,
				ClaimID: claimID,
				Stdin:   f.IOStreams.In,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}
