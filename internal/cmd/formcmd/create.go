package formcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// ErrUserAborted is returned when the user cancels the interactive form.
var ErrUserAborted = huh.ErrUserAborted

// CreateFormData holds the values collected by the interactive form. Fields are
// bound to form controls via pointer accessors. The struct is exported so that
// tests can populate it directly without running the TUI.
type CreateFormData struct {
	Role               string
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           string
	Parent             string
	Labels             string
	Author             string
}

// RunFormCreateInput holds the parameters for the form create operation,
// decoupled from CLI flag parsing so it can be tested directly.
type RunFormCreateInput struct {
	Service driving.Service
	WriteTo io.Writer

	// FormRunner presents the interactive form and populates data. In
	// production this runs the huh TUI; in tests it is replaced with a
	// function that sets fields directly.
	FormRunner func(data *CreateFormData) error
}

// RunFormCreate presents an interactive form, collects issue creation fields,
// and creates the issue via the service layer. Output is human-readable text.
func RunFormCreate(ctx context.Context, input RunFormCreateInput) error {
	data := &CreateFormData{
		Priority: "P2", // sensible default
	}

	if err := input.FormRunner(data); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	// Validate required fields.
	if data.Role == "" {
		return fmt.Errorf("role is required")
	}
	role, roleErr := domain.ParseRole(data.Role)
	if roleErr != nil {
		return fmt.Errorf("invalid role %q: must be task or epic", data.Role)
	}
	if data.Title == "" {
		return fmt.Errorf("title is required")
	}
	if data.Author == "" {
		return fmt.Errorf("author is required")
	}

	// Parse comma-separated labels into service-layer DTOs.
	var labelInputs []driving.LabelInput
	if data.Labels != "" {
		raw := splitAndTrim(data.Labels)
		parsed, err := cmdutil.ParseLabels(raw)
		if err != nil {
			return fmt.Errorf("invalid label: %w", err)
		}
		labelInputs = parsed
	}

	// Resolve parent ID if provided.
	var parentIDStr string
	if data.Parent != "" {
		resolver := cmdutil.NewIDResolver(input.Service)
		parentID, err := resolver.Resolve(ctx, data.Parent)
		if err != nil {
			return fmt.Errorf("invalid parent ID: %w", err)
		}
		parentIDStr = parentID.String()
	}

	var priority domain.Priority
	if data.Priority != "" {
		var priErr error
		priority, priErr = domain.ParsePriority(data.Priority)
		if priErr != nil {
			return fmt.Errorf("invalid priority %q: %v", data.Priority, priErr)
		}
	}

	result, err := input.Service.CreateIssue(ctx, driving.CreateIssueInput{
		Role:               role,
		Title:              data.Title,
		Description:        data.Description,
		AcceptanceCriteria: data.AcceptanceCriteria,
		Priority:           priority,
		ParentID:           parentIDStr,
		Labels:             labelInputs,
		Author:             data.Author,
	})
	if err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	t := result.Issue
	_, _ = fmt.Fprintf(input.WriteTo,
		"Created %s %s: %s (priority: %s, state: %s)\n",
		t.Role(), t.ID(), t.Title(), t.Priority(), t.State(),
	)
	return nil
}

// splitAndTrim splits a comma-separated string and trims whitespace from each
// element. Empty elements are discarded.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// DefaultFormRunner builds and runs the interactive huh form, binding each
// field to the corresponding CreateFormData pointer. It is exported so that the
// root-level "create" command can delegate to it in TTY mode.
func DefaultFormRunner(data *CreateFormData) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Role").
				Options(
					huh.NewOption("Task", "task"),
					huh.NewOption("Epic", "epic"),
				).
				Value(&data.Role),

			huh.NewInput().
				Title("Title").
				Placeholder("Issue title (required)").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("title is required")
					}
					return nil
				}).
				Value(&data.Title),

			huh.NewText().
				Title("Description").
				Placeholder("Optional description").
				Value(&data.Description),

			huh.NewText().
				Title("Acceptance Criteria").
				Placeholder("Optional acceptance criteria").
				Value(&data.AcceptanceCriteria),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Priority").
				Options(
					huh.NewOption("P0 — Critical", "P0"),
					huh.NewOption("P1 — High", "P1"),
					huh.NewOption("P2 — Medium (default)", "P2"),
					huh.NewOption("P3 — Low", "P3"),
					huh.NewOption("P4 — Minimal", "P4"),
				).
				Value(&data.Priority),

			huh.NewInput().
				Title("Parent").
				Placeholder("Optional parent issue ID (e.g., NP-abc12)").
				Value(&data.Parent),

			huh.NewInput().
				Title("Labels").
				Placeholder("Comma-separated key:value pairs (e.g., kind:feat, area:auth)").
				Value(&data.Labels),

			huh.NewInput().
				Title("Author").
				Placeholder("Your author name (required)").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("author is required")
					}
					return nil
				}).
				Value(&data.Author),
		),
	)

	return form.Run()
}

// newCreateCmd constructs the "form create" subcommand, which interactively
// prompts the user for issue creation fields using the TUI form library. Output
// is human-readable text only — there is no --json flag.
func newCreateCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Interactively create an issue",
		Description: `Presents a multi-step interactive form that walks you through creating a new
issue. The form prompts for role (task or epic), title, description, acceptance
criteria, priority, parent issue, labels, and author — with inline validation
for required fields and sensible defaults (e.g., P2 priority).

Use this command when you are at a terminal and want a guided creation
experience. The root "create" command automatically delegates here when it
detects a TTY. For scripted or agent-driven creation, use "json create"
instead — it accepts the same fields as structured JSON on stdin and produces
machine-readable output.`,
		Action: func(ctx context.Context, _ *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return RunFormCreate(ctx, RunFormCreateInput{
				Service:    svc,
				WriteTo:    f.IOStreams.Out,
				FormRunner: DefaultFormRunner,
			})
		},
	}
}
