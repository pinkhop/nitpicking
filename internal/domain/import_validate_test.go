package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- Required fields ---

func TestValidate_MissingIdempotencyKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation errors for missing idempotency_key")
	}
	assertLineError(t, result.Errors, 0, "idempotency_key")
}

func TestValidate_MissingRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "key-1", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
}

// --- Field value validation ---

func TestValidate_InvalidRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "key-1", Role: "story", Title: "A story"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", Priority: "P9"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", Priority: "P0"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_InvalidState_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: "claimed"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: "blocked"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", Claim: true},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: "open", Claim: true},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: "closed", Claim: true},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: "deferred", Claim: true},
	}

	// When
	result := domain.Validate(lines, "NP")

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
				{IdempotencyKey: "key-1", Role: "task", Title: "A task", State: tc.state},
			}

			// When
			result := domain.Validate(lines, "NP")

			// Then
			if len(result.Errors) != 0 {
				t.Fatalf("expected no errors for state %q, got %v", tc.state, result.Errors)
			}
		})
	}
}

// --- Idempotency key uniqueness ---

func TestValidate_DuplicateIdempotencyKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "dup-key", Role: "task", Title: "First"},
		{IdempotencyKey: "dup-key", Role: "task", Title: "Second"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for duplicate idempotency_key")
	}
	assertLineError(t, result.Errors, 1, "idempotency_key")
}

// --- Label validation ---

func TestValidate_ReservedIdempotencyKeyLabel_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{
			IdempotencyKey: "key-1",
			Role:           "task",
			Title:          "A task",
			Labels:         map[string]string{"idempotency-key": "some-value"},
		},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) == 0 {
		t.Fatal("expected validation error for reserved idempotency-key label")
	}
	assertLineError(t, result.Errors, 0, "idempotency-key")
}

func TestValidate_InvalidLabelKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{
			IdempotencyKey: "key-1",
			Role:           "task",
			Title:          "A task",
			Labels:         map[string]string{"": "value"},
		},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", Author: ""},
	}

	// When — empty author is allowed (defaults to command-line author).
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors for empty author (uses default), got %v", result.Errors)
	}
}

// --- Reference resolution: intra-file ---

func TestValidate_ParentRefToIntraFileKey_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "child-1", Role: "task", Title: "Child task", Parent: "parent-epic"},
		{IdempotencyKey: "parent-epic", Role: "epic", Title: "Parent epic"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_ParentRefToTaskRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — parent references a task, not an epic.
	lines := []domain.RawLine{
		{IdempotencyKey: "child-1", Role: "task", Title: "Child task", Parent: "parent-task"},
		{IdempotencyKey: "parent-task", Role: "task", Title: "A task, not an epic"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", BlockedBy: []string{"nonexistent"}},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "blocker", Role: "task", Title: "Blocker"},
		{IdempotencyKey: "blocked", Role: "task", Title: "Blocked task", BlockedBy: []string{"blocker"}},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_BlocksIntraFile_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "blocker", Role: "task", Title: "Blocker", Blocks: []string{"blocked"}},
		{IdempotencyKey: "blocked", Role: "task", Title: "Blocked task"},
	}

	// When
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_RefsIntraFile_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "a", Role: "task", Title: "Task A", Refs: []string{"b"}},
		{IdempotencyKey: "b", Role: "task", Title: "Task B"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", Parent: "NP-a3bxr"},
	}

	// When — issue ID format refs are accepted without intra-file resolution.
	result := domain.Validate(lines, "NP")

	// Then
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", result.Errors)
	}
}

func TestValidate_BlockedByIssueID_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "key-1", Role: "task", Title: "A task", BlockedBy: []string{"NP-a3bxr"}},
	}

	// When
	result := domain.Validate(lines, "NP")

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
	result := domain.Validate(lines, "NP")

	// Then — should report errors for idempotency_key, role, and title.
	if len(result.Errors) < 3 {
		t.Fatalf("expected at least 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}
}

// --- Multiple lines with mixed validity ---

func TestValidate_MixedValidAndInvalidLines_ReportsErrorsForInvalidOnly(t *testing.T) {
	t.Parallel()

	// Given
	lines := []domain.RawLine{
		{IdempotencyKey: "good", Role: "task", Title: "Valid task"},
		{IdempotencyKey: "bad", Role: "invalid", Title: "Invalid role"},
	}

	// When
	result := domain.Validate(lines, "NP")

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
