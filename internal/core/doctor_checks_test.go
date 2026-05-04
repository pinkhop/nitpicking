package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

// findingStubRun returns a Run function that emits a single finding with the
// given summary. The orchestrator classifies the finding as error or warning
// based on the registry entry's Severity, not on the stub itself — so this
// helper is shared by both error-severity and warning-severity test scenarios.
func findingStubRun(summary string) func(context.Context, *serviceImpl, driving.DoctorInput) (*doctorRunResult, error) {
	return func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return &doctorRunResult{Summary: summary}, nil
	}
}

// newerDBStubRun returns a Run function that emits a schema-version finding
// with a doctorSchemaNewerDBMarker, triggering the SkipsAll path.
func newerDBStubRun() func(context.Context, *serviceImpl, driving.DoctorInput) (*doctorRunResult, error) {
	return func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return &doctorRunResult{
			Summary:  "database version is newer than the binary version",
			Affected: []any{doctorSchemaNewerDBMarker{}},
		}, nil
	}
}

// olderDBStubRun returns a Run function that emits a schema-version finding
// WITHOUT a newer-DB marker, simulating the older-database case.
func olderDBStubRun() func(context.Context, *serviceImpl, driving.DoctorInput) (*doctorRunResult, error) {
	return func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return &doctorRunResult{
			Summary:  "database version is older than the binary version — run np admin upgrade",
			Affected: nil,
		}, nil
	}
}

// overrideRun returns a copy of the registry with the entry matching slug
// having its Run replaced. Panics if slug is not found.
func overrideRun(
	registry []doctorCheckEntry,
	slug string,
	run func(context.Context, *serviceImpl, driving.DoctorInput) (*doctorRunResult, error),
) []doctorCheckEntry {
	result := make([]doctorCheckEntry, len(registry))
	copy(result, registry)
	for i, e := range result {
		if e.Slug == slug {
			result[i].Run = run
			return result
		}
	}
	panic("overrideRun: slug not found: " + slug)
}

// newTestSvc returns a serviceImpl with a nil transactor. Safe for doctor
// tests because all stub Run functions and test-injected Run functions return
// results without accessing svc.tx.
func newTestSvc() *serviceImpl {
	return &serviceImpl{tx: nil}
}

// doctorTestInput returns a DoctorInput pre-populated with a temporary
// workspace that satisfies the real dot-np-directory, database-exists,
// agent-instructions, and git-ignore check implementations. Tests that override
// intermediate checks still need the pre-checks to pass so they can exercise
// their target cascade trigger.
func doctorTestInput(t *testing.T) driving.DoctorInput {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np directory: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatalf("precondition: create fake db file: %v", err)
	}
	// Provide a minimal git environment so git-ignore passes without relying on
	// the project's own .git and .gitignore.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".np/\n"), 0o600); err != nil {
		t.Fatalf("precondition: create .gitignore: %v", err)
	}
	// Provide a CLAUDE.md so agent-instructions passes.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Use np to track issues.\n"), 0o600); err != nil {
		t.Fatalf("precondition: create CLAUDE.md: %v", err)
	}
	return driving.DoctorInput{WorkDir: dir, DBPath: dbPath}
}

// passedSlugs extracts the check slugs from a Passed list.
func passedSlugs(passed []driving.DoctorPassedCheck) []string {
	slugs := make([]string, len(passed))
	for i, p := range passed {
		slugs[i] = p.Check
	}
	return slugs
}

// skippedSlugs extracts the check slugs from a Skipped list.
func skippedSlugs(skipped []driving.DoctorSkippedCheck) []string {
	slugs := make([]string, len(skipped))
	for i, s := range skipped {
		slugs[i] = s.Check
	}
	return slugs
}

// errorSlugs extracts the check slugs from an Errors list.
func errorSlugs(errors []driving.DoctorFinding) []string {
	slugs := make([]string, len(errors))
	for i, e := range errors {
		slugs[i] = e.Check
	}
	return slugs
}

// --- Display order ---

