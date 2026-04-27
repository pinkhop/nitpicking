package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// actionHintOutput is the JSON representation of a structured remediation
// action in a diagnostic finding.
type actionHintOutput struct {
	Kind     string `json:"kind"`
	IssueID  string `json:"issue_id,omitzero"`
	SourceID string `json:"source_id,omitzero"`
	TargetID string `json:"target_id,omitzero"`
	SQL      string `json:"sql,omitzero"`
}

// findingOutput is the JSON representation of a single diagnostic finding.
type findingOutput struct {
	Category string            `json:"category"`
	Severity string            `json:"severity"`
	Message  string            `json:"message"`
	IssueIDs []string          `json:"issue_ids,omitzero"`
	Action   *actionHintOutput `json:"action,omitzero"`
}

// renderAction converts a structured ActionHint into the equivalent np CLI
// command string for human-readable text output. Returns an empty string when
// a is nil.
func renderAction(a *driving.ActionHint) string {
	if a == nil {
		return ""
	}
	switch a.Kind {
	case driving.ActionKindRunGC:
		return "Run 'np admin gc --confirm' to remove deleted issues."
	case driving.ActionKindUndefer:
		return fmt.Sprintf("Run 'np issue undefer %s --author <name>' to restore it.", a.IssueID)
	case driving.ActionKindUnblockRelationship:
		// The stored row is (source, blocked_by, target); "blocks" inverts source
		// and target in the remove command and silently no-ops. Use "blocked_by".
		return fmt.Sprintf("Run 'np rel remove %s blocked_by %s --author <name>' to remove the stale relationship.", a.SourceID, a.TargetID)
	case driving.ActionKindCloseCompleted:
		return "Run 'np epic close-completed --author <name>' to batch-close fully resolved epics."
	case driving.ActionKindInvestigateCorruption:
		return "Back up .np/ immediately and investigate corruption."
	case driving.ActionKindExecSQL:
		return "Delete the stray rows: " + a.SQL
	case driving.ActionKindAddToGitignore:
		return "Add .np/ to .gitignore"
	default:
		return ""
	}
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
	// Prefix is the database's issue ID prefix (e.g. "PKHP"). Omitted when the
	// prefix cannot be determined (e.g., database not yet initialised or schema
	// migration pending).
	Prefix   string          `json:"prefix,omitempty"`
	Findings []findingOutput `json:"findings"`
	Checks   []checkOutput   `json:"checks,omitzero"`
	Healthy  bool            `json:"healthy"`
}

// runInput holds the parameters for the doctor command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type runInput struct {
	// GetPrefixFunc retrieves the database's issue ID prefix. When nil or when
	// the function returns an error, the prefix is treated as unavailable and
	// silently omitted from the output — the command still succeeds.
	GetPrefixFunc func(context.Context) (string, error)

	// DoctorFunc runs the diagnostic checks. In production this wraps
	// svc.Doctor; tests provide a stub.
	DoctorFunc func(context.Context, driving.DoctorInput) (driving.DoctorOutput, error)

	// AdditionalFindings are findings from filesystem-level checks that run
	// outside the service layer. They are merged with service-generated
	// findings before classification.
	AdditionalFindings []driving.DoctorFinding

	// MinSeverity is the minimum severity threshold for findings.
	MinSeverity driving.DoctorSeverity

	// JSON enables machine-readable JSON output.
	JSON bool

	// Verbose shows per-check pass/fail status for every diagnostic.
	Verbose bool

	// IOStreams provides the output writer and color scheme.
	IOStreams *iostreams.IOStreams
}

