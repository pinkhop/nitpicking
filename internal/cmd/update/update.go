package update

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// updateOutput is the JSON representation of the update command result.
type updateOutput struct {
	IssueID string `json:"issue_id"`
	Updated bool   `json:"updated"`
}

// NewCmd constructs the "update" command, which updates fields on a claimed
// issue. The caller must hold an active claim and provide its claim ID.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput         bool
		claimID            string
		title              string
		description        string
		acceptanceCriteria string
		priority           string
		parent             string
		commentBody        string
	)

	return &cli.Command{
		Name:      "update",
		Usage:     "Atomically update one or more fields on a claimed issue",
		ArgsUsage: "<ISSUE-ID>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the issue (required)",
				Required:    true,
				Destination: &claimID,
			},
			&cli.StringFlag{
				Name:        "title",
				Aliases:     []string{"t"},
				Usage:       "New title",
				Category:    "Options",
				Destination: &title,
			},
			&cli.StringFlag{
				Name:        "description",
				Aliases:     []string{"d"},
				Usage:       "New description",
				Category:    "Options",
				Destination: &description,
			},
			&cli.StringFlag{
				Name:        "acceptance-criteria",
				Usage:       "New acceptance criteria",
				Category:    "Options",
				Destination: &acceptanceCriteria,
			},
			&cli.StringFlag{
				Name:        "priority",
				Aliases:     []string{"p"},
				Usage:       "New priority: P0–P4",
				Category:    "Options",
				Destination: &priority,
			},
			&cli.StringFlag{
				Name:        "parent",
				Usage:       "New parent epic ID (empty string to remove parent)",
				Category:    "Options",
				Destination: &parent,
			},
			&cli.StringSliceFlag{
				Name:     "dimension",
				Usage:    "Set a dimension in key:value format (repeatable)",
				Category: "Options",
			},
			&cli.StringSliceFlag{
				Name:     "dimension-remove",
				Usage:    "Remove a dimension by key (repeatable)",
				Category: "Options",
			},
			&cli.StringFlag{
				Name:        "comment",
				Usage:       "Add a comment to the issue",
				Category:    "Options",
				Destination: &commentBody,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			idResolver := cmdutil.NewIDResolver(svc)
			claimResolver := cmdutil.NewClaimIssueResolver(svc, idResolver)

			issueID, err := claimResolver.Resolve(ctx, rawID, claimID)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			input := service.UpdateIssueInput{
				IssueID:         issueID,
				ClaimID:         claimID,
				DimensionRemove: cmd.StringSlice("dimension-remove"),
				CommentBody:     commentBody,
			}

			// Set optional pointer fields only when flags are explicitly provided.
			if cmd.IsSet("title") {
				input.Title = &title
			}
			if cmd.IsSet("description") {
				input.Description = &description
			}
			if cmd.IsSet("acceptance-criteria") {
				input.AcceptanceCriteria = &acceptanceCriteria
			}
			if cmd.IsSet("priority") {
				p, err := issue.ParsePriority(priority)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
				input.Priority = &p
			}
			if cmd.IsSet("parent") {
				if parent == "" {
					zeroID := issue.ID{}
					input.ParentID = &zeroID
				} else {
					pid, err := idResolver.Resolve(ctx, parent)
					if err != nil {
						return cmdutil.FlagErrorf("invalid parent ID: %s", err)
					}
					input.ParentID = &pid
				}
			}

			// Parse dimension-set values.
			rawDimensionSet := cmd.StringSlice("dimension")
			for _, s := range rawDimensionSet {
				key, value, ok := strings.Cut(s, ":")
				if !ok {
					return cmdutil.FlagErrorf("invalid dimension %q: must be in key:value format", s)
				}
				dimension, err := issue.NewDimension(key, value)
				if err != nil {
					return cmdutil.FlagErrorf("invalid dimension %q: %s", s, err)
				}
				input.DimensionSet = append(input.DimensionSet, dimension)
			}
			if err := svc.UpdateIssue(ctx, input); err != nil {
				return fmt.Errorf("updating issue: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, updateOutput{
					IssueID: issueID.String(),
					Updated: true,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Updated %s\n",
				cs.SuccessIcon(), cs.Bold(issueID.String()))
			return err
		},
	}
}
