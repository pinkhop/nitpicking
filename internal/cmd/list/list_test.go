package list_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/list"
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

func createTaskWithPriority(t *testing.T, svc driving.Service, title string, priority domain.Priority) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		Priority: priority,
		Author:   mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	return out.Issue.ID()
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

func claimAndClose(t *testing.T, svc driving.Service, issueID domain.ID) {
	t.Helper()
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	if err := svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	}); err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}
}

// --- Run Tests ---

func TestRun_NoIssues_ReportsNoneFound(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", buf.String())
	}
}

func TestRun_RoleFilterTask_ReturnsOnlyTasks(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "A task")
	createEpic(t, svc, "An epic")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			Role string `json:"role"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, item := range result.Items {
		if item.Role != "task" {
			t.Errorf("expected only tasks, got role=%q", item.Role)
		}
	}
}

func TestRun_RoleFilterEpic_ReturnsOnlyEpics(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "A task")
	createEpic(t, svc, "An epic")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleEpic}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			Role string `json:"role"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, item := range result.Items {
		if item.Role != "epic" {
			t.Errorf("expected only epics, got role=%q", item.Role)
		}
	}
}

func TestRun_MultipleRoleFilters_ReturnsBothRoles(t *testing.T) {
	t.Parallel()

	// Given — one task and one epic.
	svc := setupService(t)
	createTask(t, svc, "A task")
	createEpic(t, svc, "An epic")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask, domain.RoleEpic}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then — both the task and the epic are returned.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			Role string `json:"role"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	roles := map[string]bool{}
	for _, item := range result.Items {
		roles[item.Role] = true
	}
	if !roles["task"] {
		t.Error("expected a task in results")
	}
	if !roles["epic"] {
		t.Error("expected an epic in results")
	}
}

func TestRun_StateFilter_ReturnsMatchingState(t *testing.T) {
	t.Parallel()

	// Given — create a task and close it
	svc := setupService(t)
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)
	createTask(t, svc, "Open task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{States: []domain.State{domain.StateClosed}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 closed issue, got %d", len(result.Items))
	}
	if result.Items[0].State != "closed" {
		t.Errorf("expected closed state, got %q", result.Items[0].State)
	}
}

func TestRun_ExcludeClosed_HidesClosedByDefault(t *testing.T) {
	t.Parallel()

	// Given — create and close a task
	svc := setupService(t)
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)
	createTask(t, svc, "Open task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{ExcludeClosed: true},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			State string `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, item := range result.Items {
		if item.State == "closed" {
			t.Error("expected closed issues to be excluded")
		}
	}
}

func TestRun_IncludeClosed_ShowsClosedIssues(t *testing.T) {
	t.Parallel()

	// Given — create and close a task
	svc := setupService(t)
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)
	createTask(t, svc, "Open task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{ExcludeClosed: false},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) < 2 {
		t.Errorf("expected at least 2 issues (including closed), got %d", len(result.Items))
	}
}

func TestRun_ReadyFilter_ExcludesClaimedIssues(t *testing.T) {
	t.Parallel()

	// Given — create two tasks, claim one so it's no longer ready
	svc := setupService(t)
	readyID := createTask(t, svc, "Ready task")
	claimedID := createTask(t, svc, "Claimed task")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: claimedID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Ready: true},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// The claimed task should not appear in ready results
	for _, item := range result.Items {
		if item.ID == claimedID.String() {
			t.Error("expected claimed issue to be excluded from ready results")
		}
	}
	// The ready task should appear
	found := false
	for _, item := range result.Items {
		if item.ID == readyID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ready task %s in results", readyID)
	}
}

func TestRun_JSONOutput_ReturnsValidJSON(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "JSON output task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items   []json.RawMessage `json:"items"`
		HasMore bool              `json:"has_more"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) == 0 {
		t.Error("expected at least one item in JSON output")
	}
}

func TestRun_Limit_RestrictsResults(t *testing.T) {
	t.Parallel()

	// Given — create multiple issues
	svc := setupService(t)
	createTask(t, svc, "Task 1")
	createTask(t, svc, "Task 2")
	createTask(t, svc, "Task 3")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		Limit:   1,
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items   []json.RawMessage `json:"items"`
		HasMore bool              `json:"has_more"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected 1 item with limit=1, got %d", len(result.Items))
	}
	if !result.HasMore {
		t.Error("expected has_more=true when limit restricts results")
	}
}

func TestRun_TextOutput_IncludesIssueID(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Visible task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), issueID.String()) {
		t.Errorf("expected issue ID %s in text output, got: %s", issueID, buf.String())
	}
}

