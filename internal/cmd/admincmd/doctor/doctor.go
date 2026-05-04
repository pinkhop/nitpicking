package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// doctorJSONOutput is the spec-required JSON shape for np admin doctor.
// Default mode: errors and warnings arrays (both always present, possibly empty).
// Verbose mode: adds the passed array.
// The prefix and skipped fields from prior implementations are excluded: the
// spec's JSON stability policy defines no such fields in the output shape.
type doctorJSONOutput struct {
	Errors   []driving.DoctorFinding     `json:"errors"`
	Warnings []driving.DoctorFinding     `json:"warnings"`
	Passed   []driving.DoctorPassedCheck `json:"passed,omitzero"`
}

// findingsForJSON returns a copy of findings ready for JSON serialization.
// In non-verbose mode WhyItMatters is cleared: the spec only includes it in
// verbose JSON output. In verbose mode the findings are returned as-is.
func findingsForJSON(findings []driving.DoctorFinding, verbose bool) []driving.DoctorFinding {
	if verbose {
		return findings
	}
	out := make([]driving.DoctorFinding, len(findings))
	for i, f := range findings {
		out[i] = f
		out[i].WhyItMatters = ""
	}
	return out
}

// runInput holds the parameters for the doctor command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type runInput struct {
	// DoctorFunc runs the diagnostic checks. In production this wraps
	// svc.Doctor; tests provide a stub.
	DoctorFunc func(context.Context, driving.DoctorInput) (driving.DoctorOutput, error)

	// MinSeverity is the minimum severity threshold for display filtering.
	MinSeverity driving.DoctorSeverity

	// JSON enables machine-readable JSON output.
	JSON bool

	// Verbose shows per-check pass/fail status for every diagnostic.
	Verbose bool

	// Version is the application version string (e.g. "0.3.0"), injected from
	// the factory. Used for the banner in text output.
	Version string

	// IOStreams provides the output writer and color scheme.
	IOStreams *iostreams.IOStreams

	// WorkDir is the working directory passed to the dot-np-directory check.
	// Empty causes the check to fall back to os.Getwd().
	WorkDir string

	// DBPath is the absolute path to the database file, passed to the
	// database-exists check. Empty when no .np/ directory was found.
	DBPath string

	// DBOpenError carries the error from opening the database store. When
	// set, database-exists reports that SQLite could not open the file.
	DBOpenError error

	// LongDeferralThreshold overrides the default 7-day staleness threshold
	// for the long-deferrals check. Zero means use the default. An invalid
	// string is rejected with a clear error before any check runs.
	LongDeferralThreshold time.Duration
}

// run executes the doctor diagnostic workflow: runs all checks and renders the
// results in either text or JSON form per input.JSON / input.Verbose.
func run(ctx context.Context, input runInput) error {
	result, err := input.DoctorFunc(ctx, driving.DoctorInput{
		MinSeverity:           input.MinSeverity,
		WorkDir:               input.WorkDir,
		DBPath:                input.DBPath,
		DBOpenError:           input.DBOpenError,
		LongDeferralThreshold: input.LongDeferralThreshold,
	})
	if err != nil {
		return fmt.Errorf("running diagnostics: %w", err)
	}

	// Ensure slices are non-nil so JSON output always has the arrays.
	if result.Errors == nil {
		result.Errors = []driving.DoctorFinding{}
	}
	if result.Warnings == nil {
		result.Warnings = []driving.DoctorFinding{}
	}

	showWarnings := input.MinSeverity != driving.SeverityError

	if input.JSON {
		jsonWarnings := findingsForJSON(result.Warnings, input.Verbose)
		if !showWarnings {
			// Severity filter: --severity error removes warnings entries from the
			// JSON array per the spec. The exit code is still based on unfiltered
			// counts, which the caller computes from result.Errors/Warnings.
			jsonWarnings = []driving.DoctorFinding{}
		}

		out := doctorJSONOutput{
			Errors:   findingsForJSON(result.Errors, input.Verbose),
			Warnings: jsonWarnings,
		}
		if input.Verbose {
			passed := result.Passed
			if passed == nil {
				// Guarantee a non-nil empty array so the passed key is always
				// present in verbose JSON (omitzero only omits nil, not []).
				passed = []driving.DoctorPassedCheck{}
			}
			out.Passed = passed
		}
		if err := cmdutil.WriteJSON(input.IOStreams.Out, out); err != nil {
			return err
		}
	} else {
		if err := renderText(input, result, showWarnings); err != nil {
			return err
		}
	}

	// Signal the spec-defined exit code based on the unfiltered findings.
	// Exit 2 = one or more errors; exit 1 = warnings only; exit 0 = all pass.
	// The --severity flag does not affect the exit code.
	return doctorExitCode(result)
}

