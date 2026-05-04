package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// assertExitCode verifies that err carries the expected exit code.
// Pass wantCode 0 to assert nil (no findings); pass 1 or 2 to assert the
// corresponding ExitCodeError (warnings-only or errors-present).
func assertExitCode(t *testing.T, err error, wantCode int) {
	t.Helper()
	if wantCode == 0 {
		if err != nil {
			t.Fatalf("expected nil (exit 0), got error: %v", err)
		}
		return
	}
	var ece *cmdutil.ExitCodeError
	if !errors.As(err, &ece) {
		t.Fatalf("expected *ExitCodeError (exit %d), got: %v (%T)", wantCode, err, err)
	}
	if int(ece.Code) != wantCode {
		t.Errorf("exit code: got %d, want %d", ece.Code, wantCode)
	}
}

// --- helpers ---

// stubDoctorOutput is a minimal DoctorOutput used across tests that need a
// healthy (no findings) result.
var stubDoctorOutput = driving.DoctorOutput{
	Errors:   []driving.DoctorFinding{},
	Warnings: []driving.DoctorFinding{},
}

// stubDoctorFunc is a DoctorFunc stub that always returns a healthy result.
func stubDoctorFunc(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
	return stubDoctorOutput, nil
}

// newTestStreams constructs IOStreams and returns the stdout buffer for assertions.
func newTestStreams() (*iostreams.IOStreams, *bytes.Buffer) {
	ios, _, stdout, _ := iostreams.Test()
	return ios, stdout
}

// parseJSON is a test helper that unmarshals the buffer into a
// map[string]any and fails the test on invalid JSON.
func parseJSON(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	return result
}

// --- run tests: JSON output shape ---

// TestRun_JSON_HealthyWorkspace_DefaultMode verifies that a healthy workspace
// with --json produces exactly {errors:[], warnings:[]} with no additional keys.
func TestRun_JSON_HealthyWorkspace_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning a clean healthy result.
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  stubDoctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		IOStreams:   ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — valid JSON with exactly errors and warnings (both empty arrays).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := parseJSON(t, stdout)

	// errors and warnings must be present as empty arrays.
	errorsVal, ok := result["errors"]
	if !ok {
		t.Fatal("JSON missing required key: errors")
	}
	if arr, ok := errorsVal.([]any); !ok || len(arr) != 0 {
		t.Errorf("errors: expected empty array, got %v", errorsVal)
	}
	warningsVal, ok := result["warnings"]
	if !ok {
		t.Fatal("JSON missing required key: warnings")
	}
	if arr, ok := warningsVal.([]any); !ok || len(arr) != 0 {
		t.Errorf("warnings: expected empty array, got %v", warningsVal)
	}

	// Default mode must have exactly two top-level keys: errors and warnings.
	// Any unexpected key (prefix, passed, skipped, summary, etc.) indicates a
	// schema violation per AC1.
	allowed := map[string]bool{"errors": true, "warnings": true}
	for k := range result {
		if !allowed[k] {
			t.Errorf("default JSON must not include key %q", k)
		}
	}
}

