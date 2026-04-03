package formcmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// UpdateFormValues holds the editable field values for the update form. The
// FormRunner receives a pointer to this struct so that both the TUI form and
// test fakes can write to the same memory.
type UpdateFormValues struct {
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           string
	Parent             string
	Labels             string
	Comment            string
}

// RunUpdateInput holds the parameters for the form update operation, decoupled
// from CLI flag parsing so it can be tested directly. FormRunner allows tests
// to substitute the interactive TUI with a synchronous fake that modifies
// the values directly.
type RunUpdateInput struct {
	Service    driving.Service
	IssueID    string
	ClaimID    string
	WriteTo    io.Writer
	FormRunner func(vals *UpdateFormValues) error
}

// priorities lists the canonical priority values presented in the select field.
var priorities = []string{"P0", "P1", "P2", "P3", "P4"}

// RunUpdate fetches the current issue state, presents an interactive form
// pre-populated with existing values, detects which fields changed, and sends
// only the changed fields to the service. Output is human-readable text.
func RunUpdate(ctx context.Context, input RunUpdateInput) error {
	shown, err := input.Service.ShowIssue(ctx, input.IssueID)
	if err != nil {
		return fmt.Errorf("fetching issue: %w", err)
	}

	// Pre-populate form values from the current issue state.
	vals := UpdateFormValues{
		Title:              shown.Title,
		Description:        shown.Description,
		AcceptanceCriteria: shown.AcceptanceCriteria,
		Priority:           shown.Priority.String(),
		Parent:             shown.ParentID,
		Labels:             labelsToString(shown.Labels),
	}

	runner := input.FormRunner
	if runner == nil {
		runner = defaultUpdateFormRunner
	}

	if err := runner(&vals); err != nil {
		return fmt.Errorf("running form: %w", err)
	}

	// Build the update input, only setting pointer fields for changed values.
	svcInput := driving.UpdateIssueInput{
		IssueID:     input.IssueID,
		ClaimID:     input.ClaimID,
		CommentBody: vals.Comment,
	}

	if vals.Title != shown.Title {
		svcInput.Title = &vals.Title
	}
	if vals.Description != shown.Description {
		svcInput.Description = &vals.Description
	}
	if vals.AcceptanceCriteria != shown.AcceptanceCriteria {
		svcInput.AcceptanceCriteria = &vals.AcceptanceCriteria
	}
	if vals.Priority != shown.Priority.String() {
		p, priErr := domain.ParsePriority(vals.Priority)
		if priErr != nil {
			return fmt.Errorf("invalid priority %q: %v", vals.Priority, priErr)
		}
		svcInput.Priority = &p
	}
	if vals.Parent != shown.ParentID {
		svcInput.ParentID = &vals.Parent
	}

	// Parse and diff labels.
	newLabels := parseLabelsString(vals.Labels)
	labelSet, labelRemove := diffLabels(shown.Labels, newLabels)
	svcInput.LabelSet = labelSet
	svcInput.LabelRemove = labelRemove

	if err := input.Service.UpdateIssue(ctx, svcInput); err != nil {
		return fmt.Errorf("updating issue: %w", err)
	}

	_, err = fmt.Fprintf(input.WriteTo, "Updated %s\n", input.IssueID)
	return err
}

// defaultUpdateFormRunner builds and runs the interactive huh form, binding to the
// provided value pointers so that user input flows back to the caller.
func defaultUpdateFormRunner(vals *UpdateFormValues) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Value(&vals.Title),
			huh.NewText().
				Title("Description").
				Value(&vals.Description).
				Lines(5),
			huh.NewText().
				Title("Acceptance Criteria").
				Value(&vals.AcceptanceCriteria).
				Lines(3),
			huh.NewSelect[string]().
				Title("Priority").
				Options(priorityOptions()...).
				Value(&vals.Priority),
			huh.NewInput().
				Title("Parent").
				Description("Issue ID of the parent epic (empty to clear)").
				Value(&vals.Parent),
			huh.NewInput().
				Title("Labels").
				Description("key:value pairs, comma-separated").
				Value(&vals.Labels),
			huh.NewText().
				Title("Comment").
				Description("Optional comment to add with this update").
				Value(&vals.Comment).
				Lines(3),
		),
	)

	return form.Run()
}

// priorityOptions builds huh.Option entries for the priority select.
func priorityOptions() []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(priorities))
	for _, p := range priorities {
		opts = append(opts, huh.NewOption(p, p))
	}
	return opts
}

// labelsToString converts a label map to a comma-separated "key:value" string
// suitable for display in a text input.
func labelsToString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(labels))
	for k, v := range labels {
		pairs = append(pairs, k+":"+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ", ")
}

// parseLabelsString parses a comma-separated "key:value" string back into a
// map. Empty or whitespace-only entries are skipped. Entries without a colon
// are silently ignored.
func parseLabelsString(s string) map[string]string {
	result := make(map[string]string)
	if strings.TrimSpace(s) == "" {
		return result
	}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		k, v, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return result
}

// diffLabels computes the label changes between the old and new label maps.
// Returns labels to set (new or changed) and keys to remove (present in old
// but absent in new).
func diffLabels(old, new map[string]string) ([]driving.LabelInput, []string) {
	var labelSet []driving.LabelInput
	var labelRemove []string

	// Labels to add or update.
	for k, v := range new {
		if old[k] != v {
			labelSet = append(labelSet, driving.LabelInput{Key: k, Value: v})
		}
	}

	// Labels to remove.
	for k := range old {
		if _, exists := new[k]; !exists {
			labelRemove = append(labelRemove, k)
		}
	}

	return labelSet, labelRemove
}

// newUpdateCmd constructs the "form update" subcommand, which presents an
// interactive TUI form pre-populated with the current issue values. Only
// changed fields are included in the update. Requires an active claim.
//
// Output is human-readable text only — there is no --json flag.
func newUpdateCmd(f *cmdutil.Factory) *cli.Command {
	var claimID string

	return &cli.Command{
		Name:  "update",
		Usage: "Interactively update a claimed issue",
		Description: `Presents an interactive form pre-populated with the current values of a
claimed issue. You can modify any combination of title, description,
acceptance criteria, priority, parent, and labels. Only fields you actually
change are included in the update — unchanged fields are left untouched.
You can also attach a comment as part of the same operation.

An active claim is required. Pass the claim ID via --claim (or the NP_CLAIM
environment variable). The issue ID is resolved automatically from the claim
record. For programmatic updates, use "json update" instead, which accepts
the same fields as structured JSON on stdin with PATCH semantics.`,
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

			// Resolve issue ID from the claim record.
			issueID, err := svc.LookupClaimIssueID(ctx, claimID)
			if err != nil {
				return fmt.Errorf("looking up claim: %w", err)
			}

			return RunUpdate(ctx, RunUpdateInput{
				Service: svc,
				IssueID: issueID,
				ClaimID: claimID,
				WriteTo: f.IOStreams.Out,
			})
		},
	}
}
