package relate

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// relateOutput is the JSON representation of the relate command result.
type relateOutput struct {
	Source string `json:"source"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Action string `json:"action"`
}

// NewCmd constructs the "relate" command with "add" and "remove" subcommands
// for managing ticket relationships.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "relate",
		Usage: "Manage relationships between tickets",
		Commands: []*cli.Command{
			newAddCmd(f),
			newRemoveCmd(f),
		},
	}
}

// newAddCmd constructs the "relate add" subcommand, which creates a
// directional relationship between two tickets.
func newAddCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "add",
		Usage:     "Add a relationship between two tickets",
		ArgsUsage: "<SOURCE> <TYPE> <TARGET>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Usage:       "Author name",
				Required:    true,
				Destination: &author,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			sourceRaw := cmd.Args().Get(0)
			relTypeRaw := cmd.Args().Get(1)
			targetRaw := cmd.Args().Get(2)

			if sourceRaw == "" || relTypeRaw == "" || targetRaw == "" {
				return cmdutil.FlagErrorf("usage: np relate add <SOURCE> <TYPE> <TARGET>")
			}

			sourceID, err := ticket.ParseID(sourceRaw)
			if err != nil {
				return cmdutil.FlagErrorf("invalid source ticket ID: %s", err)
			}

			relType, err := ticket.ParseRelationType(relTypeRaw)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			targetID, err := ticket.ParseID(targetRaw)
			if err != nil {
				return cmdutil.FlagErrorf("invalid target ticket ID: %s", err)
			}

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			rel := service.RelationshipInput{
				Type:     relType,
				TargetID: targetID,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			if err := svc.AddRelationship(ctx, sourceID, rel, parsedAuthor); err != nil {
				return fmt.Errorf("adding relationship: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, relateOutput{
					Source: sourceID.String(),
					Type:   relType.String(),
					Target: targetID.String(),
					Action: "added",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Added %s relationship: %s → %s\n",
				cs.SuccessIcon(),
				relType.String(),
				cs.Bold(sourceID.String()),
				cs.Bold(targetID.String()))
			return err
		},
	}
}

// newRemoveCmd constructs the "relate remove" subcommand, which removes a
// directional relationship between two tickets.
func newRemoveCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		author     string
	)

	return &cli.Command{
		Name:      "remove",
		Usage:     "Remove a relationship between two tickets",
		ArgsUsage: "<SOURCE> <TYPE> <TARGET>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Usage:       "Author name",
				Required:    true,
				Destination: &author,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			sourceRaw := cmd.Args().Get(0)
			relTypeRaw := cmd.Args().Get(1)
			targetRaw := cmd.Args().Get(2)

			if sourceRaw == "" || relTypeRaw == "" || targetRaw == "" {
				return cmdutil.FlagErrorf("usage: np relate remove <SOURCE> <TYPE> <TARGET>")
			}

			sourceID, err := ticket.ParseID(sourceRaw)
			if err != nil {
				return cmdutil.FlagErrorf("invalid source ticket ID: %s", err)
			}

			relType, err := ticket.ParseRelationType(relTypeRaw)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			targetID, err := ticket.ParseID(targetRaw)
			if err != nil {
				return cmdutil.FlagErrorf("invalid target ticket ID: %s", err)
			}

			parsedAuthor, err := identity.NewAuthor(author)
			if err != nil {
				return cmdutil.FlagErrorf("invalid author: %s", err)
			}

			rel := service.RelationshipInput{
				Type:     relType,
				TargetID: targetID,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			if err := svc.RemoveRelationship(ctx, sourceID, rel, parsedAuthor); err != nil {
				return fmt.Errorf("removing relationship: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, relateOutput{
					Source: sourceID.String(),
					Type:   relType.String(),
					Target: targetID.String(),
					Action: "removed",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s Removed %s relationship: %s → %s\n",
				cs.SuccessIcon(),
				relType.String(),
				cs.Bold(sourceID.String()),
				cs.Bold(targetID.String()))
			return err
		},
	}
}
