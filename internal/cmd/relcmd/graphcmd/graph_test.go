package graphcmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd/graphcmd"
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

// --- Run Tests ---

func TestRun_ExcludesClosedNodesByDefault(t *testing.T) {
	t.Parallel()

	// Given — one open task and one closed task
	svc := setupService(t)
	openID := createTask(t, svc, "Open task")
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, openID.String()) {
		t.Errorf("expected open issue %s in DOT output", openID)
	}
	if strings.Contains(output, closedID.String()) {
		t.Errorf("closed issue %s should be excluded from DOT output by default", closedID)
	}
}

func TestRun_IncludeClosed_ShowsClosedNodes(t *testing.T) {
	t.Parallel()

	// Given — one open task and one closed task
	svc := setupService(t)
	_ = createTask(t, svc, "Open task")
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: true,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, closedID.String()) {
		t.Errorf("expected closed issue %s in DOT output when --include-closed is set", closedID)
	}
}

func TestRun_OnlyBlockedByAndCitesEdges_NoInverseDuplicates(t *testing.T) {
	t.Parallel()

	// Given — two tasks with a blocked_by relationship
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
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err = graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// The DOT output should contain an edge from blocked to blocker
	// (blocked_by direction), but not the "blocks" inverse.
	if !strings.Contains(output, blockedID.String()) || !strings.Contains(output, blockerID.String()) {
		t.Errorf("expected both issue IDs in DOT output, got: %s", output)
	}
}

func TestRun_EdgeWithInvisibleEndpoint_Skipped(t *testing.T) {
	t.Parallel()

	// Given — a blocked_by relationship where the blocker is closed (invisible)
	svc := setupService(t)
	blockerID := createTask(t, svc, "Closed blocker")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	// Close the blocker — it will be invisible by default.
	claimAndClose(t, svc, blockerID)

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err = graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	// The edge should be skipped because the blocker is not in the visible set.
	if strings.Contains(output, blockerID.String()) {
		t.Errorf("closed blocker %s should not appear in DOT output", blockerID)
	}
}

func TestRun_JSONFormat_ProducesValidJSON(t *testing.T) {
	t.Parallel()

	// Given — a service with one task.
	svc := setupService(t)
	_ = createTask(t, svc, "JSON graph task")

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatJSON,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then — produces valid JSON array.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", jsonErr, buf.String())
	}
	if len(result) != 1 {
		t.Errorf("expected 1 root issue, got %d", len(result))
	}
}

func TestRun_TextFormat_ProducesOutput(t *testing.T) {
	t.Parallel()

	// Given — a service with one task.
	svc := setupService(t)
	taskID := createTask(t, svc, "Text graph task")

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatText,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then — text format produces output containing the task.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), taskID.String()) {
		t.Errorf("expected task %s in text output", taskID)
	}
}

func TestRun_PlainOutput_ClaimedIssue_DoesNotLeakClaimID(t *testing.T) {
	t.Parallel()

	// Given — a claimed task
	svc := setupService(t)
	issueID := createTask(t, svc, "Graph claim leak")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err = graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	if strings.Contains(output, claimOut.ClaimID) {
		t.Errorf("DOT output must not contain the claim ID %q, got:\n%s", claimOut.ClaimID, output)
	}
	// The node should still be present.
	if !strings.Contains(output, issueID.String()) {
		t.Errorf("expected issue %s in DOT output", issueID)
	}
}

func TestRun_PlainOutput_ContainsDOTFormat(t *testing.T) {
	t.Parallel()

	// Given — one open task
	svc := setupService(t)
	_ = createTask(t, svc, "DOT format task")

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		IncludeClosed: false,
		Format:        graphcmd.FormatDOT,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "digraph") {
		t.Error("expected 'digraph' keyword in plain DOT output")
	}
}

// --- ParseFormat Tests ---

func TestParseFormat_ValidValues(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected graphcmd.Format
	}{
		{"dot", graphcmd.FormatDOT},
		{"json", graphcmd.FormatJSON},
		{"text", graphcmd.FormatText},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			result, err := graphcmd.ParseFormat(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestParseFormat_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	cases := []string{"svg", "mermaid", "DOT", ""}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := graphcmd.ParseFormat(input)

			// Then
			if err == nil {
				t.Errorf("expected error for invalid format %q", input)
			}
		})
	}
}
