package core

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- classifyFindings tests ---

func TestClassifyFindings_NoFindings_AllPass(t *testing.T) {
	t.Parallel()

	// Given — no findings at all.
	var findings []driving.DoctorFinding

	// When
	checks, filtered, healthy := classifyFindings(findings, driving.SeverityInfo)

	// Then — every check should pass and output is healthy.
	for _, c := range checks {
		if c.Status != "pass" {
			t.Errorf("check %q: expected status 'pass', got %q", c.Name, c.Status)
		}
	}
	if len(checks) == 0 {
		t.Error("expected at least one check")
	}
	if len(filtered) != 0 {
		t.Errorf("filtered findings: got %d, want 0", len(filtered))
	}
	if !healthy {
		t.Error("expected healthy to be true")
	}
}

func TestClassifyFindings_SchemaMigrationRequired_FailsSchemaMigrationCheck(t *testing.T) {
	t.Parallel()

	// Given — a schema_migration_required finding, produced when the database
	// is still at v1 schema and has not been upgraded.
	findings := []driving.DoctorFinding{
		{Category: "schema_migration_required", Severity: "error", Message: "Database is at v1 schema; run 'np admin upgrade'"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then — the schema_migration_required check should fail.
	c := checkByName(checks, "schema_migration_required")
	if c == nil {
		t.Fatal("expected 'schema_migration_required' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_BlockerCycle_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_cycle finding.
	findings := []driving.DoctorFinding{
		{Category: "blocker_cycle", Severity: "error", Message: "Cycle detected"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then — the blocker_health check should fail.
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_BlockerDeferred_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_deferred finding.
	findings := []driving.DoctorFinding{
		{Category: "blocker_deferred", Severity: "error", Message: "Deferred and blocking"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_BlockerDeleted_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_deleted finding.
	findings := []driving.DoctorFinding{
		{Category: "blocker_deleted", Severity: "error", Message: "Blocked by deleted issue"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_InstructionsFinding_FailsInstructionsCheck(t *testing.T) {
	t.Parallel()

	// Given — an instructions finding.
	findings := []driving.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then
	c := checkByName(checks, "instructions")
	if c == nil {
		t.Fatal("expected 'instructions' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_MultipleFindings_CorrectStatuses(t *testing.T) {
	t.Parallel()

	// Given — findings for schema migration and instructions, but not blocker health.
	findings := []driving.DoctorFinding{
		{Category: "schema_migration_required", Severity: "error", Message: "Database is at v1 schema"},
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then — schema_migration_required and instructions fail; blocker_health passes.
	schemaMigration := checkByName(checks, "schema_migration_required")
	if schemaMigration == nil || schemaMigration.Status != "fail" {
		t.Error("expected schema_migration_required to fail")
	}
	blockerHealth := checkByName(checks, "blocker_health")
	if blockerHealth == nil || blockerHealth.Status != "pass" {
		t.Error("expected blocker_health to pass")
	}
	instructions := checkByName(checks, "instructions")
	if instructions == nil || instructions.Status != "fail" {
		t.Error("expected instructions to fail")
	}
}

func TestClassifyFindings_GitignoreFinding_FailsGitignoreCheck(t *testing.T) {
	t.Parallel()

	// Given — a gitignore finding.
	findings := []driving.DoctorFinding{
		{Category: "gitignore", Severity: "warning", Message: ".np/ not in gitignore"},
	}

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then
	c := checkByName(checks, "gitignore")
	if c == nil {
		t.Fatal("expected 'gitignore' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

// --- Severity threshold tests ---

func TestClassifyFindings_ErrorThreshold_SkipsWarningChecks(t *testing.T) {
	t.Parallel()

	// Given — a warning-level finding.
	findings := []driving.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When — threshold is error, so warning checks should be skipped.
	checks, filtered, healthy := classifyFindings(findings, driving.SeverityError)

	// Then — warning-level checks should be skipped; error-level should pass.
	instructions := checkByName(checks, "instructions")
	if instructions == nil || instructions.Status != "skipped" {
		t.Error("expected instructions to be skipped")
	}
	blockerHealth := checkByName(checks, "blocker_health")
	if blockerHealth == nil || blockerHealth.Status != "pass" {
		t.Error("expected blocker_health to pass (error-level, no findings)")
	}
	if len(filtered) != 0 {
		t.Errorf("filtered findings should be empty when check is skipped, got %d", len(filtered))
	}
	if !healthy {
		t.Error("expected healthy when all active checks pass")
	}
}

func TestClassifyFindings_WarningThreshold_SkipsInfoButKeepsWarningAndError(t *testing.T) {
	t.Parallel()

	// Given — a warning-level finding.
	findings := []driving.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When — threshold is warning.
	checks, _, _ := classifyFindings(findings, driving.SeverityWarning)

	// Then — warning-level checks should run; instructions should fail.
	c := checkByName(checks, "instructions")
	if c == nil {
		t.Fatal("expected 'instructions' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestClassifyFindings_ChecksOrderedBySeverity(t *testing.T) {
	t.Parallel()

	// Given — no findings.
	var findings []driving.DoctorFinding

	// When
	checks, _, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then — error-level checks should come before warning-level checks.
	if len(checks) < 2 {
		t.Fatalf("expected at least 2 checks, got %d", len(checks))
	}
	if checks[0].Name != "storage_integrity" {
		t.Errorf("first check should be 'storage_integrity' (error), got %q", checks[0].Name)
	}
}

func TestClassifyFindings_FilterExcludesSkippedCheckCategories(t *testing.T) {
	t.Parallel()

	// Given — findings for warning-level checks.
	findings := []driving.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instructions"},
		{Category: "close_completed", Severity: "warning", Message: "Epic ready to close"},
	}

	// When — threshold is error, so warning categories should be excluded.
	_, filtered, _ := classifyFindings(findings, driving.SeverityError)

	// Then — only error-level check categories should pass; no warning findings.
	if len(filtered) != 0 {
		t.Errorf("filtered count: got %d, want 0", len(filtered))
	}
}

func TestClassifyFindings_FilterIncludesActiveCheckCategories(t *testing.T) {
	t.Parallel()

	// Given — findings for warning-level checks.
	findings := []driving.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instructions"},
	}

	// When — threshold is info (all checks active).
	_, filtered, _ := classifyFindings(findings, driving.SeverityInfo)

	// Then — the finding should be included.
	if len(filtered) != 1 {
		t.Errorf("filtered count: got %d, want 1", len(filtered))
	}
}

func TestClassifyFindings_FilterIncludesErrorFindings(t *testing.T) {
	t.Parallel()

	// Given — findings for error and warning-level checks.
	findings := []driving.DoctorFinding{
		{Category: "blocker_cycle", Severity: "error", Message: "Cycle detected"},
		{Category: "instructions", Severity: "warning", Message: "No instructions"},
	}

	// When — threshold is error.
	_, filtered, _ := classifyFindings(findings, driving.SeverityError)

	// Then — only error-level findings pass.
	if len(filtered) != 1 {
		t.Errorf("filtered count: got %d, want 1", len(filtered))
	}
	if filtered[0].Category != "blocker_cycle" {
		t.Errorf("expected blocker_cycle, got %q", filtered[0].Category)
	}
}

// --- CountSkippedChecks tests ---

func TestCountSkippedChecks_ReturnsCorrectCount(t *testing.T) {
	t.Parallel()

	// Given — a mix of passed, failed, and skipped checks.
	checks := []driving.DoctorCheckResult{
		{Name: "a", Status: "pass"},
		{Name: "b", Status: "skipped"},
		{Name: "c", Status: "fail"},
		{Name: "d", Status: "skipped"},
	}

	// When
	count := CountSkippedChecks(checks)

	// Then
	if count != 2 {
		t.Errorf("skipped count: got %d, want 2", count)
	}
}

// --- driving.ParseDoctorSeverity tests ---

func TestParseDoctorSeverity_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  driving.DoctorSeverity
	}{
		{name: "error", input: "error", want: driving.SeverityError},
		{name: "warning", input: "warning", want: driving.SeverityWarning},
		{name: "info", input: "info", want: driving.SeverityInfo},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, err := driving.ParseDoctorSeverity(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("driving.ParseDoctorSeverity(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseDoctorSeverity_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// When
	_, err := driving.ParseDoctorSeverity("critical")

	// Then
	if err == nil {
		t.Fatal("expected error for invalid severity value")
	}
}

func TestDoctorSeverityString_ReturnsLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		severity driving.DoctorSeverity
		want     string
	}{
		{severity: driving.SeverityError, want: "error"},
		{severity: driving.SeverityWarning, want: "warning"},
		{severity: driving.SeverityInfo, want: "info"},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			// When
			got := tc.severity.String()
			// Then
			if got != tc.want {
				t.Errorf("String(): got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDiagnosticChecks_OverdueDeferralsNotRegistered verifies that the
// overdue_deferrals check is not in the registry. The defer-until feature that
// would have emitted overdue_deferral findings was removed; the stale check
// definition causes admin doctor to report a permanently-passing check for a
// feature that no longer exists.
func TestDiagnosticChecks_OverdueDeferralsNotRegistered(t *testing.T) {
	t.Parallel()

	// Given — the diagnosticChecks registry.
	// When — we scan it for the overdue_deferrals check.
	var found bool
	for _, def := range diagnosticChecks {
		if def.Name == "overdue_deferrals" {
			found = true
			break
		}
	}

	// Then — overdue_deferrals must not be registered; the feature that produced
	// overdue_deferral findings no longer exists, so the check is permanently vacuous.
	if found {
		t.Error("diagnosticChecks must not contain 'overdue_deferrals': the defer-until feature was removed and no code emits 'overdue_deferral' findings")
	}
}

// checkByName returns the driving.DoctorCheckResult with the given name, or nil.
func checkByName(checks []driving.DoctorCheckResult, name string) *driving.DoctorCheckResult {
	for i := range checks {
		if checks[i].Name == name {
			return &checks[i]
		}
	}
	return nil
}