// doctorExitCode returns nil (exit 0) when there are no findings, an
// ExitCodeError with code 1 when there are only warnings, and an
// ExitCodeError with code 2 when there are errors (regardless of warnings).
func doctorExitCode(result driving.DoctorOutput) error {
	if len(result.Errors) > 0 {
		return &cmdutil.ExitCodeError{Code: 2}
	}
	if len(result.Warnings) > 0 {
		return &cmdutil.ExitCodeError{Code: 1}
	}
	return nil
}

// renderText writes the spec-compliant text output to input.IOStreams.Out.
// It renders both default and verbose modes.
//
// Spec note (icon glyphs): the literal Unicode glyphs ✓/!/✗ are preserved
// regardless of TTY status — only colour is stripped. The renderer therefore
// uses cs.Green/Yellow/Red directly with the spec's glyph rather than
// cs.SuccessIcon/WarningIcon/ErrorIcon, which fall back to "[ok]"/"[warning]"/
// "[error]" tags when colour is disabled.
func renderText(input runInput, result driving.DoctorOutput, showWarnings bool) error {
	cs := input.IOStreams.ColorScheme()
	w := input.IOStreams.Out

	meta := core.DoctorCheckInfoBySlug()
	cats := core.DoctorCategoryOrder()
	slugsInOrder := core.DoctorSlugsInOrder()

	glyphPass := cs.Green("✓")
	glyphWarn := cs.Yellow("!")
	glyphError := cs.Red("✗")

	// ── 1. Banner ──────────────────────────────────────────────────────────
	_, _ = fmt.Fprintf(w, "np admin doctor — np v%s\n", input.Version)
	_, _ = fmt.Fprintln(w)

	// Index findings, passes, and skips by slug for fast lookup.
	errorSlugs := make(map[string]bool, len(result.Errors))
	for _, f := range result.Errors {
		errorSlugs[f.Check] = true
	}
	warnSlugs := make(map[string]bool, len(result.Warnings))
	for _, f := range result.Warnings {
		warnSlugs[f.Check] = true
	}
	passedSlugs := make(map[string]bool, len(result.Passed))
	for _, p := range result.Passed {
		passedSlugs[p.Check] = true
	}
	skippedSlugs := make(map[string]bool, len(result.Skipped))
	for _, s := range result.Skipped {
		skippedSlugs[s.Check] = true
	}

	if input.Verbose {
		// Verbose mode: banner → per-category sections directly, no status block.
		return renderVerboseCategories(input, result, w, meta, cats, slugsInOrder,
			errorSlugs, warnSlugs, passedSlugs, glyphPass, glyphWarn, glyphError, showWarnings)
	}

	// ── 2. Category status block (default mode only) ──────────────────────
	tw := cmdutil.NewTableWriter(w, 3)
	for _, cat := range cats {
		var totalInCat, passedCount int
		var hasError, hasWarn bool
		for _, slug := range slugsInOrder {
			info, ok := meta[slug]
			if !ok || info.Category != cat {
				continue
			}
			totalInCat++
			switch {
			case errorSlugs[slug]:
				hasError = true
			case warnSlugs[slug]:
				hasWarn = true
			case passedSlugs[slug]:
				passedCount++
			}
		}

		var glyph string
		switch {
		case hasError:
			glyph = glyphError
		case hasWarn:
			glyph = glyphWarn
		default:
			glyph = glyphPass
		}

		// N is total checks in the category (not just those that ran).
		// This keeps the spec example's "6 of 7" semantics intact even when
		// some checks in the category were cascade-skipped.
		var countStr string
		if passedCount == totalInCat {
			countStr = fmt.Sprintf("%d %s passed", totalInCat, pluralWord(totalInCat, "check"))
		} else {
			countStr = fmt.Sprintf("%d of %d %s passed", passedCount, totalInCat, pluralWord(totalInCat, "check"))
		}

		tw.AddRow(glyph+" "+cat, countStr)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// ── 3. Blank line (always) ─────────────────────────────────────────────
	_, _ = fmt.Fprintln(w)

	// ── 4. Default mode: findings sorted alphabetically by title ──────────
	type titledFinding struct {
		title   string
		finding driving.DoctorFinding
		isError bool
	}
	var findings []titledFinding
	for _, f := range result.Errors {
		title := f.Check
		if info, ok := meta[f.Check]; ok {
			title = info.Title
		}
		findings = append(findings, titledFinding{title: title, finding: f, isError: true})
	}
	if showWarnings {
		for _, f := range result.Warnings {
			title := f.Check
			if info, ok := meta[f.Check]; ok {
				title = info.Title
			}
			findings = append(findings, titledFinding{title: title, finding: f, isError: false})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].title < findings[j].title
	})

	for _, tf := range findings {
		f := tf.finding
		catLabel := ""
		if info, ok := meta[f.Check]; ok {
			catLabel = strings.ToLower(info.Category)
		}

		var glyph string
		if tf.isError {
			glyph = glyphError
		} else {
			glyph = glyphWarn
		}

		// Heading: glyph + title + (category) dimmed
		_, _ = fmt.Fprintf(w, "%s %s %s\n", glyph, tf.title, cs.Dim("("+catLabel+")"))
		// Summary
		_, _ = fmt.Fprintf(w, "  %s\n", f.Summary)
		// Fix block
		if f.Fix.Command != "" {
			_, _ = fmt.Fprintf(w, "  To fix, run:\n    %s\n", f.Fix.Command)
		} else if f.Fix.Instructions != "" {
			_, _ = fmt.Fprintf(w, "  To fix:\n    %s\n", f.Fix.Instructions)
		}
		// Blank line after each finding
		_, _ = fmt.Fprintln(w)
	}

	// ── 5. Trailing summary ────────────────────────────────────────────────
	_, _ = fmt.Fprintln(w, trailingSummary(result, input.MinSeverity))
	return nil
}