// TestRun_JSONOutput_ClaimedIssue_StateIsOpenWithSecondary verifies that a
// claimed issue appears in JSON list output with state "open" and
// secondary_state "claimed", not as a separate primary state.
func TestRun_JSONOutput_ClaimedIssue_StateIsOpenWithSecondary(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task for state check")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = list.Run(t.Context(), input)
	// Then — claimed issue must appear as state "open" with secondary_state "claimed".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			ID             string `json:"id"`
			State          string `json:"state"`
			SecondaryState string `json:"secondary_state"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	var found bool
	for _, item := range result.Items {
		if item.ID != issueID.String() {
			continue
		}
		found = true
		if item.State != "open" {
			t.Errorf("state: got %q, want %q", item.State, "open")
		}
		if item.SecondaryState != "claimed" {
			t.Errorf("secondary_state: got %q, want %q", item.SecondaryState, "claimed")
		}
	}
	if !found {
		t.Errorf("expected claimed task %s in list results", issueID)
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
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
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

// TestRun_TextOutput_ColumnsFlag_IncludesCreatedColumn verifies that requesting
// the CREATED column via --columns includes it in the header row.
func TestRun_TextOutput_ColumnsFlag_IncludesCreatedColumn(t *testing.T) {
	t.Parallel()

	// Given — a column set that explicitly includes CREATED.
	svc := setupService(t)
	createTask(t, svc, "Created column check")

	cols, err := cmdutil.ParseColumns("ID,PRIORITY,CREATED,TITLE")
	if err != nil {
		t.Fatalf("precondition: parse columns failed: %v", err)
	}

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		Columns: cols,
		WriteTo: &buf,
	}

	// When
	err = list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	if !strings.Contains(firstLine, "CREATED") {
		t.Errorf("expected CREATED column in header when columns flag includes CREATED, first line: %q", firstLine)
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
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		Columns: cols,
		WriteTo: &buf,
	}

	// When
	err = list.Run(t.Context(), input)
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

// TestRun_TextOutput_CustomColumns_EmptyColumns_UsesDefaults verifies that
// an empty Columns slice falls back to the default column set.
func TestRun_TextOutput_CustomColumns_EmptyColumns_UsesDefaults(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	createTask(t, svc, "Default columns task")

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	firstLine := strings.SplitN(output, "\n", 2)[0]
	// All default headers must appear.
	for _, hdr := range []string{"ID", "PRIORITY", "ROLE", "STATE", "TITLE"} {
		if !strings.Contains(firstLine, hdr) {
			t.Errorf("expected default header %q, first line: %q", hdr, firstLine)
		}
	}
}

// TestRun_TextOutput_ClaimedIssue_ShowsOpenClaimed verifies that a claimed
// issue appears in text output as "open (claimed)", not as a distinct primary
// state.
func TestRun_TextOutput_ClaimedIssue_ShowsOpenClaimed(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task for display check")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = list.Run(t.Context(), input)
	// Then — text output for the claimed issue must contain "open (claimed)".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "open (claimed)") {
		t.Errorf("expected 'open (claimed)' in text output for claimed issue, got:\n%s", output)
	}
}

// TestRun_OrderByID_SortsByIDAscending verifies that OrderByID sorts results
// lexicographically ascending by issue ID, independent of priority. This is
// the ordering the `np list` CLI selects by default when --order is omitted.
func TestRun_OrderByID_SortsByIDAscending(t *testing.T) {
	t.Parallel()

	// Given — create multiple tasks with different priorities so that
	// priority ordering would differ from ID ordering.
	svc := setupService(t)
	id1 := createTaskWithPriority(t, svc, "Low priority task", domain.P3)
	id2 := createTaskWithPriority(t, svc, "High priority task", domain.P0)
	id3 := createTaskWithPriority(t, svc, "Medium priority task", domain.P1)

	var buf bytes.Buffer
	input := list.RunInput{
		Service: svc,
		OrderBy: driving.OrderByID,
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := list.Run(t.Context(), input)
	// Then — items should be sorted by ID ascending, not by priority.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}

	// Collect IDs returned and verify they are in ascending order.
	ids := make([]string, len(result.Items))
	for i, item := range result.Items {
		ids[i] = item.ID
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("expected ID ascending order, but ids[%d]=%q > ids[%d]=%q", i-1, ids[i-1], i, ids[i])
		}
	}

	// Verify all three IDs are present (they may be in any position).
	idSet := map[string]bool{id1.String(): false, id2.String(): false, id3.String(): false}
	for _, item := range result.Items {
		idSet[item.ID] = true
	}
	for id, found := range idSet {
		if !found {
			t.Errorf("expected issue %s in results", id)
		}
	}
}
