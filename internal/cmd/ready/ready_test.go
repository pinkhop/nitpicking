package ready_test

import (
	"bytes"
	"context"
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
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != readyID.String() {
		t.Errorf("item ID: got %q, want %q", result.Issues[0].ID, readyID.String())
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
	if len(result.Issues) < 2 {
		t.Fatalf("items: got %d, want at least 2", len(result.Issues))
	}
	if result.Issues[0].ID != highOut.Issue.ID().String() {
		t.Errorf("first item should be high priority, got ID %q, want %q",
			result.Issues[0].ID, highOut.Issue.ID().String())
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
	if _, ok := result["issues"]; !ok {
		t.Error("expected 'issues' field in JSON output")
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
	if len(result.Issues) != 2 {
		t.Errorf("items: got %d, want 2", len(result.Issues))
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
	if len(result.Issues) != 3 {
		t.Errorf("items: got %d, want 3", len(result.Issues))
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
	if len(result.Issues) != 1 {
		t.Errorf("items: got %d, want 1", len(result.Issues))
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

// --- Filter flag tests ---

// createEpicWithParent creates a ready epic with no blockers and returns its ID.
func createEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	return out.Issue.ID()
}

// createTaskWithLabel creates a ready task with the given label.
func createTaskWithLabel(t *testing.T, svc driving.Service, title, labelKey, labelValue string) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
		Labels: []driving.LabelInput{{Key: labelKey, Value: labelValue}},
	})
	if err != nil {
		t.Fatalf("precondition: create labeled task failed: %v", err)
	}
	return out.Issue.ID()
}

// createTaskUnderParent creates a ready task whose parent is the given epic.
func createTaskUnderParent(t *testing.T, svc driving.Service, title string, parentID domain.ID) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		Author:   mustAuthor(t, "test-agent"),
		ParentID: parentID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child task failed: %v", err)
	}
	return out.Issue.ID()
}

// TestRun_RoleFilterTask_ReturnsOnlyReadyTasks verifies that --role task
// restricts results to ready tasks, hiding ready epics.
func TestRun_RoleFilterTask_ReturnsOnlyReadyTasks(t *testing.T) {
	t.Parallel()

	// Given — one ready task and one ready epic
	svc := setupService(t)
	taskID := createTask(t, svc, "Ready task")
	_ = createEpic(t, svc, "Ready epic")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleTask}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != taskID.String() {
		t.Errorf("item ID: got %q, want %q (task)", result.Issues[0].ID, taskID.String())
	}
}

// TestRun_RoleFilterEpic_ReturnsOnlyReadyEpics verifies that --role epic
// restricts results to ready epics, hiding ready tasks.
func TestRun_RoleFilterEpic_ReturnsOnlyReadyEpics(t *testing.T) {
	t.Parallel()

	// Given — one ready task and one ready epic
	svc := setupService(t)
	_ = createTask(t, svc, "Ready task")
	epicID := createEpic(t, svc, "Ready epic")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{Roles: []domain.Role{domain.RoleEpic}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != epicID.String() {
		t.Errorf("item ID: got %q, want %q (epic)", result.Issues[0].ID, epicID.String())
	}
}

// TestRun_StateFilter_Open_ReturnsOnlyOpenIssues verifies that --state open
// behaves identically to np list --ready --state open.
func TestRun_StateFilter_Open_ReturnsOnlyOpenIssues(t *testing.T) {
	t.Parallel()

	// Given — one open ready task (closed issues are never ready, so this
	// just confirms the state filter threads through correctly)
	svc := setupService(t)
	taskID := createTask(t, svc, "Open ready task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{States: []domain.State{domain.StateOpen}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != taskID.String() {
		t.Errorf("item ID: got %q, want %q", result.Issues[0].ID, taskID.String())
	}
}

// TestRun_LabelFilter_ReturnsOnlyMatchingLabel verifies that --label kind:bug
// restricts results to ready issues bearing that label.
func TestRun_LabelFilter_ReturnsOnlyMatchingLabel(t *testing.T) {
	t.Parallel()

	// Given — one labeled task and one unlabeled task
	svc := setupService(t)
	bugID := createTaskWithLabel(t, svc, "Bug task", "kind", "bug")
	_ = createTask(t, svc, "Unlabeled task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter: driving.IssueFilterInput{
			LabelFilters: []driving.LabelFilterInput{{Key: "kind", Value: "bug"}},
		},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != bugID.String() {
		t.Errorf("item ID: got %q, want %q (bug task)", result.Issues[0].ID, bugID.String())
	}
}

// TestRun_ParentFilter_ReturnsOnlyChildrenOfParent verifies that --parent
// restricts results to ready children of the specified epic.
func TestRun_ParentFilter_ReturnsOnlyChildrenOfParent(t *testing.T) {
	t.Parallel()

	// Given — one epic with one child task, and one unrelated task
	svc := setupService(t)
	epicID := createEpic(t, svc, "Parent epic")
	childID := createTaskUnderParent(t, svc, "Child task", epicID)
	_ = createTask(t, svc, "Unrelated task")

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter:  driving.IssueFilterInput{ParentIDs: []string{epicID.String()}},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := ready.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("items: got %d, want 1", len(result.Issues))
	}
	if result.Issues[0].ID != childID.String() {
		t.Errorf("item ID: got %q, want %q (child task)", result.Issues[0].ID, childID.String())
	}
}