// renderVerboseCategories writes the per-category section of verbose text output.
func renderVerboseCategories(
	input runInput,
	result driving.DoctorOutput,
	w io.Writer,
	meta map[string]core.DoctorCheckInfo,
	cats []string,
	slugsInOrder []string,
	errorSlugs, warnSlugs, passedSlugs map[string]bool,
	glyphPass, glyphWarn, glyphError string,
	showWarnings bool,
) error {
	// Index findings by slug for inline expansion.
	errorBySlug := make(map[string]driving.DoctorFinding, len(result.Errors))
	for _, f := range result.Errors {
		errorBySlug[f.Check] = f
	}
	warnBySlug := make(map[string]driving.DoctorFinding, len(result.Warnings))
	for _, f := range result.Warnings {
		warnBySlug[f.Check] = f
	}

	const wrapWidth = 80

	for ci, cat := range cats {
		if ci > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintln(w, cat)

		for _, slug := range slugsInOrder {
			info, ok := meta[slug]
			if !ok || info.Category != cat {
				continue
			}
			// Skipped checks are omitted entirely from verbose listing.
			skipped := false
			for _, s := range result.Skipped {
				if s.Check == slug {
					skipped = true
					break
				}
			}
			if skipped {
				continue
			}
			// NotApplicable checks (not in any output list) are also omitted.
			if !errorSlugs[slug] && !warnSlugs[slug] && !passedSlugs[slug] {
				continue
			}

			var glyph string
			switch {
			case errorSlugs[slug]:
				glyph = glyphError
			case warnSlugs[slug]:
				glyph = glyphWarn
			default:
				glyph = glyphPass
			}

			_, _ = fmt.Fprintf(w, "  %s %s\n", glyph, info.Title)

			// Description, wrapped at wrapWidth-6 (6 spaces prefix).
			for _, line := range wrapLines(info.Description, wrapWidth-6) {
				_, _ = fmt.Fprintf(w, "      %s\n", line)
			}

			// Expand inline for error/warn findings.
			var finding driving.DoctorFinding
			var hasFinding bool
			if f, ok := errorBySlug[slug]; ok {
				finding = f
				hasFinding = true
			} else if f, ok := warnBySlug[slug]; ok && showWarnings {
				finding = f
				hasFinding = true
			}

			if !hasFinding {
				continue
			}

			// Why it matters
			writeWrappedLabel(w, "      Why it matters: ", "        ", finding.WhyItMatters, wrapWidth)

			// Blank line
			_, _ = fmt.Fprintln(w)

			// Summary
			_, _ = fmt.Fprintf(w, "      %s\n", finding.Summary)

			// Affected rows (capped at 3)
			if len(finding.Affected) > 0 {
				const maxAffectedRows = 3
				shown := finding.Affected
				overflow := 0
				if len(shown) > maxAffectedRows {
					overflow = len(shown) - maxAffectedRows
					shown = shown[:maxAffectedRows]
				}
				for _, row := range shown {
					_, _ = fmt.Fprintf(w, "        %s\n", formatAffectedRow(row))
				}
				if overflow > 0 {
					_, _ = fmt.Fprintf(w, "        (and %d more — see --json output for the complete list)\n", overflow)
				}
			}

			// Blank line
			_, _ = fmt.Fprintln(w)

			// Fix block
			if finding.Fix.Command != "" {
				_, _ = fmt.Fprintf(w, "      To fix, run:\n        %s\n", finding.Fix.Command)
			} else if finding.Fix.Instructions != "" {
				_, _ = fmt.Fprintf(w, "      To fix:\n        %s\n", finding.Fix.Instructions)
			}
		}
	}

	// Blank line before trailing summary.
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, trailingSummary(result, input.MinSeverity))
	return nil
}

