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
	checks := deriveChecks(findings)

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
	checks := deriveChecks(findings)

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
	checks := deriveChecks(findings)

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
	checks := deriveChecks(findings)

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
	checks := deriveChecks(findings)

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
	checks := deriveChecks(findings)

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

// checkByName returns the checkOutput with the given name, or nil.
func checkByName(checks []checkOutput, name string) *checkOutput {
	for i := range checks {
		if checks[i].Name == name {
			return &checks[i]
		}
	}
	return nil
}