// TestRun_JSON_HealthyWorkspace_VerboseMode verifies that verbose JSON adds a
// passed array and still omits prefix/skipped.
func TestRun_JSON_HealthyWorkspace_VerboseMode(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning passed checks (healthy workspace).
	passed := []driving.DoctorPassedCheck{
		{Check: "dot-np-directory", Description: "desc A"},
		{Check: "database-exists", Description: "desc B"},
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{},
			Passed:   passed,
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		Verbose:     true,
		IOStreams:   ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — passed array is present; prefix and skipped are absent.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := parseJSON(t, stdout)

	if _, present := result["prefix"]; present {
		t.Error("verbose JSON must not include key prefix")
	}
	if _, present := result["skipped"]; present {
		t.Error("verbose JSON must not include key skipped")
	}

	passedVal, ok := result["passed"]
	if !ok {
		t.Fatal("verbose JSON missing required key: passed")
	}
	passedArr, ok := passedVal.([]any)
	if !ok {
		t.Fatalf("passed: expected array, got %T", passedVal)
	}
	if len(passedArr) != 2 {
		t.Errorf("passed: expected 2 entries, got %d", len(passedArr))
	}
}

// TestRun_JSON_DefaultMode_WhyItMattersAbsent verifies that default JSON does
// not include why_it_matters on error or warning findings.
func TestRun_JSON_DefaultMode_WhyItMattersAbsent(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning an error finding with WhyItMatters set.
	finding := driving.DoctorFinding{
		Check:        "invalid-parent-reference",
		Description:  "Detects issues with dangling parents.",
		WhyItMatters: "This is a data integrity error.",
		Summary:      "1 issue references a parent that does not exist.",
		Fix:          driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{finding},
			Warnings: []driving.DoctorFinding{},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		Verbose:     false,
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — why_it_matters is absent from the error entry.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	if len(errorsArr) != 1 {
		t.Fatalf("errors: expected 1 entry, got %d", len(errorsArr))
	}
	entry, _ := errorsArr[0].(map[string]any)
	if _, present := entry["why_it_matters"]; present {
		t.Error("default JSON finding must not include why_it_matters")
	}
}

// TestRun_JSON_VerboseMode_WhyItMattersPresent verifies that verbose JSON
// includes why_it_matters on every error and warning finding.
func TestRun_JSON_VerboseMode_WhyItMattersPresent(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning findings with WhyItMatters set.
	errFinding := driving.DoctorFinding{
		Check:        "invalid-parent-reference",
		Description:  "Detects issues with dangling parents.",
		WhyItMatters: "Data integrity error.",
		Summary:      "1 issue.",
		Fix:          driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
	}
	warnFinding := driving.DoctorFinding{
		Check:        "long-deferrals",
		Description:  "Detects stale deferred issues.",
		WhyItMatters: "Long-untouched deferrals are usually forgotten work.",
		Summary:      "1 issue stale.",
		Fix:          driving.DoctorFix{Instructions: "Review each long-deferred issue."},
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{errFinding},
			Warnings: []driving.DoctorFinding{warnFinding},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		Verbose:     true,
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — why_it_matters is present on both error and warning findings.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	if len(errorsArr) != 1 {
		t.Fatalf("errors: expected 1 entry, got %d", len(errorsArr))
	}
	errEntry, _ := errorsArr[0].(map[string]any)
	if errEntry["why_it_matters"] != "Data integrity error." {
		t.Errorf("error finding why_it_matters: got %v, want %q", errEntry["why_it_matters"], "Data integrity error.")
	}

	warningsArr, _ := result["warnings"].([]any)
	if len(warningsArr) != 1 {
		t.Fatalf("warnings: expected 1 entry, got %d", len(warningsArr))
	}
	warnEntry, _ := warningsArr[0].(map[string]any)
	if warnEntry["why_it_matters"] != "Long-untouched deferrals are usually forgotten work." {
		t.Errorf("warning finding why_it_matters: got %v, want expected string", warnEntry["why_it_matters"])
	}
}

// TestRun_JSON_SeverityErrorFilter_WarningsEmpty verifies that --severity error
// produces an empty warnings array in JSON output.
func TestRun_JSON_SeverityErrorFilter_WarningsEmpty(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning both errors and warnings.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue.",
					Fix:     driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "long-deferrals",
					Summary: "1 stale issue.",
					Fix:     driving.DoctorFix{Instructions: "Review deferred issues."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityError, // only errors
		JSON:        true,
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — warnings array is present but empty; errors array is non-empty.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	if len(errorsArr) != 1 {
		t.Errorf("errors: expected 1 entry, got %d", len(errorsArr))
	}
	warningsVal, ok := result["warnings"]
	if !ok {
		t.Fatal("warnings key must always be present in JSON even when filtered")
	}
	warningsArr, ok := warningsVal.([]any)
	if !ok || len(warningsArr) != 0 {
		t.Errorf("warnings: expected empty array when --severity error, got %v", warningsVal)
	}
}

// TestRun_JSON_SkippedChecks_NotInOutput verifies that skipped checks do not
// appear in errors, warnings, or passed and that no top-level skipped key is
// emitted — AC9.
func TestRun_JSON_SkippedChecks_NotInOutput(t *testing.T) {
	t.Parallel()

	const skippedSlug = "storage-integrity"

	// Given — a doctor function returning a skipped check alongside an error.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{Check: "database-exists", Summary: "DB missing.", Fix: driving.DoctorFix{Instructions: "Restore backup."}},
			},
			Warnings: []driving.DoctorFinding{},
			Skipped: []driving.DoctorSkippedCheck{
				{Check: skippedSlug, Prerequisite: "database-exists"},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		Verbose:     true, // covers both default and verbose paths
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — skipped slug must not appear in errors, warnings, passed, or as a
	// top-level key in JSON output.
	raw := stdout.String()
	result := parseJSON(t, stdout)

	if _, present := result["skipped"]; present {
		t.Error("JSON must not include a top-level skipped key")
	}
	if strings.Contains(raw, skippedSlug) {
		t.Errorf("skipped check slug %q must not appear anywhere in JSON output, got: %s", skippedSlug, raw)
	}
}

// TestRun_JSON_AffectedUncapped verifies that the affected array is not capped
// in JSON output regardless of the verbose setting.
func TestRun_JSON_AffectedUncapped(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning a finding with 50 affected rows (AC6
	// specifies "a fixture with 50 affected rows yields all 50").
	affected := make([]any, 50)
	for i := range affected {
		affected[i] = map[string]string{"issue": "NP-xxxxx"}
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:    "closed-parent-with-open-child",
					Summary:  "10 issues.",
					Affected: affected,
					Fix:      driving.DoctorFix{Instructions: "Fix manually."},
				},
			},
			Warnings: []driving.DoctorFinding{},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		Verbose:     false, // non-verbose should still show all affected rows
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — all 10 affected rows are present in JSON output.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	if len(errorsArr) != 1 {
		t.Fatalf("errors: expected 1 entry, got %d", len(errorsArr))
	}
	entry, _ := errorsArr[0].(map[string]any)
	affectedVal, ok := entry["affected"]
	if !ok {
		t.Fatal("affected key missing from error finding")
	}
	affectedArr, ok := affectedVal.([]any)
	if !ok {
		t.Fatalf("affected: expected array, got %T", affectedVal)
	}
	if len(affectedArr) != 50 {
		t.Errorf("affected: expected all 50 rows in JSON (uncapped), got %d", len(affectedArr))
	}
}

// TestRun_JSON_SystemCheck_AffectedKeyAbsent verifies that system and environment
// checks (no affected-row schema) omit the affected key entirely from JSON.
func TestRun_JSON_SystemCheck_AffectedKeyAbsent(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning a system-level finding with nil Affected.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:    "dot-np-directory",
					Summary:  "No .np/ directory found.",
					Affected: nil, // system check: no affected-row schema
					Fix:      driving.DoctorFix{Instructions: "Run np init."},
				},
			},
			Warnings: []driving.DoctorFinding{},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	// Then — the affected key is entirely absent from the error entry.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	if len(errorsArr) != 1 {
		t.Fatalf("errors: expected 1 entry, got %d", len(errorsArr))
	}
	entry, _ := errorsArr[0].(map[string]any)
	if _, present := entry["affected"]; present {
		t.Error("system check JSON finding must not include the affected key when Affected is nil")
	}
}

// TestRun_JSON_AffectedRowFieldNames verifies that the JSON field names for
// per-check affected rows match the spec exactly (AC4). The renderer serialises
// DTO structs via encoding/json; the test asserts the key names in the output.
func TestRun_JSON_AffectedRowFieldNames(t *testing.T) {
	t.Parallel()

	// Given — findings that exercise each typed affected-row DTO.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "closed-parent-with-open-child",
					Summary: "1 issue.",
					Affected: []any{
						driving.ClosedParentWithOpenChildRow{Issue: "NP-par01", NonClosedChildren: []string{"NP-chi02", "NP-chi03"}},
					},
					Fix: driving.DoctorFix{Instructions: "Reopen the parent or close the children."},
				},
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue.",
					Affected: []any{
						driving.InvalidParentReferenceRow{Issue: "NP-chi04", MissingParentID: "NP-par05"},
					},
					Fix: driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "blocked-by-ancestor",
					Summary: "1 issue.",
					Affected: []any{
						driving.BlockedByAncestorRow{Issue: "NP-abc06", BlockingAncestor: "NP-anc07"},
					},
					Fix: driving.DoctorFix{Instructions: "Restructure the parent chain."},
				},
				{
					Check:   "blocker-cycles",
					Summary: "1 cycle.",
					Affected: []any{
						driving.BlockerCycleRow{Cycle: []string{"NP-aaa08", "NP-bbb09"}},
					},
					Fix: driving.DoctorFix{Instructions: "Remove an edge to break the cycle."},
				},
				{
					Check:   "priority-inversions",
					Summary: "1 issue.",
					Affected: []any{
						driving.PriorityInversionRow{Issue: "NP-chi10", Parent: "NP-par11", ChildPriority: "P0", ParentPriority: "P3"},
					},
					Fix: driving.DoctorFix{Instructions: "Raise the parent's priority."},
				},
				{
					Check:   "long-deferrals",
					Summary: "1 issue.",
					Affected: []any{
						driving.LongDeferralRow{Issue: "NP-def12"},
					},
					Fix: driving.DoctorFix{Instructions: "Review each long-deferred issue."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		JSON:        true,
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)

	// Then — each affected row must use the spec's field names exactly.
	result := parseJSON(t, stdout)

	errorsArr, _ := result["errors"].([]any)
	warningsArr, _ := result["warnings"].([]any)

	findEntry := func(arr []any, slug string) map[string]any {
		t.Helper()
		for _, item := range arr {
			m, _ := item.(map[string]any)
			if m["check"] == slug {
				return m
			}
		}
		t.Fatalf("entry with check %q not found in JSON", slug)
		return nil
	}
	firstAffectedRow := func(entry map[string]any) map[string]any {
		t.Helper()
		affArr, _ := entry["affected"].([]any)
		if len(affArr) == 0 {
			t.Fatal("affected array is empty")
		}
		row, _ := affArr[0].(map[string]any)
		return row
	}

	// closed-parent-with-open-child: {"issue", "non_closed_children"}
	e := findEntry(errorsArr, "closed-parent-with-open-child")
	row := firstAffectedRow(e)
	if row["issue"] != "NP-par01" {
		t.Errorf("closed-parent-with-open-child: issue field: got %v, want NP-par01", row["issue"])
	}
	children, _ := row["non_closed_children"].([]any)
	if len(children) != 2 {
		t.Errorf("closed-parent-with-open-child: non_closed_children: got %v entries, want 2", len(children))
	}

	// invalid-parent-reference: {"issue", "missing_parent_id"}
	e = findEntry(errorsArr, "invalid-parent-reference")
	row = firstAffectedRow(e)
	if row["missing_parent_id"] != "NP-par05" {
		t.Errorf("invalid-parent-reference: missing_parent_id: got %v, want NP-par05", row["missing_parent_id"])
	}

	// blocked-by-ancestor: {"issue", "blocking_ancestor"}
	e = findEntry(warningsArr, "blocked-by-ancestor")
	row = firstAffectedRow(e)
	if row["blocking_ancestor"] != "NP-anc07" {
		t.Errorf("blocked-by-ancestor: blocking_ancestor: got %v, want NP-anc07", row["blocking_ancestor"])
	}

	// blocker-cycles: {"cycle"}
	e = findEntry(warningsArr, "blocker-cycles")
	row = firstAffectedRow(e)
	cycle, _ := row["cycle"].([]any)
	if len(cycle) != 2 || cycle[0] != "NP-aaa08" {
		t.Errorf("blocker-cycles: cycle field: got %v", row["cycle"])
	}

	// priority-inversions: {"issue", "parent", "child_priority", "parent_priority"}
	e = findEntry(warningsArr, "priority-inversions")
	row = firstAffectedRow(e)
	if row["child_priority"] != "P0" {
		t.Errorf("priority-inversions: child_priority: got %v, want P0", row["child_priority"])
	}
	if row["parent_priority"] != "P3" {
		t.Errorf("priority-inversions: parent_priority: got %v, want P3", row["parent_priority"])
	}

	// long-deferrals: {"issue", "deferred_at", "last_activity_at"}
	e = findEntry(warningsArr, "long-deferrals")
	row = firstAffectedRow(e)
	if _, ok := row["deferred_at"]; !ok {
		t.Error("long-deferrals: deferred_at key must be present")
	}
	if _, ok := row["last_activity_at"]; !ok {
		t.Error("long-deferrals: last_activity_at key must be present")
	}
}