// wrapLines splits text into lines of at most maxWidth visible runes, breaking
// at word boundaries. Preserves existing newlines.
func wrapLines(text string, maxWidth int) []string {
	if maxWidth <= 0 || text == "" {
		return []string{text}
	}
	wrapped := cmdutil.WordWrap(text, maxWidth)
	return strings.Split(wrapped, "\n")
}

// writeWrappedLabel writes labelPrefix + text wrapped at wrapWidth. Continuation
// lines use contPrefix. Width is measured by visible runes — every
// utf8.RuneCountInString call wraps cmdutil.StripANSI per the project rule
// (CLAUDE.md gotcha: ANSI escapes must not be counted as visible chars).
func writeWrappedLabel(w io.Writer, labelPrefix, contPrefix, text string, wrapWidth int) {
	firstWidth := wrapWidth - utf8.RuneCountInString(cmdutil.StripANSI(labelPrefix))
	contWidth := wrapWidth - utf8.RuneCountInString(cmdutil.StripANSI(contPrefix))

	words := strings.Fields(text)
	if len(words) == 0 {
		return
	}

	var sb strings.Builder
	lineLen := 0
	firstLine := true

	for _, word := range words {
		wl := utf8.RuneCountInString(cmdutil.StripANSI(word))
		limit := contWidth
		if firstLine {
			limit = firstWidth
		}

		if lineLen == 0 {
			sb.WriteString(word)
			lineLen = wl
		} else if lineLen+1+wl > limit {
			// Flush current line.
			if firstLine {
				_, _ = fmt.Fprintf(w, "%s%s\n", labelPrefix, sb.String())
				firstLine = false
			} else {
				_, _ = fmt.Fprintf(w, "%s%s\n", contPrefix, sb.String())
			}
			sb.Reset()
			sb.WriteString(word)
			lineLen = wl
		} else {
			sb.WriteByte(' ')
			sb.WriteString(word)
			lineLen += 1 + wl
		}
	}
	if sb.Len() > 0 {
		if firstLine {
			_, _ = fmt.Fprintf(w, "%s%s\n", labelPrefix, sb.String())
		} else {
			_, _ = fmt.Fprintf(w, "%s%s\n", contPrefix, sb.String())
		}
	}
}