// TestRun_CombinedFilters_ANDSemantics verifies that combining --role, --label,
// and --parent applies AND semantics: only issues matching all conditions appear.
func TestRun_CombinedFilters_ANDSemantics(t *testing.T) {
	t.Parallel()

	// Given — an epic with two children: one labeled bug task and one
	// unlabeled task; plus an unrelated bug task outside the epic
	svc := setupService(t)
	epicID := createEpic(t, svc, "Parent epic")

	// Child task with both the right parent and the bug label — should match all three filters.
	ctx := t.Context()
	matchOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Bug child task",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
		Labels:   []driving.LabelInput{{Key: "kind", Value: "bug"}},
	})
	if err != nil {
		t.Fatalf("precondition: create labeled child task failed: %v", err)
	}
	matchID := matchOut.Issue.ID()

	_ = createTaskUnderParent(t, svc, "Unlabeled child task", epicID)    // same parent, no label
	_ = createTaskWithLabel(t, svc, "Unrelated bug task", "kind", "bug") // bug label, no parent

	var buf bytes.Buffer
	input := ready.RunInput{
		Service: svc,
		Filter: driving.IssueFilterInput{
			Roles:        []domain.Role{domain.RoleTask},
			ParentIDs:    []string{epicID.String()},
			LabelFilters: []driving.LabelFilterInput{{Key: "kind", Value: "bug"}},
		},
		JSON:    true,
		WriteTo: &buf,
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
	if len(result.Issues) != 1 {
		ids := make([]string, len(result.Issues))
		for i, it := range result.Issues {
			ids[i] = it.ID
		}
		t.Fatalf("items: got %d (%v), want 1", len(result.Issues), ids)
	}
	if result.Issues[0].ID != matchID.String() {
		t.Errorf("item ID: got %q, want %q (labeled child task)", result.Issues[0].ID, matchID.String())
	}
}

// --- Flag-parsing tests via NewCmd runFn injection ---

// TestNewCmd_FlagParsing_Role_SetsRoleFilter verifies that --role task is
// parsed and placed in the RunInput.Filter.Roles field.
func TestNewCmd_FlagParsing_Role_SetsRoleFilter(t *testing.T) {
	t.Parallel()

	// Given — a factory with test streams and a runFn that captures input.
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured ready.RunInput
	runFn := func(_ context.Context, input ready.RunInput) error {
		captured = input
		return nil
	}

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--role", "task"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Filter.Roles) != 1 || captured.Filter.Roles[0] != domain.RoleTask {
		t.Errorf("Filter.Roles: got %v, want [task]", captured.Filter.Roles)
	}
}