func TestDoctorDisplayOrder_ReturnsSpecMandated16Slugs(t *testing.T) {
	t.Parallel()

	// Given — the canonical display-order helper.

	// When
	order := doctorDisplayOrder()

	// Then — exactly the 16 spec-mandated slugs in the specified order.
	want := []string{
		// Database: cascade-five (prerequisite order, NOT alphabetical).
		"dot-np-directory",
		"database-exists",
		"storage-integrity",
		"schema-version",
		"column-data-validity",
		// Database: remaining (alphabetical by title).
		"closed-parent-with-open-child",
		"invalid-parent-reference",
		// Environment (alphabetical by title).
		"agent-instructions",
		"git-ignore",
		// Graph health (alphabetical by title).
		"blocked-by-ancestor",
		"blocked-by-closable-issue",
		"blocked-by-deferred-issue",
		"blocker-cycles",
		"priority-inversions",
		// Issue lifecycle (alphabetical by title).
		"closable-parent-issues",
		"long-deferrals",
	}
	if !slices.Equal(order, want) {
		t.Errorf("display order mismatch\ngot:  %v\nwant: %v", order, want)
	}
}

func TestDoctorDisplayOrder_DatabaseCascadeFiveFirst(t *testing.T) {
	t.Parallel()

	// Given — the canonical display-order helper.

	// When
	order := doctorDisplayOrder()

	// Then — the first five slugs are the cascade prerequisites in order.
	cascadeFive := []string{
		"dot-np-directory",
		"database-exists",
		"storage-integrity",
		"schema-version",
		"column-data-validity",
	}
	if len(order) < 5 {
		t.Fatalf("display order has %d entries, need at least 5", len(order))
	}
	for i, want := range cascadeFive {
		if order[i] != want {
			t.Errorf("position %d: got %q, want %q", i, order[i], want)
		}
	}
}

// --- All checks pass on a healthy workspace ---

func TestDoctorChecks_AllChecksPass_AllSixteenPassed(t *testing.T) {
	t.Parallel()

	// Given — all 16 Run functions return nil against a minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := doctorRegistry()
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — zero errors/warnings/skipped, 16 passed.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if len(out.Errors) != 0 {
		t.Errorf("errors: got %d, want 0: %v", len(out.Errors), errorSlugs(out.Errors))
	}
	if len(out.Warnings) != 0 {
		t.Errorf("warnings: got %d, want 0", len(out.Warnings))
	}
	if len(out.Skipped) != 0 {
		t.Errorf("skipped: got %d, want 0: %v", len(out.Skipped), skippedSlugs(out.Skipped))
	}
	if got := len(out.Passed); got != 16 {
		t.Errorf("passed: got %d, want 16: %v", got, passedSlugs(out.Passed))
	}
}

func TestDoctorChecks_AllChecksPass_PassedInDisplayOrder(t *testing.T) {
	t.Parallel()

	// Given — all Run functions return nil on a minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := doctorRegistry()
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — Passed list follows the canonical display order.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	displayOrder := doctorDisplayOrder()
	gotSlugs := passedSlugs(out.Passed)
	if !slices.Equal(gotSlugs, displayOrder) {
		t.Errorf("passed order mismatch\ngot:  %v\nwant: %v", gotSlugs, displayOrder)
	}
}

// --- Cascade trigger 1: dot-np-directory ---

func TestDoctorChecks_DotNpDirectoryFails_SkipsAllDbGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — dot-np-directory returns an error finding.
	svc := newTestSvc()
	registry := overrideRun(doctorRegistry(), "dot-np-directory", findingStubRun("forced error"))
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — 13 checks skipped, 2 environment checks pass, 1 error.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1: %v", got, errorSlugs(out.Errors))
	}
	if out.Errors[0].Check != "dot-np-directory" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "dot-np-directory")
	}
	wantPassed := []string{"agent-instructions", "git-ignore"}
	gotPassed := passedSlugs(out.Passed)
	if !slices.Equal(gotPassed, wantPassed) {
		t.Errorf("passed: got %v, want %v", gotPassed, wantPassed)
	}
	wantSkippedCount := 13
	if got := len(out.Skipped); got != wantSkippedCount {
		t.Errorf("skipped: got %d, want %d: %v", got, wantSkippedCount, skippedSlugs(out.Skipped))
	}
}

func TestDoctorChecks_DotNpDirectoryFails_SkippedEntriesHaveCorrectPrereq(t *testing.T) {
	t.Parallel()

	// Given — dot-np-directory returns an error finding.
	svc := newTestSvc()
	registry := overrideRun(doctorRegistry(), "dot-np-directory", findingStubRun("forced error"))
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — every Skipped entry has Prerequisite = "dot-np-directory".
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	for _, s := range out.Skipped {
		if s.Prerequisite != "dot-np-directory" {
			t.Errorf("skipped %q: Prerequisite = %q, want %q", s.Check, s.Prerequisite, "dot-np-directory")
		}
	}
}