// formatAffectedRow renders a typed affected-row value as a single display
// line for verbose text output.
func formatAffectedRow(row any) string {
	switch r := row.(type) {
	case driving.ClosedParentWithOpenChildRow:
		return r.Issue + " — non-closed children: " + strings.Join(r.NonClosedChildren, ", ")
	case driving.InvalidParentReferenceRow:
		return r.Issue + " — missing parent " + r.MissingParentID
	case driving.BlockedByAncestorRow:
		return r.Issue + " — blocked by ancestor " + r.BlockingAncestor
	case driving.BlockedByClosableIssueRow:
		return r.Issue + " — closable blocker " + r.ClosableBlocker
	case driving.BlockedByDeferredIssueRow:
		return r.Issue + " — blocked by " + r.Blocker
	case driving.BlockerCycleRow:
		return strings.Join(r.Cycle, " → ")
	case driving.PriorityInversionRow:
		return r.Issue + " (" + r.ChildPriority + ") — parent " + r.Parent + " (" + r.ParentPriority + ")"
	case driving.ClosableParentIssueRow:
		return r.Issue
	case driving.LongDeferralRow:
		return r.Issue + " — last activity " + r.LastActivityAt.Format("2006-01-02")
	default:
		return fmt.Sprintf("%v", row)
	}
}

// trailingSummary returns the trailing summary line for the doctor output.
// The counts in the summary are always unfiltered (spec: exit code reflects
// unfiltered results). minSeverity controls only the "at or above severity"
// variant of the zero-findings message.
//
// totalCount is sourced from the registry size (16) per the spec's AC5 — the
// summary always reads "M of 16 checks ran" regardless of NotApplicable checks.
func trailingSummary(result driving.DoctorOutput, minSeverity driving.DoctorSeverity) string {
	totalCount := len(core.DoctorSlugsInOrder())
	ranCount := len(result.Errors) + len(result.Warnings) + len(result.Passed)
	skippedCount := len(result.Skipped)

	errCount := len(result.Errors)
	warnCount := len(result.Warnings)
	findingCount := errCount + warnCount

	var checksPrefix string
	if skippedCount > 0 {
		checksPrefix = fmt.Sprintf("%d of %d %s ran", ranCount, totalCount, pluralWord(totalCount, "check"))
	} else {
		checksPrefix = fmt.Sprintf("%d %s", totalCount, pluralWord(totalCount, "check"))
	}

	var skippedSuffix string
	if skippedCount > 0 {
		skippedSuffix = fmt.Sprintf(" %d %s skipped (cascade prerequisite failed).", skippedCount, pluralWord(skippedCount, "check"))
	}

	// When the severity filter is error and no errors exist (only warnings or
	// all pass), the spec requires the filter-qualified zero-findings message.
	if minSeverity == driving.SeverityError && errCount == 0 {
		return checksPrefix + ", 0 findings at or above severity error." + skippedSuffix
	}

	if findingCount == 0 {
		return checksPrefix + ", 0 findings." + skippedSuffix
	}

	// Build the parenthetical breakdown: warnings and errors in that order.
	var parts []string
	if warnCount > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", warnCount, pluralWord(warnCount, "warning")))
	}
	if errCount > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", errCount, pluralWord(errCount, "error")))
	}

	findingStr := fmt.Sprintf("%d %s (%s)", findingCount, pluralWord(findingCount, "finding"), strings.Join(parts, ", "))
	return fmt.Sprintf("%s, %s.%s", checksPrefix, findingStr, skippedSuffix)
}

