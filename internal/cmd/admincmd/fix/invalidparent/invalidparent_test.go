package invalidparent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/fix/invalidparent"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- service stub ---

// repairServiceStub is a minimal stub satisfying invalidparent's local service
// interface, which exposes only RepairInvalidParentReferences.
type repairServiceStub struct {
	repairFn func(ctx context.Context, input driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error)
}

// RepairInvalidParentReferences delegates to repairFn.
func (s *repairServiceStub) RepairInvalidParentReferences(ctx context.Context, input driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
	return s.repairFn(ctx, input)
}

// twoAffected returns a stub that reports two dangling parent references.
func twoAffected() *repairServiceStub {
	return &repairServiceStub{
		repairFn: func(_ context.Context, _ driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
			return driving.RepairInvalidParentsOutput{
				Repaired: []driving.RepairedParentRecord{
					{IssueID: "NP-jkl78", RemovedParentID: "NP-mno90"},
					{IssueID: "NP-pqr01", RemovedParentID: "NP-stu22"},
				},
			}, nil
		},
	}
}

// oneAffected returns a stub that reports exactly one dangling parent reference.
func oneAffected() *repairServiceStub {
	return &repairServiceStub{
		repairFn: func(_ context.Context, _ driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
			return driving.RepairInvalidParentsOutput{
				Repaired: []driving.RepairedParentRecord{
					{IssueID: "NP-abc12", RemovedParentID: "NP-xyz99"},
				},
			}, nil
		},
	}
}

// zeroAffected returns a stub that reports no dangling parent references.
func zeroAffected() *repairServiceStub {
	return &repairServiceStub{
		repairFn: func(_ context.Context, _ driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
			return driving.RepairInvalidParentsOutput{}, nil
		},
	}
}

// --- test helpers ---

// runWith calls Run with the given service and returns stdout and any error.
func runWith(t *testing.T, svc *repairServiceStub, author string, dryRun, jsonOutput bool) (string, error) {
	t.Helper()
	ios, _, stdout, _ := iostreams.Test()
	input := invalidparent.RunInput{
		Author: author,
		DryRun: dryRun,
		JSON:   jsonOutput,
		Out:    ios.Out,
		Svc:    svc,
	}
	err := invalidparent.Run(t.Context(), input)
	return stdout.String(), err
}

// --- success path: text output ---

// TestRun_Success_TwoAffected_TextOutput verifies the banner, per-item lines,
// and summary for a successful repair of two issues.
func TestRun_Success_TwoAffected_TextOutput(t *testing.T) {
	t.Parallel()

	// Given
	svc := twoAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, false)
	// Then — banner, blank line, both items, blank line, summary.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "Removing dangling parent references...") {
		t.Errorf("expected banner in output, got: %q", output)
	}
	if !strings.Contains(output, "Cleaned NP-jkl78 (was → NP-mno90)") {
		t.Errorf("expected item line for NP-jkl78, got: %q", output)
	}
	if !strings.Contains(output, "Cleaned NP-pqr01 (was → NP-stu22)") {
		t.Errorf("expected item line for NP-pqr01, got: %q", output)
	}
	if !strings.Contains(output, "2 issues fixed.") {
		t.Errorf("expected summary '2 issues fixed.', got: %q", output)
	}
}

// TestRun_Success_TwoAffected_ItemOrder_Deterministic verifies that items
// appear in the order returned by the service (issue ID ascending per spec).
func TestRun_Success_TwoAffected_ItemOrder_Deterministic(t *testing.T) {
	t.Parallel()

	// Given
	svc := twoAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, false)
	// Then — NP-jkl78 appears before NP-pqr01.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	posFirst := strings.Index(output, "NP-jkl78")
	posSecond := strings.Index(output, "NP-pqr01")
	if posFirst < 0 || posSecond < 0 || posFirst >= posSecond {
		t.Errorf("expected NP-jkl78 before NP-pqr01 in output:\n%s", output)
	}
}

// --- success path: JSON output ---