// run executes the doctor diagnostic workflow: resolves the prefix, runs all
// checks, and renders the results. When the prefix is unavailable it is silently
// omitted from the output but the command still succeeds.
func run(ctx context.Context, input runInput) error {
	// Resolve the prefix. An unavailable prefix is not a fatal condition —
	// the diagnostic command should still run and report on database health.
	var prefix string
	if input.GetPrefixFunc != nil {
		p, prefixErr := input.GetPrefixFunc(ctx)
		if prefixErr == nil {
			prefix = p
		}
		// Silently ignore prefixErr: e.g., the database has not yet been
		// initialised or a schema migration is pending. The command succeeds
		// and simply omits the prefix.
	}

	result, err := input.DoctorFunc(ctx, driving.DoctorInput{
		MinSeverity:        input.MinSeverity,
		AdditionalFindings: input.AdditionalFindings,
	})
	if err != nil {
		return fmt.Errorf("running diagnostics: %w", err)
	}

	if input.JSON {
		out := doctorOutput{
			Prefix:   prefix,
			Healthy:  result.Healthy,
			Findings: make([]findingOutput, 0, len(result.Findings)),
		}
		for _, finding := range result.Findings {
			fo := findingOutput{
				Category: finding.Category,
				Severity: finding.Severity,
				Message:  finding.Message,
				IssueIDs: finding.IssueIDs,
			}
			if finding.Action != nil {
				fo.Action = &actionHintOutput{
					Kind:     string(finding.Action.Kind),
					IssueID:  finding.Action.IssueID,
					SourceID: finding.Action.SourceID,
					TargetID: finding.Action.TargetID,
					SQL:      finding.Action.SQL,
				}
			}
			out.Findings = append(out.Findings, fo)
		}
		if input.Verbose {
			out.Checks = make([]checkOutput, 0, len(result.Checks))
			for _, c := range result.Checks {
				out.Checks = append(out.Checks, checkOutput{
					Name:   c.Name,
					Status: c.Status,
					Detail: c.Detail,
				})
			}
		}
		return cmdutil.WriteJSON(input.IOStreams.Out, out)
	}

	cs := input.IOStreams.ColorScheme()
	w := input.IOStreams.Out

	// Emit the prefix line when it is known, before any check or finding output.
	if prefix != "" {
		_, _ = fmt.Fprintf(w, "Prefix: %s\n", prefix)
	}

	if input.Verbose {
		skippedCount := core.CountSkippedChecks(result.Checks)
		for _, c := range result.Checks {
			// Omit skipped checks in text mode.
			if c.Status == "skipped" {
				continue
			}
			icon := cs.SuccessIcon()
			if c.Status == "fail" {
				icon = cs.ErrorIcon()
			}
			_, _ = fmt.Fprintf(w, "%s %s — %s\n", icon, c.Name, c.Detail)
		}
		if skippedCount > 0 {
			label := core.SeverityBelow(input.MinSeverity)
			_, _ = fmt.Fprintf(w, "%s\n",
				cs.Dim(fmt.Sprintf("%d %s checks skipped (use --severity %s to include)",
					skippedCount, label, label)))
		}
		if !result.Healthy {
			_, _ = fmt.Fprintln(w)
		}
	}

	if result.Healthy {
		if !input.Verbose {
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
		if suggestion := renderAction(finding.Action); suggestion != "" {
			_, _ = fmt.Fprintf(w, "  Suggestion: %s\n", suggestion)
		}
	}

	return nil
}

// NewCmd constructs the "doctor" command, which runs diagnostics on the
// database and reports any integrity issues or inconsistencies.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput  bool
		verbose     bool
		severityArg string
	)

	return &cli.Command{
		Name:  "doctor",
		Usage: "Run diagnostics on the database",
		Description: `Runs a suite of diagnostic checks against the issue database and the
surrounding project environment. Checks include schema version (v1 databases
must be upgraded with 'np admin upgrade'), orphaned relationships, referential
integrity, blocker graph health, git ignore status for .np/, and presence of
agent instruction files. Doctor operates on both v1 and v2 databases so it
can detect and report schema_migration_required.

Use this when something feels wrong — no issues appear as ready, an
agent reports unexpected errors, or you suspect data corruption. Each
finding includes a severity (error, warning, info), a description, and
where possible a concrete remediation command. Use --verbose to see
every check (including passing ones), --severity to filter by minimum
severity, and --json for machine-readable output that agents can parse
and act on programmatically.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
				Category:    cmdutil.FlagCategorySupplemental,
			},
			&cli.BoolFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Usage:       "Show per-check pass/fail status for every diagnostic",
				Destination: &verbose,
				Category:    cmdutil.FlagCategorySupplemental,
			},
			&cli.StringFlag{
				Name:        "severity",
				Usage:       "Minimum severity threshold: error, warning, info (default: info)",
				Value:       "info",
				Destination: &severityArg,
				Category:    cmdutil.FlagCategorySupplemental,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			minSeverity, err := driving.ParseDoctorSeverity(severityArg)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, svcErr := cmdutil.NewTracker(f)
			if svcErr != nil {
				return svcErr
			}

			// Run filesystem-level checks that don't require database
			// access, then pass them to the service as additional findings.
			cwd, _ := os.Getwd()
			var additionalFindings []driving.DoctorFinding
			additionalFindings = append(additionalFindings, checkNpInstructionsPresent(cwd)...)
			additionalFindings = append(additionalFindings, checkNpGitIgnored(cwd, gitCheckIgnore)...)

			return run(ctx, runInput{
				GetPrefixFunc:      svc.GetPrefix,
				DoctorFunc:         svc.Doctor,
				AdditionalFindings: additionalFindings,
				MinSeverity:        minSeverity,
				JSON:               jsonOutput,
				Verbose:            verbose,
				IOStreams:          f.IOStreams,
			})
		},
	}
}

// instructionFiles lists the agent instruction files to check for np references.
var instructionFiles = []string{"CLAUDE.md", "AGENTS.md"}

// checkNpInstructionsPresent checks whether at least one agent instruction file
// (CLAUDE.md or AGENTS.md) exists and contains a reference to np. AI agents
// need these instructions to know that np is the project's issue tracker.
func checkNpInstructionsPresent(cwd string) []driving.DoctorFinding {
	var found bool
	var anyExists bool

	for _, name := range instructionFiles {
		path := filepath.Join(cwd, name)
		data, err := os.ReadFile(path) // #nosec G304 -- path is cwd joined with a fixed file name from instructionFiles
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

	return []driving.DoctorFinding{{
		Category: "instructions",
		Severity: "warning",
		Message:  msg,
	}}
}

// errNotGitRepo signals that the working directory is not inside a git
// repository. checkNpGitIgnored treats this as a reason to skip the check.
var errNotGitRepo = errors.New("not a git repository")

// gitIgnoreChecker is the signature for a function that checks whether a path
// is ignored by git. Returns true if ignored, false if not, or errNotGitRepo
// if the directory is not inside a git repository.
type gitIgnoreChecker func(dir, path string) (bool, error)

// gitCheckIgnore is the production implementation that shells out to git. It
// returns true when git reports the path as ignored (exit 0), false when not
// ignored (exit 1), and errNotGitRepo when git is unavailable or the directory
// is not a git repository (exit 128 or exec failure).
var gitCheckIgnore gitIgnoreChecker = func(dir, path string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", path) // #nosec G204 -- path is the fixed string ".np/"
	cmd.Dir = dir

	err := cmd.Run()
	if err == nil {
		return true, nil // exit 0 → ignored
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == 1 {
			return false, nil // exit 1 → not ignored
		}
	}

	// exit 128 (not a git repo), or exec failure (git not installed).
	return false, errNotGitRepo
}

// checkNpGitIgnored verifies that the .np/ directory is ignored by git. When
// the working directory is not inside a git repository, the check is silently
// skipped. The checker parameter allows tests to inject a stub.
func checkNpGitIgnored(cwd string, checker gitIgnoreChecker) []driving.DoctorFinding {
	ignored, err := checker(cwd, ".np/")
	if err != nil {
		// Not a git repo or git not installed — skip the check.
		return nil
	}

	if ignored {
		return nil
	}

	return []driving.DoctorFinding{{
		Category: "gitignore",
		Severity: "warning",
		Message:  "The .np/ directory is not ignored by git — it may be accidentally committed",
		Action:   &driving.ActionHint{Kind: driving.ActionKindAddToGitignore},
	}}
}
