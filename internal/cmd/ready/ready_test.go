package ready_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/ready"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
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
	svc := core.New(tx, nil)

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

// --- Run Tests ---

func TestRun_ReadyFilter_OnlyReturnsReadyIssues(t *testing.T) {
	t.Parallel()

	// Given — one ready task and one blocked task (not ready)
	svc := setupService(t)
	readyID := createTask(t, svc, "Ready task")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: readyID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Items))
	}
	if result.Items[0].ID != readyID.String() {
		t.Errorf("item ID: got %q, want %q", result.Items[0].ID, readyID.String())
	}
}

func TestRun_OrdersByPriority(t *testing.T) {
	t.Parallel()

	// Given — two ready tasks with different priorities
	svc := setupService(t)

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Low priority ready",
		Author:   mustAuthor(t, "test-agent"),
		Priority: domain.P3,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	highOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority ready",
		Author:   mustAuthor(t, "test-agent"),
		Priority: domain.P0,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) < 2 {
		t.Fatalf("items: got %d, want at least 2", len(result.Items))
	}
	if result.Items[0].ID != highOut.Issue.ID().String() {
		t.Errorf("first item should be high priority, got ID %q, want %q",
			result.Items[0].ID, highOut.Issue.ID().String())
	}
}

func TestRun_EmptyResult_TextShowsNoReadyMessage(t *testing.T) {
	t.Parallel()

	// Given — no ready issues (empty database after init)
	svc := setupService(t)

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "No ready issues") {
		t.Errorf("expected 'No ready issues' message, got: %s", output)
	}
}

func TestRun_JSONOutput_ContainsExpectedFields(t *testing.T) {
	t.Parallel()

	// Given — one ready task
	svc := setupService(t)
	_ = createTask(t, svc, "JSON ready task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, ok := result["items"]; !ok {
		t.Error("expected 'items' field in JSON output")
	}
	if _, ok := result["has_more"]; !ok {
		t.Error("expected 'has_more' field in JSON output")
	}
}

func TestRun_LimitRestricts_ResultCount(t *testing.T) {
	t.Parallel()

	// Given — three ready tasks, limit set to 2
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")
	_ = createTask(t, svc, "Task C")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
		Limit:       2,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) != 2 {
		t.Errorf("items: got %d, want 2", len(result.Items))
	}
	if !result.HasMore {
		t.Error("has_more: got false, want true")
	}
}

func TestRun_UnlimitedReturnsAll(t *testing.T) {
	t.Parallel()

	// Given — three ready tasks, limit set to -1 (unlimited)
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")
	_ = createTask(t, svc, "Task C")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
		Limit:       -1,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) != 3 {
		t.Errorf("items: got %d, want 3", len(result.Items))
	}
	if result.HasMore {
		t.Error("has_more: got true, want false")
	}
}

func TestRun_ZeroLimit_UsesDefault(t *testing.T) {
	t.Parallel()

	// Given — one ready task, limit left at zero (default behavior)
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
		Limit:       0,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) != 1 {
		t.Errorf("items: got %d, want 1", len(result.Items))
	}
}

// TestRun_TextOutput_Header_PrintsDefaultColumnHeaders verifies that the text
// output includes all default column headers in the correct order.
func TestRun_TextOutput_Header_PrintsDefaultColumnHeaders(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Header check task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	expectedHeaders := []string{"ID", "PRIORITY", "ROLE", "STATE", "TITLE"}
	for _, hdr := range expectedHeaders {
		if !strings.Contains(firstLine, hdr) {
			t.Errorf("expected header row to contain %q, first line: %q", hdr, firstLine)
		}
	}
}

// TestRun_TextOutput_CustomColumns_ShowsOnlySelectedColumns verifies that
// passing a custom column set renders only those columns in the given order.
func TestRun_TextOutput_CustomColumns_ShowsOnlySelectedColumns(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Custom columns task")

	cols, err := cmdutil.ParseColumns("ID,TITLE,STATE")
	if err != nil {
		t.Fatalf("precondition: parse columns failed: %v", err)
	}

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        false,
		Columns:     cols,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	if !strings.Contains(firstLine, "ID") {
		t.Errorf("expected ID in header, first line: %q", firstLine)
	}
	if !strings.Contains(firstLine, "TITLE") {
		t.Errorf("expected TITLE in header, first line: %q", firstLine)
	}
	if !strings.Contains(firstLine, "STATE") {
		t.Errorf("expected STATE in header, first line: %q", firstLine)
	}
	// PRIORITY should not appear since it was not selected.
	if strings.Contains(firstLine, "PRIORITY") {
		t.Errorf("PRIORITY should not appear in custom column set, first line: %q", firstLine)
	}
}

// TestRun_TextOutput_EmptyColumns_UsesDefaults verifies that an empty Columns
// slice falls back to the default column set.
func TestRun_TextOutput_EmptyColumns_UsesDefaults(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Default columns task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	for _, hdr := range []string{"ID", "PRIORITY", "ROLE", "STATE", "TITLE"} {
		if !strings.Contains(firstLine, hdr) {
			t.Errorf("expected default header %q, first line: %q", hdr, firstLine)
		}
	}
}

func TestRun_TextOutput_IncludesIssueDetails(t *testing.T) {
	t.Parallel()

	// Given — one ready task
	svc := setupService(t)
	taskID := createTask(t, svc, "Ready text task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, taskID.String()) {
		t.Errorf("expected issue ID %s in text output, got: %s", taskID, output)
	}
	if !strings.Contains(output, "Ready text task") {
		t.Errorf("expected title in text output, got: %s", output)
	}
	if !strings.Contains(output, "1 ready") {
		t.Errorf("expected '1 ready' summary in text output, got: %s", output)
	}
}