// TestRun_Success_TwoAffected_JSONOutput verifies the JSON shape for a
// successful repair: "fixed" array with "issue"/"removed_parent_id" keys and
// a "count" field.
func TestRun_Success_TwoAffected_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given
	svc := twoAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, true)
	// Then — valid JSON with fixed array and count.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if _, ok := result["fixed"]; !ok {
		t.Fatal("JSON missing 'fixed' key")
	}
	fixed, ok := result["fixed"].([]any)
	if !ok {
		t.Fatalf("'fixed' is not an array: %T", result["fixed"])
	}
	if len(fixed) != 2 {
		t.Errorf("fixed count: got %d, want 2", len(fixed))
	}
	if result["count"] != float64(2) {
		t.Errorf("count: got %v, want 2", result["count"])
	}
	// Verify per-item keys.
	item0, ok := fixed[0].(map[string]any)
	if !ok {
		t.Fatalf("fixed[0] is not an object: %T", fixed[0])
	}
	if item0["issue"] == nil {
		t.Error("fixed[0] missing 'issue' key")
	}
	if item0["removed_parent_id"] == nil {
		t.Error("fixed[0] missing 'removed_parent_id' key")
	}
}

// TestRun_Success_OneAffected_SingularText verifies that repairing exactly one
// issue produces "1 issue fixed." (singular), not "1 issues fixed.".
func TestRun_Success_OneAffected_SingularText(t *testing.T) {
	t.Parallel()

	// Given
	svc := oneAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, false)
	// Then — summary uses singular "issue".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "1 issue fixed.") {
		t.Errorf("expected singular summary '1 issue fixed.', got: %q", output)
	}
	if strings.Contains(output, "1 issues fixed.") {
		t.Errorf("incorrect plural '1 issues fixed.' in output: %q", output)
	}
}

// TestRun_DryRun_OneAffected_SingularText verifies that a dry-run with exactly
// one affected issue produces "Would fix 1 issue." (singular).
func TestRun_DryRun_OneAffected_SingularText(t *testing.T) {
	t.Parallel()

	// Given
	svc := oneAffected()

	// When
	output, err := runWith(t, svc, "test-author", true, false)
	// Then — dry-run summary uses singular "issue".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "Would fix 1 issue.") {
		t.Errorf("expected singular dry-run summary 'Would fix 1 issue.', got: %q", output)
	}
	if strings.Contains(output, "Would fix 1 issues.") {
		t.Errorf("incorrect plural 'Would fix 1 issues.' in output: %q", output)
	}
}

// --- no-op path ---

// TestRun_NoOp_TextOutput verifies the no-op text message when no issues are
// affected.
func TestRun_NoOp_TextOutput(t *testing.T) {
	t.Parallel()

	// Given
	svc := zeroAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, false)
	// Then — no error, no-op message.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "No invalid parent references found. Nothing to fix.") {
		t.Errorf("expected no-op message, got: %q", output)
	}
}

// TestRun_NoOp_JSONOutput verifies the JSON shape for the no-op case:
// empty "fixed" array and count=0.
func TestRun_NoOp_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given
	svc := zeroAffected()

	// When
	output, err := runWith(t, svc, "test-author", false, true)
	// Then — valid JSON with empty fixed array and count=0.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	fixed, ok := result["fixed"].([]any)
	if !ok {
		t.Fatalf("'fixed' is not an array: %T", result["fixed"])
	}
	if len(fixed) != 0 {
		t.Errorf("fixed: got %d items, want 0", len(fixed))
	}
	if result["count"] != float64(0) {
		t.Errorf("count: got %v, want 0", result["count"])
	}
}

// --- dry-run path ---

