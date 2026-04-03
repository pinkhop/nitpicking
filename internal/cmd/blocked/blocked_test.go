package blocked_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/blocked"
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

func noColor() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

// --- Run Tests ---

func TestRun_BlockedFilter_OnlyReturnsBlockedIssues(t *testing.T) {
	t.Parallel()

	// Given — one blocked task and one unblocked task
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker task")
	blockedID := createTask(t, svc, "Blocked task")
	_ = createTask(t, svc, "Unblocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
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
	if result.Items[0].ID != blockedID.String() {
		t.Errorf("item ID: got %q, want %q", result.Items[0].ID, blockedID.String())
	}
}

func TestRun_ExcludesClosed_BlockedButClosedNotReturned(t *testing.T) {
	t.Parallel()

	// Given — a blocked task that has been closed
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

	claimAndClose(t, svc, blockedID)

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("items: got %d, want 0 (closed blocked issues should be excluded)", len(result.Items))
	}
}

func TestRun_JSONOutput_ContainsExpectedFields(t *testing.T) {
	t.Parallel()

	// Given — one blocked task
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker")
	blockedID := createTask(t, svc, "Blocked")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
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

func TestRun_EmptyResult_TextShowsNoBlockedMessage(t *testing.T) {
	t.Parallel()

	// Given — no blocked issues
	svc := setupService(t)
	_ = createTask(t, svc, "Unblocked task")

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "No blocked issues") {
		t.Errorf("expected 'No blocked issues' message, got: %s", output)
	}
}

func TestRun_TextOutput_IncludesBlockedIssueDetails(t *testing.T) {
	t.Parallel()

	// Given — one blocked task
	svc := setupService(t)
	blockerID := createTask(t, svc, "The blocker")
	blockedID := createTask(t, svc, "The blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, blockedID.String()) {
		t.Errorf("expected blocked issue ID %s in output, got: %s", blockedID, output)
	}
	if !strings.Contains(output, "The blocked task") {
		t.Errorf("expected blocked issue title in output, got: %s", output)
	}
	if !strings.Contains(output, "1 blocked") {
		t.Errorf("expected '1 blocked' summary line in output, got: %s", output)
	}
}

func TestRun_TextOutput_IncludesStateColumn(t *testing.T) {
	t.Parallel()

	// Given — one blocked task.
	svc := setupService(t)
	blockerID := createTask(t, svc, "The blocker")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
	// Then — the output should include a state column showing "open (blocked)".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "open (blocked)") {
		t.Errorf("expected 'open (blocked)' state column in output, got: %s", output)
	}
}

func TestRun_JSONOutput_IncludesBlockerIDs(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by two issues.
	svc := setupService(t)
	blocker1 := createTask(t, svc, "Blocker one")
	blocker2 := createTask(t, svc, "Blocker two")
	blockedID := createTask(t, svc, "Blocked task")

	for _, blockerID := range []domain.ID{blocker1, blocker2} {
		err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
			TargetID: blockerID.String(),
			Type:     domain.RelBlockedBy,
		}, mustAuthor(t, "test-agent"))
		if err != nil {
			t.Fatalf("precondition: add relationship failed: %v", err)
		}
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw struct {
		Items []struct {
			ID         string   `json:"id"`
			BlockerIDs []string `json:"blocker_ids"`
		} `json:"items"`
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if len(raw.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(raw.Items))
	}
	if len(raw.Items[0].BlockerIDs) != 2 {
		t.Fatalf("blocker_ids: got %d, want 2", len(raw.Items[0].BlockerIDs))
	}
	blockerIDSet := make(map[string]bool)
	for _, id := range raw.Items[0].BlockerIDs {
		blockerIDSet[id] = true
	}
	if !blockerIDSet[blocker1.String()] {
		t.Errorf("blocker_ids missing %s", blocker1)
	}
	if !blockerIDSet[blocker2.String()] {
		t.Errorf("blocker_ids missing %s", blocker2)
	}
}

func TestRun_TextOutput_IncludesBlockerIDsAfterTitle(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by one domain.
	svc := setupService(t)
	blockerID := createTask(t, svc, "The blocker")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	// The blocker ID should appear in the text output after the title.
	if !strings.Contains(output, blockerID.String()) {
		t.Errorf("expected blocker ID %s in text output, got:\n%s", blockerID, output)
	}
}

func TestRun_OrdersByPriority_HigherPriorityFirst(t *testing.T) {
	t.Parallel()

	// Given — two blocked tasks with different priorities
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker")

	// Create two blocked tasks with different priorities.
	lowOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Low priority blocked",
		Author:   mustAuthor(t, "test-agent"),
		Priority: domain.P3,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	highOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "High priority blocked",
		Author:   mustAuthor(t, "test-agent"),
		Priority: domain.P0,
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	// Block both tasks.
	for _, id := range []domain.ID{lowOut.Issue.ID(), highOut.Issue.ID()} {
		err := svc.AddRelationship(t.Context(), id.String(), driving.RelationshipInput{
			TargetID: blockerID.String(),
			Type:     domain.RelBlockedBy,
		}, mustAuthor(t, "test-agent"))
		if err != nil {
			t.Fatalf("precondition: add relationship failed: %v", err)
		}
	}

	var buf bytes.Buffer
	input := blocked.RunInput{
		Service:     svc,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err = blocked.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result cmdutil.ListOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items: got %d, want 2", len(result.Items))
	}
	if result.Items[0].ID != highOut.Issue.ID().String() {
		t.Errorf("first item should be high priority, got ID %q, want %q",
			result.Items[0].ID, highOut.Issue.ID().String())
	}
}