// --- Cascade trigger 2: database-exists ---

func TestDoctorChecks_DatabaseExistsFails_SkipsRemainingDbGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — database-exists returns an error finding.
	svc := newTestSvc()
	registry := overrideRun(doctorRegistry(), "database-exists", findingStubRun("forced error"))
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — 12 checks skipped; dot-np-directory + 2 environment checks pass.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1: %v", got, errorSlugs(out.Errors))
	}
	if out.Errors[0].Check != "database-exists" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "database-exists")
	}
	wantPassed := []string{"dot-np-directory", "agent-instructions", "git-ignore"}
	gotPassed := passedSlugs(out.Passed)
	if !slices.Equal(gotPassed, wantPassed) {
		t.Errorf("passed: got %v, want %v", gotPassed, wantPassed)
	}
	if got := len(out.Skipped); got != 12 {
		t.Errorf("skipped: got %d, want 12: %v", got, skippedSlugs(out.Skipped))
	}
	for _, s := range out.Skipped {
		if s.Prerequisite != "database-exists" {
			t.Errorf("skipped %q: Prerequisite = %q, want %q", s.Check, s.Prerequisite, "database-exists")
		}
	}
}

// --- Cascade trigger 3: storage-integrity ---

func TestDoctorChecks_StorageIntegrityFails_SkipsRemainingDbGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — storage-integrity returns an error finding; earlier checks pass
	// on the minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := overrideRun(doctorRegistry(), "storage-integrity", findingStubRun("forced error"))
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — 11 checks skipped; dot-np-directory, database-exists, environment pass.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "storage-integrity" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "storage-integrity")
	}
	wantPassed := []string{"dot-np-directory", "database-exists", "agent-instructions", "git-ignore"}
	if !slices.Equal(passedSlugs(out.Passed), wantPassed) {
		t.Errorf("passed: got %v, want %v", passedSlugs(out.Passed), wantPassed)
	}
	if got := len(out.Skipped); got != 11 {
		t.Errorf("skipped: got %d, want 11: %v", got, skippedSlugs(out.Skipped))
	}
	for _, s := range out.Skipped {
		if s.Prerequisite != "storage-integrity" {
			t.Errorf("skipped %q: Prerequisite = %q, want %q", s.Check, s.Prerequisite, "storage-integrity")
		}
	}
}

// --- Cascade trigger 4a: schema-version (newer DB — skips everything) ---

func TestDoctorChecks_SchemaVersionNewerDB_SkipsAllRemainingIncludingEnvironment(t *testing.T) {
	t.Parallel()

	// Given — schema-version returns a newer-DB finding (SkipsAll applies);
	// earlier checks pass on the minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := overrideRun(doctorRegistry(), "schema-version", newerDBStubRun())
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — 12 checks skipped (everything after schema-version, including environment).
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "schema-version" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "schema-version")
	}
	// Only the 3 checks that ran before schema-version should be in Passed.
	wantPassed := []string{"dot-np-directory", "database-exists", "storage-integrity"}
	if !slices.Equal(passedSlugs(out.Passed), wantPassed) {
		t.Errorf("passed: got %v, want %v", passedSlugs(out.Passed), wantPassed)
	}
	if got := len(out.Skipped); got != 12 {
		t.Errorf("skipped: got %d, want 12: %v", got, skippedSlugs(out.Skipped))
	}
	for _, s := range out.Skipped {
		if s.Prerequisite != "schema-version" {
			t.Errorf("skipped %q: Prerequisite = %q, want %q", s.Check, s.Prerequisite, "schema-version")
		}
	}
}

// --- Cascade trigger 4b: schema-version (older DB — skips nothing) ---