// TestRun_DryRun_TwoAffected_TextOutput verifies the "Would clean" per-item
// lines and the "Would fix N issues" summary.
func TestRun_DryRun_TwoAffected_TextOutput(t *testing.T) {
	t.Parallel()

	// Given — dry-run stub still reports the two items (no writes occur).
	svc := twoAffected()

	// When
	output, err := runWith(t, svc, "test-author", true, false)
	// Then — Would clean lines, summary with re-run hint.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "Would clean NP-jkl78 (parent NP-mno90 does not exist)") {
		t.Errorf("expected dry-run item line for NP-jkl78, got: %q", output)
	}
	if !strings.Contains(output, "Would clean NP-pqr01 (parent NP-stu22 does not exist)") {
		t.Errorf("expected dry-run item line for NP-pqr01, got: %q", output)
	}
	if !strings.Contains(output, "Would fix 2 issues. Re-run without --dry-run to apply.") {
		t.Errorf("expected dry-run summary, got: %q", output)
	}
}

// TestRun_DryRun_TwoAffected_JSONOutput verifies the JSON shape for a dry-run:
// "would_fix" array with "issue"/"missing_parent_id" keys and a "count" field.
func TestRun_DryRun_TwoAffected_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given
	svc := twoAffected()

	// When
	output, err := runWith(t, svc, "test-author", true, true)
	// Then — valid JSON with would_fix array.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if _, ok := result["would_fix"]; !ok {
		t.Fatal("JSON missing 'would_fix' key")
	}
	wouldFix, ok := result["would_fix"].([]any)
	if !ok {
		t.Fatalf("'would_fix' is not an array: %T", result["would_fix"])
	}
	if len(wouldFix) != 2 {
		t.Errorf("would_fix count: got %d, want 2", len(wouldFix))
	}
	if result["count"] != float64(2) {
		t.Errorf("count: got %v, want 2", result["count"])
	}
	// Verify per-item keys.
	item0, ok := wouldFix[0].(map[string]any)
	if !ok {
		t.Fatalf("would_fix[0] is not an object: %T", wouldFix[0])
	}
	if item0["issue"] == nil {
		t.Error("would_fix[0] missing 'issue' key")
	}
	if item0["missing_parent_id"] == nil {
		t.Error("would_fix[0] missing 'missing_parent_id' key")
	}
}

// TestRun_DryRun_ForwardsDryRunFlag verifies that Run forwards DryRun=true to
// the service. The actual no-write guarantee is enforced by the core layer and
// is tested there; this test confirms the CLI layer correctly propagates the
// flag so the core's dry-run path is activated.
func TestRun_DryRun_ForwardsDryRunFlag(t *testing.T) {
	t.Parallel()

	// Given — a stub that records whether DryRun was set.
	var capturedDryRun bool
	svc := &repairServiceStub{
		repairFn: func(_ context.Context, input driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
			capturedDryRun = input.DryRun
			return driving.RepairInvalidParentsOutput{
				Repaired: []driving.RepairedParentRecord{
					{IssueID: "NP-abc12", RemovedParentID: "NP-xyz99"},
				},
			}, nil
		},
	}

	// When
	_, err := runWith(t, svc, "test-author", true, false)
	// Then — DryRun was passed to the service.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedDryRun {
		t.Error("expected DryRun=true to be passed to the service")
	}
}

// --- service error propagation ---

// TestRun_ServiceError_ReturnsError verifies that an error from the service is
// propagated to the caller.
func TestRun_ServiceError_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a stub that returns a sentinel error.
	sentinel := errors.New("database failure")
	svc := &repairServiceStub{
		repairFn: func(_ context.Context, _ driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
			return driving.RepairInvalidParentsOutput{}, sentinel
		},
	}

	// When
	_, err := runWith(t, svc, "test-author", false, false)

	// Then — error wraps the sentinel.
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected error chain to contain sentinel, got: %v", err)
	}
}

// --- NewCmd flag wiring ---