// TestRun_JSON_DeterministicOrdering verifies that two identical runs produce
// byte-for-byte identical JSON output.
func TestRun_JSON_DeterministicOrdering(t *testing.T) {
	t.Parallel()

	// Given — a doctor function returning findings in a fixed order.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue.",
					Affected: []any{
						driving.InvalidParentReferenceRow{Issue: "NP-aaa01", MissingParentID: "NP-bbb02"},
					},
					Fix: driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "long-deferrals",
					Summary: "2 stale issues.",
					Affected: []any{
						driving.LongDeferralRow{Issue: "NP-ccc03"},
						driving.LongDeferralRow{Issue: "NP-ddd04"},
					},
					Fix: driving.DoctorFix{Instructions: "Review deferred issues."},
				},
			},
		}, nil
	}

	runOnce := func() string {
		ios, stdout := newTestStreams()
		input := runInput{
			DoctorFunc:  doctorFunc,
			MinSeverity: driving.SeverityWarning,
			JSON:        true,
			IOStreams:   ios,
		}
		// Errors+warnings present → exit 2.
		assertExitCode(t, run(t.Context(), input), 2)
		return stdout.String()
	}

	// When — two identical runs are performed.
	first := runOnce()
	second := runOnce()

	// Then — output is byte-for-byte identical.
	if first != second {
		t.Errorf("non-deterministic JSON output:\nfirst:  %s\nsecond: %s", first, second)
	}
}