// TestNewCmd_FlagParsing_Role_InvalidValue_ReturnsError verifies that an
// unrecognised role value produces an error consistent with np list's behavior.
func TestNewCmd_FlagParsing_Role_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, _ ready.RunInput) error { return nil }

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--role", "invalid-role"})
	// Then
	if err == nil {
		t.Fatal("expected an error for invalid role, got nil")
	}
}

// TestNewCmd_FlagParsing_State_SetsStateFilter verifies that --state open is
// parsed and placed in the RunInput.Filter.States field.
func TestNewCmd_FlagParsing_State_SetsStateFilter(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured ready.RunInput
	runFn := func(_ context.Context, input ready.RunInput) error {
		captured = input
		return nil
	}

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--state", "open"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Filter.States) != 1 || captured.Filter.States[0] != domain.StateOpen {
		t.Errorf("Filter.States: got %v, want [open]", captured.Filter.States)
	}
}

// TestNewCmd_FlagParsing_State_InvalidValue_ReturnsError verifies that an
// unrecognised state value produces an error, delegated to domain.ParseState.
func TestNewCmd_FlagParsing_State_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, _ ready.RunInput) error { return nil }

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--state", "notastate"})
	// Then
	if err == nil {
		t.Fatal("expected an error for invalid state, got nil")
	}
}

// TestNewCmd_FlagParsing_Label_SetsLabelFilter verifies that --label kind:bug
// is parsed via cmdutil.ParseLabelFilters and placed in Filter.LabelFilters.
func TestNewCmd_FlagParsing_Label_SetsLabelFilter(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured ready.RunInput
	runFn := func(_ context.Context, input ready.RunInput) error {
		captured = input
		return nil
	}

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--label", "kind:bug"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Filter.LabelFilters) != 1 {
		t.Fatalf("LabelFilters: got %d, want 1", len(captured.Filter.LabelFilters))
	}
	lf := captured.Filter.LabelFilters[0]
	if lf.Key != "kind" || lf.Value != "bug" || lf.Negate {
		t.Errorf("LabelFilter: got {Key:%q Value:%q Negate:%v}, want {Key:\"kind\" Value:\"bug\" Negate:false}",
			lf.Key, lf.Value, lf.Negate)
	}
}

// TestNewCmd_FlagParsing_Label_InvalidValue_ReturnsError verifies that a
// label value without the key:value separator is rejected.
func TestNewCmd_FlagParsing_Label_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, _ ready.RunInput) error { return nil }

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"ready", "--label", "no-colon"})
	// Then
	if err == nil {
		t.Fatal("expected an error for invalid label format, got nil")
	}
}

// TestNewCmd_FlagParsing_CombinedFilters_AllFieldsSet verifies that passing
// --role, --state, and --label together populates all three filter fields.
func TestNewCmd_FlagParsing_CombinedFilters_AllFieldsSet(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured ready.RunInput
	runFn := func(_ context.Context, input ready.RunInput) error {
		captured = input
		return nil
	}

	// When
	cmd := ready.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{
		"ready",
		"--role", "task",
		"--state", "open",
		"--label", "kind:bug",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured.Filter.Roles) != 1 || captured.Filter.Roles[0] != domain.RoleTask {
		t.Errorf("Filter.Roles: got %v, want [task]", captured.Filter.Roles)
	}
	if len(captured.Filter.States) != 1 || captured.Filter.States[0] != domain.StateOpen {
		t.Errorf("Filter.States: got %v, want [open]", captured.Filter.States)
	}
	if len(captured.Filter.LabelFilters) != 1 ||
		captured.Filter.LabelFilters[0].Key != "kind" ||
		captured.Filter.LabelFilters[0].Value != "bug" {
		t.Errorf("Filter.LabelFilters: got %v, want [{Key:kind Value:bug}]", captured.Filter.LabelFilters)
	}
}