// TestNewCmd_FlagsParsed verifies that --author, --dry-run, and --json are
// wired into the RunInput that reaches the business logic.
func TestNewCmd_FlagsParsed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantAuthor string
		wantDryRun bool
		wantJSON   bool
	}{
		{
			name:       "all flags",
			args:       []string{"invalid-parent-reference", "--author", "alice", "--dry-run", "--json"},
			wantAuthor: "alice",
			wantDryRun: true,
			wantJSON:   true,
		},
		{
			name:       "no optional flags",
			args:       []string{"invalid-parent-reference", "--author", "bob"},
			wantAuthor: "bob",
			wantDryRun: false,
			wantJSON:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{IOStreams: ios}

			var captured invalidparent.RunInput
			runFn := func(_ context.Context, input invalidparent.RunInput) error {
				captured = input
				return nil
			}

			// When
			cmd := invalidparent.NewCmd(f, runFn)
			if err := cmd.Run(t.Context(), tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Then — RunInput fields match the flags.
			if captured.Author != tc.wantAuthor {
				t.Errorf("Author: got %q, want %q", captured.Author, tc.wantAuthor)
			}
			if captured.DryRun != tc.wantDryRun {
				t.Errorf("DryRun: got %v, want %v", captured.DryRun, tc.wantDryRun)
			}
			if captured.JSON != tc.wantJSON {
				t.Errorf("JSON: got %v, want %v", captured.JSON, tc.wantJSON)
			}
		})
	}
}

// TestNewCmd_FlagsParsed_AuthorFromEnv verifies that NP_AUTHOR is accepted in
// place of --author. This test is not parallel because it modifies an
// environment variable that is process-global.
func TestNewCmd_FlagsParsed_AuthorFromEnv(t *testing.T) {
	t.Setenv("NP_AUTHOR", "carol")

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured invalidparent.RunInput
	runFn := func(_ context.Context, input invalidparent.RunInput) error {
		captured = input
		return nil
	}

	// When
	cmd := invalidparent.NewCmd(f, runFn)
	if err := cmd.Run(t.Context(), []string{"invalid-parent-reference"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then
	if captured.Author != "carol" {
		t.Errorf("Author from env: got %q, want %q", captured.Author, "carol")
	}
}

// TestNewCmd_MissingAuthor_ExitsNonZero verifies that omitting --author (with
// NP_AUTHOR unset) causes the command to exit with a non-zero error.
//
// Not parallel: mutates the process-wide NP_AUTHOR environment variable.
func TestNewCmd_MissingAuthor_ExitsNonZero(t *testing.T) {
	// Save NP_AUTHOR and restore it after the test, whether or not it was set.
	prev, wasSet := os.LookupEnv("NP_AUTHOR")
	if err := os.Unsetenv("NP_AUTHOR"); err != nil {
		t.Fatalf("unsetting NP_AUTHOR: %v", err)
	}
	if wasSet {
		t.Cleanup(func() { _ = os.Setenv("NP_AUTHOR", prev) })
	}

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	cmd := invalidparent.NewCmd(f)
	err := cmd.Run(t.Context(), []string{"invalid-parent-reference"})

	if err == nil {
		t.Fatal("expected error when --author is missing, got nil")
	}
}

// TestNewCmd_UnknownFlag_ReturnsError verifies that an unrecognised flag
// produces an error (urfave/cli/v3 rejects unknown flags by default).
func TestNewCmd_UnknownFlag_ReturnsError(t *testing.T) {
	t.Parallel()

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	cmd := invalidparent.NewCmd(f)
	err := cmd.Run(t.Context(), []string{"invalid-parent-reference", "--author", "alice", "--unknown-flag"})

	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
}

// TestNewCmd_ExposesRequiredFlags verifies that --author, --dry-run, and --json
// are all registered on the command, ensuring they appear in --help output.
func TestNewCmd_ExposesRequiredFlags(t *testing.T) {
	t.Parallel()

	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	cmd := invalidparent.NewCmd(f)

	flagNames := make(map[string]bool)
	for _, fl := range cmd.Flags {
		for _, name := range fl.Names() {
			flagNames[name] = true
		}
	}

	for _, want := range []string{"author", "dry-run", "json"} {
		if !flagNames[want] {
			t.Errorf("command missing flag --%s", want)
		}
	}
}
