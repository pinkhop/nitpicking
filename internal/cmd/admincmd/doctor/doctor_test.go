package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- checkNpGitIgnored tests ---

func TestCheckNpGitIgnored_Ignored_ReturnsNoFindings(t *testing.T) {
	t.Parallel()

	// Given — a stub that reports the path is ignored.
	stub := func(dir, path string) (bool, error) { return true, nil }

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — no findings.
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckNpGitIgnored_NotIgnored_ReturnsWarning(t *testing.T) {
	t.Parallel()

	// Given — a stub that reports the path is NOT ignored.
	stub := func(dir, path string) (bool, error) { return false, nil }

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — one warning finding about gitignore.
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Category != "gitignore" {
		t.Errorf("category: got %q, want %q", findings[0].Category, "gitignore")
	}
	if findings[0].Severity != "warning" {
		t.Errorf("severity: got %q, want %q", findings[0].Severity, "warning")
	}
}

func TestCheckNpGitIgnored_NotGitRepo_ReturnsNoFindings(t *testing.T) {
	t.Parallel()

	// Given — a stub that returns an error (not a git repo).
	stub := func(dir, path string) (bool, error) {
		return false, errNotGitRepo
	}

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — no findings (check is skipped when not in a git repo).
	if len(findings) != 0 {
		t.Errorf("expected 0 findings when not in git repo, got %d", len(findings))
	}
}

// --- run tests: prefix ---

// stubDoctorOutput is a minimal DoctorOutput used across prefix-related tests.
var stubDoctorOutput = driving.DoctorOutput{
	Healthy:  true,
	Findings: []driving.DoctorFinding{},
	Checks:   []driving.DoctorCheckResult{},
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

func TestRun_WithPrefix_TextOutput_IncludesPrefix(t *testing.T) {
	t.Parallel()

	// Given — a prefix function that returns a known prefix.
	getPrefix := func(_ context.Context) (string, error) {
		return "PROJ", nil
	}
	ios, stdout := newTestStreams()

	input := runInput{
		GetPrefixFunc: getPrefix,
		DoctorFunc:    stubDoctorFunc,
		MinSeverity:   driving.SeverityInfo,
		IOStreams:     ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — no error and prefix appears in the text output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "PROJ") {
		t.Errorf("expected prefix %q in output, got: %q", "PROJ", output)
	}
}

func TestRun_PrefixUnavailable_TextOutput_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a prefix function that returns an error (e.g., uninitialized DB).
	getPrefix := func(_ context.Context) (string, error) {
		return "", errors.New("database not initialized")
	}
	ios, _ := newTestStreams()

	input := runInput{
		GetPrefixFunc: getPrefix,
		DoctorFunc:    stubDoctorFunc,
		MinSeverity:   driving.SeverityInfo,
		IOStreams:     ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — command still succeeds even though the prefix is unavailable.
	if err != nil {
		t.Fatalf("unexpected error when prefix unavailable: %v", err)
	}
}

func TestRun_WithPrefix_JSONOutput_IncludesPrefixField(t *testing.T) {
	t.Parallel()

	// Given — a prefix function that returns a known prefix.
	getPrefix := func(_ context.Context) (string, error) {
		return "PROJ", nil
	}
	ios, stdout := newTestStreams()

	input := runInput{
		GetPrefixFunc: getPrefix,
		DoctorFunc:    stubDoctorFunc,
		MinSeverity:   driving.SeverityInfo,
		JSON:          true,
		IOStreams:     ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — the JSON output contains the prefix field.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, stdout.String())
	}
	if result["prefix"] != "PROJ" {
		t.Errorf("prefix: got %v, want %q", result["prefix"], "PROJ")
	}
}

func TestRun_PrefixUnavailable_JSONOutput_OmitsPrefixField(t *testing.T) {
	t.Parallel()

	// Given — a prefix function that returns an error.
	getPrefix := func(_ context.Context) (string, error) {
		return "", errors.New("database not initialized")
	}
	ios, stdout := newTestStreams()

	input := runInput{
		GetPrefixFunc: getPrefix,
		DoctorFunc:    stubDoctorFunc,
		MinSeverity:   driving.SeverityInfo,
		JSON:          true,
		IOStreams:     ios,
	}

	// When
	err := run(t.Context(), input)
	// Then — command succeeds and the JSON does not include a prefix key.
	if err != nil {
		t.Fatalf("unexpected error when prefix unavailable: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, stdout.String())
	}
	if _, ok := result["prefix"]; ok {
		t.Errorf("expected prefix to be absent from JSON when unavailable, but found key: %v", result)
	}
}

// --- renderAction tests ---

func TestRenderAction_AllBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hint *driving.ActionHint
		want string
	}{
		{
			name: "nil input returns empty string",
			hint: nil,
			want: "",
		},
		{
			name: "ActionKindRunGC returns gc command",
			hint: &driving.ActionHint{Kind: driving.ActionKindRunGC},
			want: "Run 'np admin gc --confirm' to remove deleted issues.",
		},
		{
			name: "ActionKindUndefer returns undefer command with issue ID",
			hint: &driving.ActionHint{Kind: driving.ActionKindUndefer, IssueID: "FOO-abc12"},
			want: "Run 'np issue undefer FOO-abc12 --author <name>' to restore it.",
		},
		{
			// The stored row is (source, blocked_by, target); "blocks" inverts
			// source/target and silently no-ops because it targets a non-existent
			// row. The command must use "blocked_by", not "blocks".
			name: "ActionKindUnblockRelationship returns rel remove blocked_by command",
			hint: &driving.ActionHint{
				Kind:     driving.ActionKindUnblockRelationship,
				SourceID: "FOO-abc12",
				TargetID: "FOO-xyz99",
			},
			want: "Run 'np rel remove FOO-abc12 blocked_by FOO-xyz99 --author <name>' to remove the stale relationship.",
		},
		{
			name: "ActionKindCloseCompleted returns close-completed command",
			hint: &driving.ActionHint{Kind: driving.ActionKindCloseCompleted},
			want: "Run 'np epic close-completed --author <name>' to batch-close fully resolved epics.",
		},
		{
			name: "ActionKindInvestigateCorruption returns backup instruction",
			hint: &driving.ActionHint{Kind: driving.ActionKindInvestigateCorruption},
			want: "Back up .np/ immediately and investigate corruption.",
		},
		{
			name: "ActionKindExecSQL returns delete stray rows with SQL",
			hint: &driving.ActionHint{
				Kind: driving.ActionKindExecSQL,
				SQL:  "DELETE FROM issues WHERE id = 99",
			},
			want: "Delete the stray rows: DELETE FROM issues WHERE id = 99",
		},
		{
			name: "ActionKindAddToGitignore returns gitignore instruction",
			hint: &driving.ActionHint{Kind: driving.ActionKindAddToGitignore},
			want: "Add .np/ to .gitignore",
		},
		{
			// Exercises the default branch; uses a sentinel string that cannot
			// collide with any real ActionKind constant.
			name: "unknown ActionKind returns empty string",
			hint: &driving.ActionHint{Kind: "__unknown_for_testing_default_branch__"},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := renderAction(tc.hint)

			// Then
			if got != tc.want {
				t.Errorf("renderAction: got %q, want %q", got, tc.want)
			}
		})
	}
}
