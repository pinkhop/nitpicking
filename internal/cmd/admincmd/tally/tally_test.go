package tally_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/tally"
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

func claimAndClose(t *testing.T, svc driving.Service, id domain.ID) {
	t.Helper()
	ctx := t.Context()
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: id.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: id.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}
}

func claimAndDefer(t *testing.T, svc driving.Service, id domain.ID) {
	t.Helper()
	ctx := t.Context()
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: id.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: id.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionDefer,
	})
	if err != nil {
		t.Fatalf("precondition: defer failed: %v", err)
	}
}

func claimIssue(t *testing.T, svc driving.Service, id domain.ID) string {
	t.Helper()
	ctx := t.Context()
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: id.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	return claimOut.ClaimID
}

func noColor() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

// --- Run Tests ---

func TestRun_EmptyDatabase_AllCountsZero(t *testing.T) {
	t.Parallel()

	// Given — freshly initialized database with no issues
	svc := setupService(t)

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, field := range []string{"open", "deferred", "closed", "ready", "blocked", "total"} {
		v, ok := result[field]
		if !ok {
			t.Errorf("expected field %q in JSON output", field)
			continue
		}
		if v.(float64) != 0 {
			t.Errorf("%s: got %v, want 0", field, v)
		}
	}
}

func TestRun_CountsByState_MatchesIssueStates(t *testing.T) {
	t.Parallel()

	// Given — issues in various states:
	//   3 open (2 unclaimed + 1 claimed; claiming no longer changes primary state)
	//   1 deferred
	//   1 closed
	svc := setupService(t)

	// Two open tasks (remain open).
	_ = createTask(t, svc, "Open task 1")
	_ = createTask(t, svc, "Open task 2")

	// One claimed task — state stays open; claiming creates a claim row only.
	claimedID := createTask(t, svc, "Claimed task")
	_ = claimIssue(t, svc, claimedID)

	// One deferred task.
	deferredID := createTask(t, svc, "Deferred task")
	claimAndDefer(t, svc, deferredID)

	// One closed task.
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Open     int `json:"open"`
		Deferred int `json:"deferred"`
		Closed   int `json:"closed"`
		Total    int `json:"total"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Claimed issues remain open — all three unclaimed+claimed tasks appear under open.
	if result.Open != 3 {
		t.Errorf("open: got %d, want 3", result.Open)
	}
	if result.Deferred != 1 {
		t.Errorf("deferred: got %d, want 1", result.Deferred)
	}
	if result.Closed != 1 {
		t.Errorf("closed: got %d, want 1", result.Closed)
	}
	if result.Total != 5 {
		t.Errorf("total: got %d, want 5", result.Total)
	}
}

func TestRun_TotalIsSumOfStateCounts(t *testing.T) {
	t.Parallel()

	// Given — three open tasks (total should be 3)
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")
	_ = createTask(t, svc, "Task C")

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Open     int `json:"open"`
		Deferred int `json:"deferred"`
		Closed   int `json:"closed"`
		Total    int `json:"total"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	expectedTotal := result.Open + result.Deferred + result.Closed
	if result.Total != expectedTotal {
		t.Errorf("total: got %d, want sum of state counts %d", result.Total, expectedTotal)
	}
}

func TestRun_ReadyCount_OnlyCountsReadyIssues(t *testing.T) {
	t.Parallel()

	// Given — one open task (ready) and one blocked task (not ready)
	svc := setupService(t)
	readyID := createTask(t, svc, "Ready task")
	blockedID := createTask(t, svc, "Blocked task")

	// Block one task with the other.
	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: readyID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Ready   int `json:"ready"`
		Blocked int `json:"blocked"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Ready != 1 {
		t.Errorf("ready: got %d, want 1", result.Ready)
	}
	if result.Blocked != 1 {
		t.Errorf("blocked: got %d, want 1", result.Blocked)
	}
}

func TestRun_BlockedCount_ExcludesClosedIssues(t *testing.T) {
	t.Parallel()

	// Given — a blocked task that has been closed should not appear in blocked count
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker")
	blockedID := createTask(t, svc, "Blocked then closed")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// Close the blocked task.
	claimAndClose(t, svc, blockedID)

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Blocked int `json:"blocked"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.Blocked != 0 {
		t.Errorf("blocked: got %d, want 0 (closed issues should be excluded)", result.Blocked)
	}
}

func TestRun_JSONOutput_ContainsAllExpectedFields(t *testing.T) {
	t.Parallel()

	// Given — a single open task
	svc := setupService(t)
	_ = createTask(t, svc, "Single task")

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	requiredFields := []string{"open", "deferred", "closed", "ready", "blocked", "total"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field %q in JSON output", field)
		}
	}
}

func TestRun_TextOutput_IncludesStateLabelsAndCounts(t *testing.T) {
	t.Parallel()

	// Given — two open tasks
	svc := setupService(t)
	_ = createTask(t, svc, "Task 1")
	_ = createTask(t, svc, "Task 2")

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// Verify state labels are present.
	for _, label := range []string{"Open", "Deferred", "Closed", "Ready", "Blocked"} {
		if !strings.Contains(output, label) {
			t.Errorf("expected %q label in text output, got: %s", label, output)
		}
	}

	// Verify total line is present.
	if !strings.Contains(output, "total") {
		t.Errorf("expected 'total' in text output, got: %s", output)
	}
}

func TestRun_TextOutput_IncludesTotalLine(t *testing.T) {
	t.Parallel()

	// Given — three open tasks
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")
	_ = createTask(t, svc, "Task C")

	var buf bytes.Buffer
	input := tally.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := tally.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "3 total") {
		t.Errorf("expected '3 total' in text output, got: %s", output)
	}
}
