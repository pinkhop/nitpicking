package show

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// showOutput is the JSON representation of the show command result.
type showOutput struct {
	ID                 string               `json:"id"`
	Role               string               `json:"role"`
	Title              string               `json:"title"`
	Description        string               `json:"description,omitzero"`
	AcceptanceCriteria string               `json:"acceptance_criteria,omitzero"`
	Priority           string               `json:"priority"`
	State              string               `json:"state"`
	ParentID           string               `json:"parent_id,omitzero"`
	Revision           int                  `json:"revision"`
	Author             string               `json:"author,omitzero"`
	IsReady            bool                 `json:"is_ready"`
	IsComplete         bool                 `json:"is_complete,omitzero"`
	ClaimID            string               `json:"claim_id,omitzero"`
	ClaimAuthor        string               `json:"claim_author,omitzero"`
	ClaimStaleAt       string               `json:"claim_stale_at,omitzero"`
	Relationships      []relationshipOutput `json:"relationships,omitzero"`
	CreatedAt          string               `json:"created_at"`
}

// relationshipOutput is the JSON representation of a single relationship.
type relationshipOutput struct {
	Type     string `json:"type"`
	TargetID string `json:"target_id"`
}

// NewCmd constructs the "show" command, which displays the full detail view
// of a single ticket.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "show",
		Usage:     "Show full details for a ticket",
		ArgsUsage: "<TICKET-ID>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
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

			svc := f.Service()
			result, err := svc.ShowTicket(ctx, ticketID)
			if err != nil {
				return fmt.Errorf("showing ticket: %w", err)
			}

			t := result.Ticket

			if jsonOutput {
				out := showOutput{
					ID:                 t.ID().String(),
					Role:               t.Role().String(),
					Title:              t.Title(),
					Description:        t.Description(),
					AcceptanceCriteria: t.AcceptanceCriteria(),
					Priority:           t.Priority().String(),
					State:              t.State().String(),
					Revision:           result.Revision,
					Author:             result.Author.String(),
					IsReady:            result.IsReady,
					IsComplete:         result.IsComplete,
					ClaimID:            result.ClaimID,
					ClaimAuthor:        result.ClaimAuthor,
					CreatedAt:          t.CreatedAt().Format(time.RFC3339),
				}

				if !t.ParentID().IsZero() {
					out.ParentID = t.ParentID().String()
				}

				if !result.ClaimStaleAt.IsZero() {
					out.ClaimStaleAt = result.ClaimStaleAt.Format(time.RFC3339)
				}

				for _, rel := range result.Relationships {
					out.Relationships = append(out.Relationships, relationshipOutput{
						Type:     rel.Type().String(),
						TargetID: rel.TargetID().String(),
					})
				}

				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			// Human-readable output.
			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			_, _ = fmt.Fprintf(w, "%s  %s\n", cs.Bold(t.ID().String()), t.Title())
			_, _ = fmt.Fprintf(w, "Role: %s  |  State: %s  |  Priority: %s\n",
				t.Role().String(), t.State().String(), t.Priority().String())

			if !t.ParentID().IsZero() {
				_, _ = fmt.Fprintf(w, "Parent: %s\n", t.ParentID().String())
			}

			_, _ = fmt.Fprintf(w, "Revision: %d  |  Author: %s\n", result.Revision, result.Author.String())

			if result.IsReady {
				_, _ = fmt.Fprintf(w, "Ready: %s\n", cs.Green("yes"))
			}
			if result.IsComplete {
				_, _ = fmt.Fprintf(w, "Complete: %s\n", cs.Green("yes"))
			}

			if result.ClaimID != "" {
				_, _ = fmt.Fprintf(w, "Claim: %s by %s", cs.Cyan(result.ClaimID), result.ClaimAuthor)
				if !result.ClaimStaleAt.IsZero() {
					_, _ = fmt.Fprintf(w, " (stale at %s)", result.ClaimStaleAt.Format(time.RFC3339))
				}
				_, _ = fmt.Fprintln(w)
			}

			if t.Description() != "" {
				_, _ = fmt.Fprintf(w, "\n%s\n%s\n", cs.Bold("Description:"), t.Description())
			}
			if t.AcceptanceCriteria() != "" {
				_, _ = fmt.Fprintf(w, "\n%s\n%s\n", cs.Bold("Acceptance Criteria:"), t.AcceptanceCriteria())
			}

			if len(result.Relationships) > 0 {
				_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold("Relationships:"))
				for _, rel := range result.Relationships {
					_, _ = fmt.Fprintf(w, "  %s → %s\n", rel.Type().String(), rel.TargetID().String())
				}
			}

			// Display facets.
			if t.Facets().Len() > 0 {
				_, _ = fmt.Fprintf(w, "\n%s\n", cs.Bold("Facets:"))
				for k, v := range t.Facets().All() {
					_, _ = fmt.Fprintf(w, "  %s: %s\n", k, v)
				}
			}

			return nil
		},
	}
}
