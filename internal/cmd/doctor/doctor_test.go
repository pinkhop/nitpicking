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

func TestDeriveChecks_NoReadyIssues_FailsReadinessCheck(t *testing.T) {
	t.Parallel()

	// Given — a no_ready_issues finding.
	findings := []service.DoctorFinding{
		{Category: "no_ready_issues", Severity: "warning", Message: "No issues are ready"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the readiness check should fail.
	c := checkByName(checks, "readiness")
	if c == nil {
		t.Fatal("expected 'readiness' check")
	}
	if c.Status != "fail" {
		t.Errorf("expected status 'fail', got %q", c.Status)
	}
}

func TestDeriveChecks_CloseEligibleBlocker_FailsReadinessCheck(t *testing.T) {
	t.Parallel()

	// Given — a close_eligible_blocker finding (subcategory of readiness).
	findings := []service.DoctorFinding{
		{Category: "close_eligible_blocker", Severity: "warning", Message: "Epic X is close-eligible"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — the readiness check should fail.
	c := checkByName(checks, "readiness")
	if c == nil {
		t.Fatal("expected 'readiness' check")
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

	// Given — findings for stale claims and instructions, but not readiness.
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
		{Category: "instructions", Severity: "warning", Message: "No instruction files"},
	}

	// When
	checks := deriveChecks(findings, severityInfo)

	// Then — stale_claims and instructions fail; readiness passes.
	staleClaims := checkByName(checks, "stale_claims")
	if staleClaims == nil || staleClaims.Status != "fail" {
		t.Error("expected stale_claims to fail")
	}
	readiness := checkByName(checks, "readiness")
	if readiness == nil || readiness.Status != "pass" {
		t.Error("expected readiness to pass")
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

	// Then — all current checks are warning-level, so all should be skipped.
	for _, c := range checks {
		if c.Status != "skipped" {
			t.Errorf("check %q: expected status 'skipped', got %q", c.Name, c.Status)
		}
	}
}

func TestDeriveChecks_WarningThreshold_IncludesWarningChecks(t *testing.T) {
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

	// Given — findings for both warning and info checks (all current checks
	// are warning-level, so with an error threshold all should be excluded).
	findings := []service.DoctorFinding{
		{Category: "stale_claim", Severity: "warning", Message: "Stale claim"},
		{Category: "instructions", Severity: "warning", Message: "No instructions"},
	}

	// When — threshold is error, so warning categories should be excluded.
	filtered := filterFindings(findings, severityError)

	// Then — no findings should pass the filter.
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