// TestDoctorChecks_SchemaVersionOlderDB_SkipsNothing verifies the spec's
// asymmetry: an older-DB error skips nothing outright. All other 15 checks
// still run and pass against the healthy fake database.
func TestDoctorChecks_SchemaVersionOlderDB_SkipsNothing(t *testing.T) {
	t.Parallel()

	// Given — schema-version returns an older-DB error; earlier checks pass on
	// the minimal healthy workspace; remaining stubs return nil.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := overrideRun(doctorRegistry(), "schema-version", olderDBStubRun())
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — schema-version emits an error but all other 15 checks still pass.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1: %v", got, errorSlugs(out.Errors))
	}
	if out.Errors[0].Check != "schema-version" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "schema-version")
	}
	if got := len(out.Skipped); got != 0 {
		t.Errorf("skipped: got %d, want 0: %v", got, skippedSlugs(out.Skipped))
	}
	if got := len(out.Passed); got != 15 {
		t.Errorf("passed: got %d, want 15: %v", got, passedSlugs(out.Passed))
	}
}

// --- Cascade trigger 5: column-data-validity ---

func TestDoctorChecks_ColumnDataValidityFails_SkipsRemainingDbGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — column-data-validity returns an error finding; earlier checks
	// pass on the minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := overrideRun(doctorRegistry(), "column-data-validity", findingStubRun("forced error"))
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — 9 checks skipped; database + schema-version + environment pass.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "column-data-validity" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "column-data-validity")
	}
	wantPassed := []string{
		"dot-np-directory", "database-exists", "storage-integrity", "schema-version",
		"agent-instructions", "git-ignore",
	}
	if !slices.Equal(passedSlugs(out.Passed), wantPassed) {
		t.Errorf("passed: got %v, want %v", passedSlugs(out.Passed), wantPassed)
	}
	if got := len(out.Skipped); got != 9 {
		t.Errorf("skipped: got %d, want 9: %v", got, skippedSlugs(out.Skipped))
	}
	for _, s := range out.Skipped {
		if s.Prerequisite != "column-data-validity" {
			t.Errorf("skipped %q: Prerequisite = %q, want %q", s.Check, s.Prerequisite, "column-data-validity")
		}
	}
}

// --- MinSeverity filtering ---

// TestDoctorChecks_MinSeverityError_WarningStillInWarnings verifies that
// MinSeverity filtering never affects Errors/Warnings population; findings
// are always fully populated and filtering is left to the CLI renderer.
func TestDoctorChecks_MinSeverityError_WarningFindingPreservedInWarnings(t *testing.T) {
	t.Parallel()

	// Given — a warning-severity check (agent-instructions) returns a finding;
	// the cascade prereqs pass on the minimal healthy workspace.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})
	registry := overrideRun(doctorRegistry(), "agent-instructions", findingStubRun("no instruction file found"))
	baseInput := doctorTestInput(t)

	// When — MinSeverity = error (would filter warnings in the CLI).
	baseInput.MinSeverity = driving.SeverityError
	out, err := runDoctorChecks(t.Context(), svc, registry, baseInput)
	// Then — warning finding is still in Warnings; check is not in Passed.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Warnings); got != 1 {
		t.Errorf("warnings: got %d, want 1", got)
	}
	if len(out.Warnings) > 0 && out.Warnings[0].Check != "agent-instructions" {
		t.Errorf("warning check: got %q, want %q", out.Warnings[0].Check, "agent-instructions")
	}
	for _, p := range out.Passed {
		if p.Check == "agent-instructions" {
			t.Errorf("agent-instructions appeared in Passed but it had a warning finding")
		}
	}
}

// TestDoctorChecks_MinSeverityWarning_ErrorFindingPreservedInErrors verifies
// that an error-severity finding is always in Errors regardless of MinSeverity.
func TestDoctorChecks_MinSeverityWarning_ErrorFindingPreservedInErrors(t *testing.T) {
	t.Parallel()

	// Given — dot-np-directory (error severity) returns a finding.
	svc := newTestSvc()
	registry := overrideRun(doctorRegistry(), "dot-np-directory", findingStubRun("forced error"))
	input := doctorTestInput(t)
	input.MinSeverity = driving.SeverityWarning

	// When — MinSeverity = warning (same as default; should not affect results).
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — error finding is in Errors; check is not in Passed.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Errorf("errors: got %d, want 1", got)
	}
	if len(out.Errors) > 0 && out.Errors[0].Check != "dot-np-directory" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "dot-np-directory")
	}
	for _, p := range out.Passed {
		if p.Check == "dot-np-directory" {
			t.Errorf("dot-np-directory appeared in Passed but it had an error finding")
		}
	}
}
