package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// findingOutput is the JSON representation of a single diagnostic finding.
type findingOutput struct {
	Category   string   `json:"category"`
	Severity   string   `json:"severity"`
	Message    string   `json:"message"`
	IssueIDs   []string `json:"issue_ids,omitzero"`
	Suggestion string   `json:"suggestion,omitzero"`
}

// checkOutput is the JSON representation of a single diagnostic check result
// emitted in verbose mode.
type checkOutput struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// doctorOutput is the JSON representation of the doctor command result.
type doctorOutput struct {
	Findings []findingOutput `json:"findings"`
	Checks   []checkOutput   `json:"checks,omitzero"`
	Healthy  bool            `json:"healthy"`
}

// checkDefinition maps finding categories to a named diagnostic check.
type checkDefinition struct {
	// Name is the check's identifier shown in output.
	Name string
	// Categories lists the finding categories that belong to this check.
	Categories []string
	// PassDetail is the human-readable message when the check passes.
	PassDetail string
}

// diagnosticChecks defines the ordered list of checks doctor performs.
// Each check maps to one or more finding categories.
var diagnosticChecks = []checkDefinition{
	{
		Name:       "stale_claims",
		Categories: []string{"stale_claim"},
		PassDetail: "No stale claims found",
	},
	{
		Name:       "readiness",
		Categories: []string{"no_ready_issues", "close_eligible_blocker", "deferred_blocker"},
		PassDetail: "Ready issues available",
	},
	{
		Name:       "instructions",
		Categories: []string{"instructions"},
		PassDetail: "Agent instruction files reference np",
	},
}

// deriveChecks computes the pass/fail status of each diagnostic check based
// on the findings returned by the doctor analysis. A check fails when any
// finding matches one of its categories.
func deriveChecks(findings []service.DoctorFinding) []checkOutput {
	// Index findings by category for fast lookup.
	categoryFindings := make(map[string][]service.DoctorFinding)
	for _, f := range findings {
		categoryFindings[f.Category] = append(categoryFindings[f.Category], f)
	}

	checks := make([]checkOutput, 0, len(diagnosticChecks))
	for _, def := range diagnosticChecks {
		var matched []service.DoctorFinding
		for _, cat := range def.Categories {
			matched = append(matched, categoryFindings[cat]...)
		}

		if len(matched) == 0 {
			checks = append(checks, checkOutput{
				Name:   def.Name,
				Status: "pass",
				Detail: def.PassDetail,
			})
			continue
		}

		// Use the first matched finding's message as the detail.
		checks = append(checks, checkOutput{
			Name:   def.Name,
			Status: "fail",
			Detail: matched[0].Message,
		})
	}

	return checks
}

// NewCmd constructs the "doctor" command, which runs diagnostics on the
// database and reports any integrity issues or inconsistencies.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		verbose    bool
	)

	return &cli.Command{
		Name:  "doctor",
		Usage: "Run diagnostics on the database",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
			},
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Usage:       "Show per-check pass/fail status for every diagnostic",
				Destination: &verbose,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			result, err := svc.Doctor(ctx)
			if err != nil {
				return fmt.Errorf("running diagnostics: %w", err)
			}

			// Run filesystem-level checks that don't require database access.
			cwd, _ := os.Getwd()
			result.Findings = append(result.Findings, checkNpInstructionsPresent(cwd)...)

			healthy := len(result.Findings) == 0

			if jsonOutput {
				out := doctorOutput{
					Healthy:  healthy,
					Findings: make([]findingOutput, 0, len(result.Findings)),
				}
				for _, finding := range result.Findings {
					out.Findings = append(out.Findings, findingOutput{
						Category:   finding.Category,
						Severity:   finding.Severity,
						Message:    finding.Message,
						IssueIDs:   finding.IssueIDs,
						Suggestion: finding.Suggestion,
					})
				}
				if verbose {
					out.Checks = deriveChecks(result.Findings)
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			if verbose {
				checks := deriveChecks(result.Findings)
				for _, c := range checks {
					icon := cs.SuccessIcon()
					if c.Status == "fail" {
						icon = cs.ErrorIcon()
					}
					_, _ = fmt.Fprintf(w, "%s %s — %s\n", icon, c.Name, c.Detail)
				}
				if !healthy {
					_, _ = fmt.Fprintln(w)
				}
			}

			if healthy {
				if !verbose {
					_, err := fmt.Fprintf(w, "%s No issues found.\n", cs.SuccessIcon())
					return err
				}
				return nil
			}

			for _, finding := range result.Findings {
				icon := cs.WarningIcon()
				if finding.Severity == "error" {
					icon = cs.ErrorIcon()
				}

				_, _ = fmt.Fprintf(w, "%s [%s] %s\n", icon, finding.Category, finding.Message)
				if len(finding.IssueIDs) > 0 {
					_, _ = fmt.Fprintf(w, "  Affected issues: %s\n",
						strings.Join(finding.IssueIDs, ", "))
				}
				if finding.Suggestion != "" {
					_, _ = fmt.Fprintf(w, "  Suggestion: %s\n", finding.Suggestion)
				}
			}

			return nil
		},
	}
}

// instructionFiles lists the agent instruction files to check for np references.
var instructionFiles = []string{"CLAUDE.md", "AGENTS.md"}

// checkNpInstructionsPresent checks whether at least one agent instruction file
// (CLAUDE.md or AGENTS.md) exists and contains a reference to np. AI agents
// need these instructions to know that np is the project's issue tracker.
func checkNpInstructionsPresent(cwd string) []service.DoctorFinding {
	var found bool
	var anyExists bool

	for _, name := range instructionFiles {
		path := filepath.Join(cwd, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		anyExists = true
		if strings.Contains(string(data), "np ") || strings.Contains(string(data), "`np`") {
			found = true
			break
		}
	}

	if found {
		return nil
	}

	msg := "No agent instruction files (CLAUDE.md, AGENTS.md) found — AI agents need a brief np reference in their instruction file so they know the tool exists. Add a note mentioning np, and provide the full output of 'np agent prime' at the start of each session."
	if anyExists {
		msg = "Agent instruction files exist but none mention np — AI agents need a brief np reference so they know the tool exists. Add a note mentioning np, and provide the full output of 'np agent prime' at the start of each session."
	}

	return []service.DoctorFinding{{
		Category: "instructions",
		Severity: "warning",
		Message:  msg,
	}}
}
