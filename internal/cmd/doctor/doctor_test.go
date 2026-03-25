package doctor

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
)

func TestDeriveChecks_NoFindings_AllPass(t *testing.T) {
	t.Parallel()

	// Given — no findings at all.
	var findings []service.DoctorFinding

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — every check should pass.
	for _, c := range checks {
		if c.Status != "pass" {
			t.Errorf("check %q: expected status 'pass', got %q", c.Name, c.Status)
		}
	}
	if len(checks) == 0 {
		t.Error("expected at least one check")
	}
}

func TestDeriveChecks_StaleClaim_FailsClaimCheck(t *testing.T) {
	t.Parallel()

	// Given — a stale_claim finding.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Issue NP-abc is stale"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the stale_claims check should fail.
	c := checkByName(checks, "stale_claims")
	if c == nil {
		t.Fatal("expected 'stale_claims' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_BlockerCycle_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_cycle finding.
	findings := []service.DoctorFinding{
		{Category: "blocker_cycle", Severity: "error", Message: "Cycle detected in blocked_by chain involving NP-abc"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the blocker_health check should fail.
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_BlockerDeferred_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_deferred finding.
	findings := []service.DoctorFinding{
		{Category: "blocker_deferred", Severity: "error", Message: "Issue NP-xyz is deferred and blocking"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the blocker_health check should fail.
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_BlockerDeleted_FailsBlockerHealthCheck(t *testing.T) {
	t.Parallel()

	// Given — a blocker_deleted finding.
	findings := []service.DoctorFinding{
		{Category: "blocker_deleted", Severity: "error", Message: "Blocked by deleted issue"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the blocker_health check should fail.
	c := checkByName(checks, "blocker_health")
	if c == nil {
		t.Fatal("expected 'blocker_health' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_InstructionsFinding_FailsInstructionsCheck(t *testing.T) {
	t.Parallel()

	// Given — an instructions finding.
	findings := []service.DoctorFinding{
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the instructions check should fail.
	c := checkByName(checks, "instructions")
	if c == nil {
		t.Fatal("expected 'instructions' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_MultipleFindings_CorrectStatuses(t *testing.T) {
	t.Parallel()

	// Given — findings for stale claims and instructions, but not blocker health.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — stale_claims and instructions fail; blocker_health passes.
	staleClaims := checkByName(checks, "stale_claims")
	if staleClaims == nil || staleClaims.Status != "fail" {
		t.Error("expected stale_claims to fail")
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

// --- Severity threshold tests ---

func TestDeriveChecks_ErrorThreshold_SkipsWarningChecks(t *testing.T) {
	t.Parallel()

	// Given — a warning-level finding.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
	}

	// When — threshold is error, so warning checks should be skipped.
	checks := deriveChecks(findings, severityError)

	// Then — warning-level checks should be skipped; error-level should pass.
	staleClaims := checkByName(checks, "stale_claims")
	if staleClaims == nil || staleClaims.Status != "skipped" {
		t.Error("expected stale_claims to be skipped")
	}
	blockerHealth := checkByName(checks, "blocker_health")
	if blockerHealth == nil || blockerHealth.Status != "pass" {
		t.Error("expected blocker_health to pass (error-level, no findings)")
	}
}

func TestDeriveChecks_WarningThreshold_SkipsInfoButKeepsWarningAndError(t *testing.T) {
	t.Parallel()

	// Given — a warning-level finding.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
	}

	// When — threshold is warning.
	checks := deriveChecks(findings, severityWarning)

	// Then — warning-level checks should run; stale_claims should fail.
	c := checkByName(checks, "stale_claims")
	if c == nil {
		t.Fatal("expected 'stale_claims' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_ChecksOrderedBySeverity(t *testing.T) {
	t.Parallel()

	// Given — no findings.
	var findings []service.DoctorFinding

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — blocker_health (error) should come before stale_claims (warning).
	if len(checks) < 2 {
		t.Fatalf("expected at least 2 checks, got %d", len(checks))
	}
	if checks[0].Name != "blocker_health" {
		t.Errorf("first check should be 'blocker_health' (error), got %q", checks[0].Name)
	}
}

func TestCountSkipped_ReturnsCorrectCount(t *testing.T) {
	t.Parallel()

	// Given — a mix of passed, failed, and skipped checks.
	checks := []checkOutput{
		{Name: "a", Status: "pass"},
		{Name: "b", Status: "skipped"},
		{Name: "c", Status: "fail"},
		{Name: "d", Status: "skipped"},
	}

	// When
	count := countSkipped(checks)

	// Then
	if count != 2 {
		t.Errorf("skipped count: got %d, want 2", count)
	}
}

func TestFilterFindings_ExcludesSkippedCheckCategories(t *testing.T) {
	t.Parallel()

	// Given — findings for warning-level checks.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
		{Category: "instructions", Severity: "warning", Message: "No instructions"},
	}

	// When — threshold is error, so warning categories should be excluded.
	filtered := filterFindings(findings, severityError)

	// Then — only error-level check categories should pass; no warning findings.
	if len(filtered) != 0 {
		t.Errorf("filtered count: got %d, want 0", len(filtered))
	}
}

func TestFilterFindings_IncludesActiveCheckCategories(t *testing.T) {
	t.Parallel()

	// Given — findings for warning-level checks.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
	}

	// When — threshold is info (all checks active).
	filtered := filterFindings(findings, severityInfo)

	// Then — the finding should be included.
	if len(filtered) != 1 {
		t.Errorf("filtered count: got %d, want 1", len(filtered))
	}
}

func TestFilterFindings_IncludesErrorFindings(t *testing.T) {
	t.Parallel()

	// Given — findings for error-level checks.
	findings := []service.DoctorFinding{
		{Category: "blocker_cycle", Severity: "error", Message: "Cycle detected"},
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
	}

	// When — threshold is error.
	filtered := filterFindings(findings, severityError)

	// Then — only error-level findings pass.
	if len(filtered) != 1 {
		t.Errorf("filtered count: got %d, want 1", len(filtered))
	}
	if filtered[0].Category != "blocker_cycle" {
		t.Errorf("expected blocker_cycle, got %q", filtered[0].Category)
	}
}

func TestParseSeverity_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  checkSeverity
	}{
		{name: "error", input: "error", want: severityError},
		{name: "warning", input: "warning", want: severityWarning},
		{name: "info", input: "info", want: severityInfo},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got, err := parseSeverity(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("parseSeverity(%q): got %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseSeverity_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// When
	_, err := parseSeverity("critical")

	// Then
	if err == nil {
		t.Fatal("expected error for invalid severity value")
	}
}

func TestSeverityString_ReturnsLabel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		severity checkSeverity
		want     string
	}{
		{severity: severityError, want: "error"},
		{severity: severityWarning, want: "warning"},
		{severity: severityInfo, want: "info"},
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

// checkByName returns the checkOutput with the given name, or nil.
func checkByName(checks []checkOutput, name string) *checkOutput {
	for i := range checks {
		if checks[i].Name == name {
			return &checks[i]
		}
	}
	return nil
}