// --- run tests: text output rendering ---

// allChecksPassed returns a Passed slice containing all 16 doctor checks,
// matching what the orchestrator would produce in a fully healthy workspace.
// Used by tests that exercise the category status block's "N checks passed"
// path (AC4).
func allChecksPassed() []driving.DoctorPassedCheck {
	slugs := []string{
		"dot-np-directory", "database-exists", "storage-integrity",
		"schema-version", "column-data-validity",
		"closed-parent-with-open-child", "invalid-parent-reference",
		"agent-instructions", "git-ignore",
		"blocked-by-ancestor", "blocked-by-closable-issue", "blocked-by-deferred-issue",
		"blocker-cycles", "priority-inversions",
		"closable-parent-issues", "long-deferrals",
	}
	out := make([]driving.DoctorPassedCheck, len(slugs))
	for i, slug := range slugs {
		out[i] = driving.DoctorPassedCheck{Check: slug, Description: "ok"}
	}
	return out
}

// TestRun_Text_HealthyWorkspace_DefaultMode_Golden verifies the healthy default
// output matches the spec's example byte-for-byte (modulo version). AC1.
func TestRun_Text_HealthyWorkspace_DefaultMode_Golden(t *testing.T) {
	t.Parallel()

	// Given — all 16 checks passed.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{},
			Passed:   allChecksPassed(),
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When
	if err := run(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()

	// Then — output matches the spec's healthy-state example byte-for-byte.
	want := "np admin doctor — np v0.3.0\n" +
		"\n" +
		"✓ Database          7 checks passed\n" +
		"✓ Environment       2 checks passed\n" +
		"✓ Graph health      5 checks passed\n" +
		"✓ Issue lifecycle   2 checks passed\n" +
		"\n" +
		"16 checks, 0 findings.\n"
	if got != want {
		t.Errorf("healthy output mismatch:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

// TestRun_Text_TwoFindings_DefaultMode_Golden verifies the spec's two-finding
// example (one warning + one error) byte-for-byte, including alphabetical
// finding order across categories and the correct fix-block labels. AC2.
func TestRun_Text_TwoFindings_DefaultMode_Golden(t *testing.T) {
	t.Parallel()

	// Given — fixture matching the spec's "two findings" example (1 warning,
	// 1 error). Database has 6 passed + 1 error; Graph health has 4 passed +
	// 1 warning; Environment and Issue lifecycle fully pass.
	passed := []driving.DoctorPassedCheck{
		{Check: "dot-np-directory"},
		{Check: "database-exists"},
		{Check: "storage-integrity"},
		{Check: "schema-version"},
		{Check: "column-data-validity"},
		{Check: "closed-parent-with-open-child"},
		{Check: "agent-instructions"},
		{Check: "git-ignore"},
		{Check: "blocked-by-ancestor"},
		{Check: "blocked-by-closable-issue"},
		{Check: "blocker-cycles"},
		{Check: "priority-inversions"},
		{Check: "closable-parent-issues"},
		{Check: "long-deferrals"},
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue references a parent that does not exist.",
					Fix:     driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "blocked-by-deferred-issue",
					Summary: "3 issues are blocked by issues that have been deferred.",
					Fix:     driving.DoctorFix{Instructions: "Claim one of the deferred blocking issues, complete it, then close it."},
				},
			},
			Passed: passed,
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	got := stdout.String()

	// Then — alphabetical order: "Blocked by deferred issue" precedes
	// "Invalid parent reference"; correct fix-block labels.
	want := "np admin doctor — np v0.3.0\n" +
		"\n" +
		"✗ Database          6 of 7 checks passed\n" +
		"✓ Environment       2 checks passed\n" +
		"! Graph health      4 of 5 checks passed\n" +
		"✓ Issue lifecycle   2 checks passed\n" +
		"\n" +
		"! Blocked by deferred issue (graph health)\n" +
		"  3 issues are blocked by issues that have been deferred.\n" +
		"  To fix:\n" +
		"    Claim one of the deferred blocking issues, complete it, then close it.\n" +
		"\n" +
		"✗ Invalid parent reference (database)\n" +
		"  1 issue references a parent that does not exist.\n" +
		"  To fix, run:\n" +
		"    np admin fix invalid-parent-reference --author <name>\n" +
		"\n" +
		"16 checks, 2 findings (1 warning, 1 error).\n"
	if got != want {
		t.Errorf("two-findings output mismatch:\n--- want ---\n%s--- got ---\n%s", want, got)
	}
}

// TestRun_Text_HealthyWorkspace_DefaultMode verifies the banner, category
// status block, and trailing summary for a fully healthy workspace using the
// fully-populated fixture (so AC4 count strings are real, not "0 of 0").
func TestRun_Text_HealthyWorkspace_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — all 16 checks pass with realistic Passed entries.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{},
			Passed:   allChecksPassed(),
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "1.2.3",
		IOStreams:   ios,
	}

	// When
	if err := run(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()

	// Then — banner present.
	if !strings.Contains(out, "np admin doctor — np v1.2.3") {
		t.Errorf("banner not found in output:\n%s", out)
	}
	// Trailing healthy summary with registry size of 16.
	if !strings.Contains(out, "16 checks, 0 findings.") {
		t.Errorf("healthy summary line not found in output:\n%s", out)
	}
	// Each category shows its actual count: 7/2/5/2.
	for _, want := range []string{
		"Database          7 checks passed",
		"Environment       2 checks passed",
		"Graph health      5 checks passed",
		"Issue lifecycle   2 checks passed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected category line %q not in output:\n%s", want, out)
		}
	}
	// No findings section.
	if strings.Contains(out, "To fix") {
		t.Errorf("unexpected 'To fix' in healthy output:\n%s", out)
	}
}

