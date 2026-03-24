package relcmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// newBlocksCmd constructs the "rel blocks" parent command with add, remove,
// and list subcommands for managing blocking relationships.
func newBlocksCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "blocks",
		Usage: "Manage blocking relationships",
		Commands: []*cli.Command{
			newRelAddCmd(f, "blocks", issue.RelBlockedBy),
			newRelRemoveCmd(f, "blocks", issue.RelBlockedBy),
			newRelTypeListCmd(f, "blocks", issue.RelBlockedBy, issue.RelBlocks),
		},
	}
}

// newRelAddCmd constructs a generic "add" subcommand for a relationship type.
// The relType determines which relationship is created: the source is blocked
// by (or cites) the target.
func newRelAddCmd(f *cmdutil.Factory, typeName string, relType issue.RelationType) *cli.Command {
	var (
		jsonOutput bool
		author     string
		source     string
		target     string
	)

	return &cli.Command{
		Name:  "add",
		Usage: fmt.Sprintf("Add a %s relationship", typeName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "Source issue ID",
				Required:    true,
				Destination: &source,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target issue ID",
				Required:    true,
				Destination: &target,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			sourceID, err := resolver.Resolve(ctx, source)
			if err != nil {
				return cmdutil.FlagErrorf("invalid source issue ID: %s", err)
			}
			targetID, err := resolver.Resolve(ctx, target)
			if err != nil {
				return cmdutil.FlagErrorf("invalid target issue ID: %s", err)
			}

			rel := service.RelationshipInput{
				Type:     relType,
				TargetID: targetID,
			}
			if err := svc.AddRelationship(ctx, sourceID, rel, parsedAuthor); err != nil {
				return fmt.Errorf("adding %s relationship: %w", typeName, err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"source": sourceID.String(),
					"type":   relType.String(),
					"target": targetID.String(),
					"action": "added",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Added %s: %s → %s\n",
				cs.SuccessIcon(),
				relType.String(),
				cs.Bold(sourceID.String()),
				cs.Bold(targetID.String()))
			return err
		},
	}
}

// newRelRemoveCmd constructs a generic "remove" subcommand for a relationship
// type.
func newRelRemoveCmd(f *cmdutil.Factory, typeName string, relType issue.RelationType) *cli.Command {
	var (
		jsonOutput bool
		author     string
		source     string
		target     string
	)

	return &cli.Command{
		Name:  "remove",
		Usage: fmt.Sprintf("Remove a %s relationship", typeName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name",
				Required:    true,
				Destination: &author,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "Source issue ID",
				Required:    true,
				Destination: &source,
			},
			&cli.StringFlag{
				Name:        "target",
				Aliases:     []string{"t"},
				Usage:       "Target issue ID",
				Required:    true,
				Destination: &target,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			sourceID, err := resolver.Resolve(ctx, source)
			if err != nil {
				return cmdutil.FlagErrorf("invalid source issue ID: %s", err)
			}
			targetID, err := resolver.Resolve(ctx, target)
			if err != nil {
				return cmdutil.FlagErrorf("invalid target issue ID: %s", err)
			}

			rel := service.RelationshipInput{
				Type:     relType,
				TargetID: targetID,
			}
			if err := svc.RemoveRelationship(ctx, sourceID, rel, parsedAuthor); err != nil {
				return fmt.Errorf("removing %s relationship: %w", typeName, err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"source": sourceID.String(),
					"type":   relType.String(),
					"target": targetID.String(),
					"action": "removed",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed %s: %s → %s\n",
				cs.SuccessIcon(),
				relType.String(),
				cs.Bold(sourceID.String()),
				cs.Bold(targetID.String()))
			return err
		},
	}
}

// newRelTypeListCmd constructs a "list" subcommand that shows relationships
// filtered by the given types.
func newRelTypeListCmd(f *cmdutil.Factory, typeName string, types ...issue.RelationType) *cli.Command {
	var (
		jsonOutput bool
		issueArg   string
	)

	return &cli.Command{
		Name:  "list",
		Usage: fmt.Sprintf("List %s relationships for an issue", typeName),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Issue ID",
				Required:    true,
				Destination: &issueArg,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			issueID, err := resolver.Resolve(ctx, issueArg)
			if err != nil {
				return cmdutil.FlagErrorf("invalid issue ID: %s", err)
			}

			shown, err := svc.ShowIssue(ctx, issueID)
			if err != nil {
				return fmt.Errorf("looking up issue: %w", err)
			}

			filtered := FilterRelationships(shown.Relationships, types...)
			return renderRelationships(f, filtered, issueID, jsonOutput)
		},
	}
}
