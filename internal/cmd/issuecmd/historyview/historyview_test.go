package historyview_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd/historyview"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func createTask(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
}

func noColor() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

func ptr(s string) *string {
	return &s
}

// --- Run Tests ---

func TestRun_TextOutput_IncludesEntryFields(t *testing.T) {
	t.Parallel()

	// Given — a task with at least one history entry (from creation)
	svc := setupService(t)
	issueID := createTask(t, svc, "History task")

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// Should contain revision, event type, author.
	if !strings.Contains(output, "r0") {
		t.Errorf("expected revision 'r0' in text output, got: %s", output)
	}
	if !strings.Contains(output, "test-agent") {
		t.Errorf("expected author 'test-agent' in text output, got: %s", output)
	}
}

func TestRun_TextOutput_FieldChangesIndented(t *testing.T) {
	t.Parallel()

	// Given — a task that has been updated (creating a field change entry)
	svc := setupService(t)
	issueID := createTask(t, svc, "Original title")

	// Claim and update to generate a field change.
	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.UpdateIssue(t.Context(), driving.UpdateIssueInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Title:   ptr("Updated title"),
	})
	if err != nil {
		t.Fatalf("precondition: update failed: %v", err)
	}

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// Field changes should be indented with 4 spaces.
	if !strings.Contains(output, "    title:") {
		t.Errorf("expected indented field change for 'title' in text output, got: %s", output)
	}
}

func TestRun_JSONOutput_ContainsExpectedShape(t *testing.T) {
	t.Parallel()

	// Given — a task with history
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON history task")

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, field := range []string{"issue_id", "entries", "has_more"} {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field %q in JSON output", field)
		}
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
}

func TestRun_JSONOutput_EntryContainsExpectedFields(t *testing.T) {
	t.Parallel()

	// Given — a task with at least one history entry
	svc := setupService(t)
	issueID := createTask(t, svc, "Entry fields task")

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	entry := result.Entries[0]
	for _, field := range []string{"revision", "author", "event_type", "timestamp"} {
		if _, ok := entry[field]; !ok {
			t.Errorf("expected field %q in entry", field)
		}
	}
}

func TestRun_LimitRespectsPage(t *testing.T) {
	t.Parallel()

	// Given — a task with multiple history entries (create + claim + update)
	svc := setupService(t)
	issueID := createTask(t, svc, "Pagination task")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.UpdateIssue(t.Context(), driving.UpdateIssueInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Title:   ptr("Updated title"),
	})
	if err != nil {
		t.Fatalf("precondition: update failed: %v", err)
	}

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		Limit:       1,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Entries []map[string]any `json:"entries"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("entries: got %d, want 1 (limit=1)", len(result.Entries))
	}
	if !result.HasMore {
		t.Error("has_more: got false, want true (more entries exist)")
	}
}

func TestRun_TextOutput_ClaimDoesNotCreateHistoryEntry(t *testing.T) {
	t.Parallel()

	// Given — a task that has been claimed. Claiming creates a claim row but
	// does not record a history event; only the creation event should appear.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claim leak check")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		Limit:       -1,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// The claim ID must never appear in history output.
	if strings.Contains(output, claimOut.ClaimID) {
		t.Errorf("text output must not contain the claim ID %q, got:\n%s", claimOut.ClaimID, output)
	}
	// Claiming no longer generates a history event; only the creation entry exists.
	if !strings.Contains(output, "created") {
		t.Errorf("expected 'created' event type in text output, got:\n%s", output)
	}
}

func TestRun_JSONOutput_ClaimEvent_DoesNotLeakClaimID(t *testing.T) {
	t.Parallel()

	// Given — a task that has been claimed
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON claim leak check")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		Limit:       -1,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	if strings.Contains(output, claimOut.ClaimID) {
		t.Errorf("JSON output must not contain the claim ID %q, got:\n%s", claimOut.ClaimID, output)
	}
}

func TestRun_JSONOutput_FieldChangesIncluded(t *testing.T) {
	t.Parallel()

	// Given — a task that has been updated (field change recorded)
	svc := setupService(t)
	issueID := createTask(t, svc, "Changes task")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.UpdateIssue(t.Context(), driving.UpdateIssueInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Title:   ptr("Changed title"),
	})
	if err != nil {
		t.Fatalf("precondition: update failed: %v", err)
	}

	var buf bytes.Buffer
	input := historyview.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		Limit:       -1,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = historyview.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Entries []struct {
			Changes []struct {
				Field  string `json:"field"`
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"changes"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Find the update entry with a title field change (before→after).
	foundChange := false
	for _, e := range result.Entries {
		for _, c := range e.Changes {
			if c.Field == "title" && c.Before == "Changes task" && c.After == "Changed title" {
				foundChange = true
			}
		}
	}
	if !foundChange {
		t.Error("expected a title field change from 'Changes task' to 'Changed title' in history entries")
	}
}