// TestRun_Text_SingleWarning_DefaultMode verifies that a single warning finding
// is rendered with the correct heading format and fix block.
func TestRun_Text_SingleWarning_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — one warning from blocked-by-deferred-issue.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "blocked-by-deferred-issue",
					Summary: "3 issues are blocked by issues that have been deferred.",
					Fix:     driving.DoctorFix{Instructions: "Claim one of the deferred blocking issues, complete it, then close it."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — warnings only → exit 1.
	assertExitCode(t, run(t.Context(), input), 1)
	out := stdout.String()

	// Then — heading uses resolved title (not slug).
	if !strings.Contains(out, "Blocked by deferred issue") {
		t.Errorf("resolved title not found:\n%s", out)
	}
	// Category label in heading (lowercase).
	if !strings.Contains(out, "(graph health)") {
		t.Errorf("category label not found:\n%s", out)
	}
	// Summary line.
	if !strings.Contains(out, "3 issues are blocked by issues that have been deferred.") {
		t.Errorf("summary not found:\n%s", out)
	}
	// Instruction fix block.
	if !strings.Contains(out, "To fix:") {
		t.Errorf("fix label 'To fix:' not found:\n%s", out)
	}
	// Summary line.
	if !strings.Contains(out, "1 finding") {
		t.Errorf("trailing summary not found:\n%s", out)
	}
}

// TestRun_Text_SingleError_DefaultMode verifies that a single error finding is
// rendered with the command fix block label "To fix, run:".
func TestRun_Text_SingleError_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — one error from invalid-parent-reference.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue references a parent that does not exist.",
					Fix:     driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	out := stdout.String()

	// Then — resolved title.
	if !strings.Contains(out, "Invalid parent reference") {
		t.Errorf("resolved title not found:\n%s", out)
	}
	// Category label.
	if !strings.Contains(out, "(database)") {
		t.Errorf("category label not found:\n%s", out)
	}
	// Command fix label.
	if !strings.Contains(out, "To fix, run:") {
		t.Errorf("fix label 'To fix, run:' not found:\n%s", out)
	}
	if !strings.Contains(out, "np admin fix invalid-parent-reference") {
		t.Errorf("fix command not found:\n%s", out)
	}
	// Error-count in summary.
	if !strings.Contains(out, "1 error") {
		t.Errorf("error count not in summary:\n%s", out)
	}
}