// pluralWord returns word unchanged when count is 1, and word+"s" otherwise.
// Used for simple English pluralisation (check/checks, finding/findings, etc.).
func pluralWord(count int, word string) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

// parseLongDeferralThreshold parses the long-deferral threshold value as
// resolved by urfave/cli (which already merges flag and env-var sources).
// Returns zero (so the service applies its default) when value is empty.
// Returns an error suitable for FlagErrorf wrapping when value is invalid.
func parseLongDeferralThreshold(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := cmdutil.ParseExtendedDuration(value)
	if err != nil {
		return 0, fmt.Errorf("--long-deferral-threshold: %s", err)
	}
	return d, nil
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
surrounding project environment. Checks include schema integrity, orphaned
relationships, blocker graph health, git ignore status for .np/, and
presence of agent instruction files. Doctor is read-only — it never
modifies the database, the filesystem, or any external state.

Use --verbose to see every check (including passing ones), --severity to
filter by minimum severity, and --json for machine-readable output.`,
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
				Usage:       "Minimum severity threshold: error, warning (default: warning)",
				Value:       "warning",
				Destination: &severityArg,
				Category:    cmdutil.FlagCategorySupplemental,
			},
			&cli.StringFlag{
				Name:     "long-deferral-threshold",
				Usage:    "Duration after which a deferred issue is considered stale (e.g., 7d, 14d). Default 7d.",
				Sources:  cli.EnvVars("NP_LONG_DEFERRAL_THRESHOLD"),
				Category: cmdutil.FlagCategorySupplemental,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			minSeverity, err := driving.ParseDoctorSeverity(severityArg)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Parse the long-deferral threshold before touching the database so
			// an invalid value produces a clear error immediately. The flag
			// declaration uses cli.EnvVars so cmd.String returns the env var
			// value when the flag is unset.
			longDeferralThreshold, err := parseLongDeferralThreshold(cmd.String("long-deferral-threshold"))
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Capture the working directory for the dot-np-directory check before
			// attempting any database discovery.
			workDir, _ := os.Getwd()
			if f.Workspace != "" {
				workDir = f.Workspace
			}

			// Attempt database path discovery (read-only; never creates .np/).
			// An error means no .np/ directory was found; dbPath is empty.
			dbPath, _ := f.DatabasePath()

			// Attempt to open the store only when a path was discovered AND the
			// file looks usable (exists and non-zero). Skipping clearly-broken
			// files keeps the doctor from panicking inside the SQLite library
			// during follow-on calls like GetPrefix; the database-exists check
			// reports the broken file as a finding.
			var dbOpenErr error
			var svc driving.Service

			if dbPath != "" {
				info, statErr := os.Stat(dbPath)
				if statErr == nil && info.Size() > 0 {
					store, storeErr := f.Store()
					if storeErr != nil {
						dbOpenErr = storeErr
					} else {
						svc = core.New(store, store)
					}
				}
			}

			// Create a degraded service with nil tx when the database is not
			// available. The cascade semantics in runDoctorChecks guarantee that
			// DB-connected checks are skipped before they reach svc.tx.
			if svc == nil {
				svc = core.New(nil, nil)
			}

			return run(ctx, runInput{
				DoctorFunc:            svc.Doctor,
				MinSeverity:           minSeverity,
				JSON:                  jsonOutput,
				Verbose:               verbose,
				Version:               f.AppVersion,
				IOStreams:             f.IOStreams,
				WorkDir:               workDir,
				DBPath:                dbPath,
				DBOpenError:           dbOpenErr,
				LongDeferralThreshold: longDeferralThreshold,
			})
		},
	}
}
