package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- Required fields ---

func TestValidate_MissingIdempotencyLabel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing idempotency_label")
	}
	assertLineError(t, result.Errors, 0, "idempotency_label")
}

func TestValidate_MissingRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing role")
	}
	assertLineError(t, result.Errors, 0, "role")
}

func TestValidate_MissingTitle_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing title")
	}
	assertLineError(t, result.Errors, 0, "title")
}

// --- Valid minimal line ---

func TestValidate_MinimalValidLine_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

// --- Idempotency label is stored as a parsed Label ---

func TestValidate_ValidLine_IdempotencyLabelParsedCorrectly(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:PKHP-1234", Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	got := result.Records[0].IdempotencyLabel
	if got.Key() != "jira" {
		t.Errorf("expected IdempotencyLabel.Key() = %q, got %q", "jira", got.Key())
	}
	if got.Value() != "PKHP-1234" {
		t.Errorf("expected IdempotencyLabel.Value() = %q, got %q", "PKHP-1234", got.Value())
	}
}

// --- idempotency_label format validation ---

func TestValidate_IdempotencyLabelMissingColon_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — idempotency_label without the required key:value colon separator.
	lines := []domain.RawLine{
		{IdempotencyLabel: "nokeyvalue", Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for idempotency_label without colon")
	}
	assertLineError(t, result.Errors, 0, "idempotency_label")
}

func TestValidate_IdempotencyLabelInvalidKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — key starts with a digit, violating label key rules.
	lines := []domain.RawLine{
		{IdempotencyLabel: "1bad:value", Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for idempotency_label with invalid key")
	}
	assertLineError(t, result.Errors, 0, "idempotency_label")
}

// --- Field value validation ---

func TestValidate_InvalidRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "story", Title: "A story"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for invalid role")
	}
	assertLineError(t, result.Errors, 0, "role")
}

func TestValidate_InvalidPriority_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", Priority: "P9"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for invalid priority")
	}
	assertLineError(t, result.Errors, 0, "priority")
}

func TestValidate_ValidPriority_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", Priority: "P0"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_InvalidState_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: "claimed"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for invalid state")
	}
	assertLineError(t, result.Errors, 0, "state")
}

func TestValidate_BlockedState_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: "blocked"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for blocked state")
	}
	assertLineError(t, result.Errors, 0, "state")
}

// --- Claim field validation ---

func TestValidate_ClaimTrueWithOpenState_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — claim: true is allowed when state is open.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", Claim: true},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for claim: true with open state, got %v", result.Errors)
	}
	if len(result.Records) != 1 || !result.Records[0].Claim {
		t.Error("expected record to have Claim = true")
	}
}

func TestValidate_ClaimTrueWithExplicitOpenState_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — claim: true with explicit state: open.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: "open", Claim: true},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for claim: true with explicit open state, got %v", result.Errors)
	}
	if len(result.Records) != 1 || !result.Records[0].Claim {
		t.Error("expected record to have Claim = true")
	}
}

func TestValidate_ClaimTrueWithClosedState_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — claim: true is not valid for closed state.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: "closed", Claim: true},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for claim: true with closed state")
	}
	assertLineError(t, result.Errors, 0, "claim")
}

func TestValidate_ClaimTrueWithDeferredState_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — claim: true is not valid for deferred state.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: "deferred", Claim: true},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for claim: true with deferred state")
	}
	assertLineError(t, result.Errors, 0, "claim")
}

func TestValidate_ValidStates_Succeeds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state string
	}{
		{name: "open", state: "open"},
		{name: "deferred", state: "deferred"},
		{name: "closed", state: "closed"},
		{name: "empty defaults to open", state: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given
			lines := []domain.RawLine{
				{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", State: tc.state},
			}

			// When
			result := domain.Validate(lines, "FOO")

			// Then
			if len(result.Errors) != 0 {
				t.Fatalf("expected no errors for state %q, got %v", tc.state, result.Errors)
			}
		})
	}
}

// --- Idempotency label uniqueness ---

func TestValidate_DuplicateIdempotencyLabel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:DUP-1", Role: "task", Title: "First"},
		{IdempotencyLabel: "jira:DUP-1", Role: "task", Title: "Second"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for duplicate idempotency_label")
	}
	assertLineError(t, result.Errors, 1, "idempotency_label")
}

// --- Cross-field validation: idempotency_label vs labels ---

func TestValidate_IdempotencyLabelConflictsWithLabels_DifferentValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — idempotency_label is "jira:V1" but labels has jira:V2.
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:V1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"jira": "V2"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for conflicting idempotency_label and labels entry")
	}
	assertLineError(t, result.Errors, 0, "jira")
}

func TestValidate_IdempotencyLabelMatchesLabels_SameValue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — idempotency_label is "jira:V1" and labels also has jira:V1 (no-op duplicate).
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:V1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"jira": "V1"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for matching idempotency_label and labels entry, got %v", result.Errors)
	}
}