// TestRun_Text_Mixed_DefaultMode verifies that with both a warning and an error
// the findings section is sorted alphabetically by resolved title.
func TestRun_Text_Mixed_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — one error (invalid-parent-reference, title "Invalid parent reference")
	// and one warning (blocked-by-deferred-issue, title "Blocked by deferred issue").
	// Alphabetically "Blocked" < "Invalid", so warning should appear first.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "invalid-parent-reference",
					Summary: "1 issue has dangling parent.",
					Fix:     driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "blocked-by-deferred-issue",
					Summary: "2 issues blocked by deferred.",
					Fix:     driving.DoctorFix{Instructions: "Claim the deferred blocker."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	out := stdout.String()

	// Then — both titles present.
	posBlocked := strings.Index(out, "Blocked by deferred issue")
	posInvalid := strings.Index(out, "Invalid parent reference")
	if posBlocked < 0 || posInvalid < 0 {
		t.Fatalf("expected both titles in output:\n%s", out)
	}
	// "Blocked" appears before "Invalid" (alphabetical order).
	if posBlocked > posInvalid {
		t.Errorf("findings not sorted alphabetically: Blocked at %d, Invalid at %d\n%s", posBlocked, posInvalid, out)
	}

	// Trailing summary counts both.
	if !strings.Contains(out, "2 findings") {
		t.Errorf("finding count not in summary:\n%s", out)
	}
	if !strings.Contains(out, "1 warning") {
		t.Errorf("warning count not in summary:\n%s", out)
	}
	if !strings.Contains(out, "1 error") {
		t.Errorf("error count not in summary:\n%s", out)
	}
}

// TestRun_Text_CascadeTruncated_DefaultMode verifies that when checks are
// skipped the trailing summary line contains the "M of N checks ran … K checks
// skipped" form.
func TestRun_Text_CascadeTruncated_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — database-exists fails; several checks are skipped.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:   "database-exists",
					Summary: "Database file is missing.",
					Fix:     driving.DoctorFix{Instructions: "Restore from backup."},
				},
			},
			Warnings: []driving.DoctorFinding{},
			Passed: []driving.DoctorPassedCheck{
				{Check: "dot-np-directory", Description: "Confirms .np/ directory exists."},
				{Check: "agent-instructions", Description: "Checks CLAUDE.md."},
				{Check: "git-ignore", Description: "Checks .gitignore."},
			},
			Skipped: []driving.DoctorSkippedCheck{
				{Check: "storage-integrity", Prerequisite: "database-exists"},
				{Check: "schema-version", Prerequisite: "database-exists"},
				{Check: "column-data-validity", Prerequisite: "database-exists"},
				{Check: "closed-parent-with-open-child", Prerequisite: "database-exists"},
				{Check: "invalid-parent-reference", Prerequisite: "database-exists"},
				{Check: "blocked-by-ancestor", Prerequisite: "database-exists"},
				{Check: "blocked-by-closable-issue", Prerequisite: "database-exists"},
				{Check: "blocked-by-deferred-issue", Prerequisite: "database-exists"},
				{Check: "blocker-cycles", Prerequisite: "database-exists"},
				{Check: "priority-inversions", Prerequisite: "database-exists"},
				{Check: "closable-parent-issues", Prerequisite: "database-exists"},
				{Check: "long-deferrals", Prerequisite: "database-exists"},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	out := stdout.String()

	// Then — "M of N checks ran" format.
	if !strings.Contains(out, "of 16 checks ran") {
		t.Errorf("cascade summary format not found:\n%s", out)
	}
	// Skipped count present.
	if !strings.Contains(out, "checks skipped") {
		t.Errorf("skipped count not in summary:\n%s", out)
	}
	if !strings.Contains(out, "cascade prerequisite failed") {
		t.Errorf("cascade reason not in summary:\n%s", out)
	}
}

