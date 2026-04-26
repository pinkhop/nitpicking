package search_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/issuecmd/search"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
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

func createEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
}

// --- Run Tests ---

func TestRun_EmptyQuery_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestRun_MatchingQuery_ReturnsResults(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Fix authentication timeout")
	createTask(t, svc, "Update database schema")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "authentication",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Issues []struct {
			Title string `json:"title"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected at least one search result")
	}
	if !strings.Contains(result.Issues[0].Title, "authentication") {
		t.Errorf("expected matching title, got %q", result.Issues[0].Title)
	}
}

func TestRun_NoMatches_ReportsNoneFound(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Fix login bug")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "zzzznonexistentzzzz",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", buf.String())
	}
}

func TestRun_RoleFilter_ReturnsOnlyMatchingRole(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Searchable task item")
	createEpic(t, svc, "Searchable epic item")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "Searchable",
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Issues []struct {
			Role string `json:"role"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, item := range result.Issues {
		if item.Role != "task" {
			t.Errorf("expected only tasks, got role=%q", item.Role)
		}
	}
}

func TestRun_JSONOutput_HasExpectedStructure(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Structured output test")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "Structured",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Issues []struct {
			ID       string `json:"id"`
			Role     string `json:"role"`
			State    string `json:"state"`
			Priority string `json:"priority"`
			Title    string `json:"title"`
		} `json:"issues"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Issues))
	}
	item := result.Issues[0]
	if item.ID == "" {
		t.Error("expected non-empty ID")
	}
	if item.Role != "task" {
		t.Errorf("role: got %q, want %q", item.Role, "task")
	}
	if item.State != "open" {
		t.Errorf("state: got %q, want %q", item.State, "open")
	}
}

// TestRun_TextOutput_Header_PrintsDefaultColumnHeaders verifies that the
// search text output includes all default column headers.
func TestRun_TextOutput_Header_PrintsDefaultColumnHeaders(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Searchable header check")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "Searchable",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
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
// passing a custom column set renders only those columns.
func TestRun_TextOutput_CustomColumns_ShowsOnlySelectedColumns(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Searchable custom columns")

	cols, err := cmdutil.ParseColumns("ID,TITLE")
	if err != nil {
		t.Fatalf("precondition: parse columns failed: %v", err)
	}

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "Searchable",
		JSON:    false,
		Columns: cols,
		WriteTo: &buf,
	}

	// When
	err = search.Run(t.Context(), input)
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
	if strings.Contains(firstLine, "PRIORITY") {
		t.Errorf("PRIORITY should not appear in custom column set, first line: %q", firstLine)
	}
}

func TestRun_TextOutput_IncludesMatchingTitle(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Plaintext search result")

	var buf bytes.Buffer
	input := search.RunInput{
		Service: svc,
		Query:   "Plaintext",
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := search.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Plaintext search result") {
		t.Errorf("expected title in text output, got: %s", buf.String())
	}
}