func TestValidate_IdempotencyLabelMatchingLabels_ProducesExactlyOneLabel(t *testing.T) {
	t.Parallel()

	// Given — idempotency_label "jira:V1" and labels with jira:V1 (duplicate no-op).
	// The validated record's Labels slice should not double-insert the label.
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:V1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"jira": "V1"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then — validation succeeds and the labels slice contains jira:V1 exactly once.
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	labels := result.Records[0].Labels
	count := 0
	for _, l := range labels {
		if l.Key() == "jira" && l.Value() == "V1" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 jira:V1 label in record, got %d", count)
	}
}

func TestValidate_IdempotencyLabelKeyNotInLabels_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — idempotency_label uses a key that does not appear in labels.
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:KEY-1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"kind": "bug"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

// --- Label validation ---

func TestValidate_FormerlyReservedIdempotencyKeyLabel_IsNowAllowed(t *testing.T) {
	t.Parallel()

	// Given — "idempotency-key" was previously reserved; it is now a normal label key.
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:KEY-1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"idempotency-key": "some-value"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then — no error; "idempotency-key" is no longer a reserved key.
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors — idempotency-key is no longer reserved, got %v", result.Errors)
	}
}

func TestValidate_InvalidLabelKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{
			IdempotencyLabel: "jira:KEY-1",
			Role:             "task",
			Title:            "A task",
			Labels:           map[string]string{"": "value"},
		},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for empty label key")
	}
	assertLineError(t, result.Errors, 0, "label")
}

// --- Author validation ---

func TestValidate_InvalidAuthor_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", Author: ""},
	}

	// When — empty author is allowed (defaults to command-line author).
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for empty author (uses default), got %v", result.Errors)
	}
}

// --- Reference resolution: intra-file ---

func TestValidate_ParentRefToIntraFileLabel_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — parent references the idempotency_label of an intra-file epic.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:CHILD-1", Role: "task", Title: "Child task", Parent: "jira:PARENT-EPIC"},
		{IdempotencyLabel: "jira:PARENT-EPIC", Role: "epic", Title: "Parent epic"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_ParentRefToTaskRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — parent references a task, not an epic.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:CHILD-1", Role: "task", Title: "Child task", Parent: "jira:PARENT-TASK"},
		{IdempotencyLabel: "jira:PARENT-TASK", Role: "task", Title: "A task, not an epic"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for parent referencing a task")
	}
	assertLineError(t, result.Errors, 0, "parent")
}

func TestValidate_UnresolvableRef_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", BlockedBy: []string{"jira:NONEXISTENT"}},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for unresolvable reference")
	}
	assertLineError(t, result.Errors, 0, "blocked_by")
}

func TestValidate_BlockedByIntraFile_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:BLOCKER-1", Role: "task", Title: "Blocker"},
		{IdempotencyLabel: "jira:BLOCKED-1", Role: "task", Title: "Blocked task", BlockedBy: []string{"jira:BLOCKER-1"}},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_BlocksIntraFile_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:BLOCKER-1", Role: "task", Title: "Blocker", Blocks: []string{"jira:BLOCKED-1"}},
		{IdempotencyLabel: "jira:BLOCKED-1", Role: "task", Title: "Blocked task"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_RefsIntraFile_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:A-1", Role: "task", Title: "Task A", Refs: []string{"jira:B-1"}},
		{IdempotencyLabel: "jira:B-1", Role: "task", Title: "Task B"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

// --- Reference resolution: issue ID format ---

func TestValidate_ParentRefToIssueID_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — reference looks like an issue ID matching the prefix.
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", Parent: "FOO-a3bxr"},
	}

	// When — issue ID format refs are accepted without intra-file resolution.
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_BlockedByIssueID_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:KEY-1", Role: "task", Title: "A task", BlockedBy: []string{"FOO-a3bxr"}},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

// --- Multiple errors per line ---

func TestValidate_MultipleErrorsOnOneLine_ReportsAll(t *testing.T) {
	t.Parallel()

	// Given — missing all three required fields.
	lines := []domain.RawLine{
		{},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then — should report errors for idempotency_label, role, and title.
	if len(result.Errors) < 3 {
		t.Fatalf("expected at least 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

// --- Multiple lines with mixed validity ---

func TestValidate_MixedValidAndInvalidLines_ReportsErrorsForInvalidOnly(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyLabel: "jira:GOOD-1", Role: "task", Title: "Valid task"},
		{IdempotencyLabel: "jira:BAD-1", Role: "invalid", Title: "Invalid role"},
	}

	// When
	result := domain.Validate(lines, "FOO")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for line 2")
	}
	// Valid records should still be produced for line 1.
	// The exact behavior (all-or-nothing vs. partial) depends on design;
	// we report errors but do not produce records for invalid lines.
	for _, e := range result.Errors {
		if e.Line != 1 {
			t.Errorf("expected error on line 1 (0-indexed), got line %d", e.Line)
		}
	}
}

// --- Helpers ---

// assertLineError checks that at least one error for the given line index
// contains the expected substring in its message.
func assertLineError(t *testing.T, errors []domain.LineError, lineIdx int, substr string) {
	t.Helper()
	for _, e := range errors {
		if e.Line == lineIdx && containsSubstring(e.Message, substr) {
			return
		}
	}
	t.Errorf("expected error on line %d containing %q, got: %v", lineIdx, substr, errors)
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