// TestRun_Text_SeverityErrorFilter_DefaultMode verifies that with
// --severity error the warning finding is hidden from the output body but
// the trailing summary still reports "at or above severity error" when no
// errors exist.
func TestRun_Text_SeverityErrorFilter_DefaultMode(t *testing.T) {
	t.Parallel()

	// Given — only a warning exists; severity filter set to error.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{
				{
					Check:   "long-deferrals",
					Summary: "2 issues are long-deferred.",
					Fix:     driving.DoctorFix{Instructions: "Review each."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityError,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — warnings in unfiltered results → exit 1 even with --severity error.
	assertExitCode(t, run(t.Context(), input), 1)
	out := stdout.String()

	// Then — warning finding body is absent (filtered out).
	if strings.Contains(out, "Long deferrals") {
		t.Errorf("warning finding should be hidden with --severity error:\n%s", out)
	}
	// Summary names the filter.
	if !strings.Contains(out, "at or above severity error") {
		t.Errorf("severity filter message not in summary:\n%s", out)
	}
}

// TestRun_Text_VerboseMode verifies that verbose output groups checks by
// category, shows every check's description, and expands finding details inline.
func TestRun_Text_VerboseMode(t *testing.T) {
	t.Parallel()

	// Given — one error (invalid-parent-reference) plus several passed checks.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{
				{
					Check:        "invalid-parent-reference",
					Description:  "Detects issues that have a parent relationship, but the parent issue does not exist.",
					WhyItMatters: "This is a data integrity error.",
					Summary:      "1 issue references a parent that does not exist.",
					Affected: []any{
						driving.InvalidParentReferenceRow{Issue: "NP-aaa01", MissingParentID: "NP-bbb02"},
					},
					Fix: driving.DoctorFix{Command: "np admin fix invalid-parent-reference --author <name>"},
				},
			},
			Warnings: []driving.DoctorFinding{},
			Passed: []driving.DoctorPassedCheck{
				{Check: "dot-np-directory", Description: "Confirms a .np/ directory exists."},
				{Check: "database-exists", Description: "Confirms the database file exists."},
				{Check: "storage-integrity", Description: "Verifies the SQLite file is structurally intact."},
				{Check: "schema-version", Description: "Checks the schema version."},
				{Check: "column-data-validity", Description: "Verifies column data validity."},
				{Check: "closed-parent-with-open-child", Description: "Detects closed parents with open children."},
				{Check: "agent-instructions", Description: "Checks agent instructions file."},
				{Check: "git-ignore", Description: "Checks .gitignore."},
				{Check: "blocked-by-ancestor", Description: "Detects blocked-by-ancestor."},
				{Check: "blocked-by-closable-issue", Description: "Detects blocked-by-closable."},
				{Check: "blocked-by-deferred-issue", Description: "Detects blocked-by-deferred."},
				{Check: "blocker-cycles", Description: "Detects blocker cycles."},
				{Check: "priority-inversions", Description: "Detects priority inversions."},
				{Check: "closable-parent-issues", Description: "Detects closable parents."},
				{Check: "long-deferrals", Description: "Detects long deferrals."},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Verbose:     true,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — errors present → exit 2.
	assertExitCode(t, run(t.Context(), input), 2)
	out := stdout.String()

	// Then — all four category headers present.
	for _, cat := range []string{"Database", "Environment", "Graph health", "Issue lifecycle"} {
		if !strings.Contains(out, "\n"+cat+"\n") {
			t.Errorf("category header %q not found as standalone line:\n%s", cat, out)
		}
	}
	// Failing check has inline expansion.
	if !strings.Contains(out, "Why it matters:") {
		t.Errorf("why-it-matters not found in verbose output:\n%s", out)
	}
	// Affected row rendered.
	if !strings.Contains(out, "NP-aaa01") {
		t.Errorf("affected row issue not found:\n%s", out)
	}
	if !strings.Contains(out, "missing parent NP-bbb02") {
		t.Errorf("affected row missing_parent not found:\n%s", out)
	}
	// Fix block.
	if !strings.Contains(out, "To fix, run:") {
		t.Errorf("fix label not found:\n%s", out)
	}
}

// TestRun_Text_VerboseMode_NoCategoryStatusBlock verifies that verbose mode
// does not render the per-category status block (banner → per-category section
// directly per the spec example).
func TestRun_Text_VerboseMode_NoCategoryStatusBlock(t *testing.T) {
	t.Parallel()

	// Given — a healthy workspace in verbose mode.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors:   []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{},
			Passed:   allChecksPassed(),
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Verbose:     true,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When
	if err := run(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()

	// Then — banner is followed by a blank line then the "Database" subheading,
	// not the status-block table line "✓ Database  N checks passed".
	if strings.Contains(out, "✓ Database          7 checks passed") {
		t.Errorf("verbose mode must not include the category status block:\n%s", out)
	}
	// Database subheading appears as a standalone line.
	if !strings.Contains(out, "\nDatabase\n") {
		t.Errorf("verbose mode must include 'Database' as a standalone subheading:\n%s", out)
	}
}

// TestRun_Text_VerboseMode_SeverityErrorFilter_HidesWarnings verifies AC6's
// "from both default and verbose output" — warnings are hidden in verbose mode
// when --severity error is set, but the trailing summary still names the filter.
func TestRun_Text_VerboseMode_SeverityErrorFilter_HidesWarnings(t *testing.T) {
	t.Parallel()

	// Given — only a warning exists; severity filter is error; verbose mode.
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{
				{
					Check:        "long-deferrals",
					Description:  "Detects issues that have been deferred and not updated for an excessive period.",
					WhyItMatters: "Long-untouched deferred issues are usually forgotten work.",
					Summary:      "2 issues are long-deferred.",
					Affected: []any{
						driving.LongDeferralRow{Issue: "NP-stale1"},
					},
					Fix: driving.DoctorFix{Instructions: "Review each."},
				},
			},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityError,
		Verbose:     true,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — warnings in unfiltered results → exit 1 even with --severity error.
	assertExitCode(t, run(t.Context(), input), 1)
	out := stdout.String()

	// Then — the warning's expansion is absent (Why it matters / Affected /
	// fix instructions are not shown for filtered warnings).
	if strings.Contains(out, "Why it matters") {
		t.Errorf("warning expansion must be hidden with --severity error in verbose:\n%s", out)
	}
	if strings.Contains(out, "NP-stale1") {
		t.Errorf("warning's affected row must be hidden with --severity error:\n%s", out)
	}
	if strings.Contains(out, "Review each.") {
		t.Errorf("warning's fix text must be hidden with --severity error:\n%s", out)
	}
	// Trailing summary names the filter.
	if !strings.Contains(out, "at or above severity error") {
		t.Errorf("severity filter message not in verbose summary:\n%s", out)
	}
}

// TestRun_Text_VerboseMode_AffectedCapAt3 verifies that in verbose text output
// the affected-issues list is capped at 3 rows with the overflow line.
func TestRun_Text_VerboseMode_AffectedCapAt3(t *testing.T) {
	t.Parallel()

	// Given — a finding with 5 affected rows.
	affected := make([]any, 5)
	for i := range affected {
		affected[i] = driving.ClosableParentIssueRow{Issue: fmt.Sprintf("NP-%05d", i+1)}
	}
	doctorFunc := func(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
		return driving.DoctorOutput{
			Errors: []driving.DoctorFinding{},
			Warnings: []driving.DoctorFinding{{
				Check:    "closable-parent-issues",
				Summary:  "5 issues can be closed.",
				Affected: affected,
				Fix:      driving.DoctorFix{Command: "np epic close-completed"},
			}},
		}, nil
	}
	ios, stdout := newTestStreams()
	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		Verbose:     true,
		Version:     "0.3.0",
		IOStreams:   ios,
	}

	// When — warnings only → exit 1.
	assertExitCode(t, run(t.Context(), input), 1)
	out := stdout.String()

	// Then — exactly 3 affected rows visible plus the overflow line.
	if !strings.Contains(out, "NP-00001") {
		t.Errorf("first affected row not found:\n%s", out)
	}
	if !strings.Contains(out, "NP-00003") {
		t.Errorf("third affected row not found:\n%s", out)
	}
	if strings.Contains(out, "NP-00004") {
		t.Errorf("fourth row should be hidden (capped at 3):\n%s", out)
	}
	// Overflow line must match the spec exactly: em dash (U+2014), 8-space
	// indent, joined wording. This pins the format so cosmetic regressions
	// (hyphen-vs-em-dash, indent shifts) fail the test.
	wantOverflow := "        (and 2 more — see --json output for the complete list)"
	if !strings.Contains(out, wantOverflow) {
		t.Errorf("expected exact overflow line %q in output:\n%s", wantOverflow, out)
	}
}

// --- run tests: long-deferral threshold wiring ---

// TestRun_LongDeferralThreshold_PassedToDoctorFunc verifies that the threshold
// supplied via runInput is forwarded verbatim to the DoctorFunc via DoctorInput.
func TestRun_LongDeferralThreshold_PassedToDoctorFunc(t *testing.T) {
	t.Parallel()

	// Given — a DoctorFunc spy that records the input it was called with.
	var capturedInput driving.DoctorInput
	doctorFunc := func(_ context.Context, in driving.DoctorInput) (driving.DoctorOutput, error) {
		capturedInput = in
		return stubDoctorOutput, nil
	}
	ios, _ := newTestStreams()
	wantThreshold := 14 * 24 * time.Hour

	input := runInput{
		DoctorFunc:            doctorFunc,
		MinSeverity:           driving.SeverityWarning,
		IOStreams:             ios,
		LongDeferralThreshold: wantThreshold,
	}

	// When
	if err := run(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Then — DoctorInput.LongDeferralThreshold matches what was supplied.
	if capturedInput.LongDeferralThreshold != wantThreshold {
		t.Errorf("LongDeferralThreshold: got %v, want %v", capturedInput.LongDeferralThreshold, wantThreshold)
	}
}

// TestRun_LongDeferralThreshold_ZeroValueForwarded verifies that a zero
// threshold is forwarded as zero, allowing the service layer to apply its
// own default.
func TestRun_LongDeferralThreshold_ZeroValueForwarded(t *testing.T) {
	t.Parallel()

	// Given — a DoctorFunc spy and runInput with no threshold set.
	var capturedInput driving.DoctorInput
	doctorFunc := func(_ context.Context, in driving.DoctorInput) (driving.DoctorOutput, error) {
		capturedInput = in
		return stubDoctorOutput, nil
	}
	ios, _ := newTestStreams()

	input := runInput{
		DoctorFunc:  doctorFunc,
		MinSeverity: driving.SeverityWarning,
		IOStreams:   ios,
	}

	// When
	if err := run(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Then — zero is forwarded as zero (service layer applies the default).
	if capturedInput.LongDeferralThreshold != 0 {
		t.Errorf("LongDeferralThreshold: got %v, want 0", capturedInput.LongDeferralThreshold)
	}
}

// --- parseLongDeferralThreshold tests ---
//
// These tests exercise the helper directly. Flag-vs-env-var precedence is
// handled by urfave/cli (cli.EnvVars on the StringFlag), so the helper sees
// only the merged value urfave produces.

// TestParseLongDeferralThreshold_ValidValue_Parsed verifies a valid extended
// duration is parsed correctly.
func TestParseLongDeferralThreshold_ValidValue_Parsed(t *testing.T) {
	t.Parallel()

	// Given — a valid 2-week duration string.
	// When
	got, err := parseLongDeferralThreshold("2w")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 14 * 24 * time.Hour
	if got != want {
		t.Errorf("threshold: got %v, want %v", got, want)
	}
}

// TestParseLongDeferralThreshold_EmptyValue_ReturnsZero verifies the empty
// string yields zero so the service layer can apply the 7-day default.
func TestParseLongDeferralThreshold_EmptyValue_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — an empty value (neither flag nor env var set).
	// When
	got, err := parseLongDeferralThreshold("")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("threshold: got %v, want 0 (so service default applies)", got)
	}
}

// TestParseLongDeferralThreshold_InvalidValue_ErrorNamesFlag verifies that
// an invalid value yields an error tagged with the flag name. The flag
// label is the one users will recognise even when the value came from the
// env var, because urfave/cli treats the env var as a flag source.
func TestParseLongDeferralThreshold_InvalidValue_ErrorNamesFlag(t *testing.T) {
	t.Parallel()

	// Given — an unparseable duration string.
	// When
	_, err := parseLongDeferralThreshold("nonsense")
	// Then — error names the flag and includes the invalid input.
	if err == nil {
		t.Fatal("expected error for invalid value, got nil")
	}
	if !strings.Contains(err.Error(), "--long-deferral-threshold") {
		t.Errorf("error should name the flag, got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("error should include the invalid input, got: %q", err.Error())
	}
}
