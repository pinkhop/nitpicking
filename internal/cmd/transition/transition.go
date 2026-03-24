package transition

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// transitionOutput is the JSON representation of a transition command result.
type transitionOutput struct {
	TicketID string `json:"ticket_id"`
	Action   string `json:"action"`
}

// newTransitionCmd builds a single transition subcommand (release, close,
// defer, wait). All four share identical flag sets and differ only in the
// action label passed to the service.
func newTransitionCmd(f *cmdutil.Factory, name, usage string, action service.TransitionAction) *cli.Command {
	var (
		jsonOutput bool
		claimID    string
	)

	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.StringFlag{
				Name:        "claim",
				Sources:     cli.EnvVars("NP_CLAIM"),
				Usage:       "Active claim ID for the ticket",
				Required:    true,
				Destination: &claimID,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("ticket ID argument is required")
			}

			ticketID, err := ticket.ParseID(rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid ticket ID: %s", err)
			}

			input := service.TransitionInput{
				TicketID: ticketID,
				ClaimID:  claimID,
				Action:   action,
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			if err := svc.TransitionState(ctx, input); err != nil {
				return fmt.Errorf("transitioning ticket: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, transitionOutput{
					TicketID: ticketID.String(),
					Action:   name,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err = fmt.Fprintf(f.IOStreams.Out, "%s %s %s\n",
				cs.SuccessIcon(),
				pastTense(name),
				cs.Bold(ticketID.String()))
			return err
		},
	}
}

// NewStateCmd constructs the "state" parent command with close, defer, and
// wait subcommands for terminal and special state transitions.
func NewStateCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "state",
		Usage: "Transition ticket state (close, defer, wait)",
		Commands: []*cli.Command{
			newTransitionCmd(f, "close", "Close a claimed task", service.ActionClose),
			newTransitionCmd(f, "defer", "Defer a claimed ticket", service.ActionDefer),
			newTransitionCmd(f, "wait", "Mark a claimed ticket as waiting", service.ActionWait),
		},
	}
}

// NewReleaseCmd constructs the "release" command, which returns a claimed
// ticket to its default unclaimed state. Release stays at the root level
// because it is the most common transition — returning to a working state.
func NewReleaseCmd(f *cmdutil.Factory) *cli.Command {
	return newTransitionCmd(f, "release", "Release a claimed ticket", service.ActionRelease)
}

// pastTense returns a human-readable past-tense label for each transition
// action name.
func pastTense(name string) string {
	switch name {
	case "release":
		return "Released"
	case "close":
		return "Closed"
	case "defer":
		return "Deferred"
	case "wait":
		return "Set waiting on"
	default:
		return "Transitioned"
	}
}
