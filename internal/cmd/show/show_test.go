package show_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/show"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
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

func TestRun_JSONOutput_IncludesAllRequiredFields(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Show test task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}

	requiredFields := []string{"id", "role", "title", "priority", "state", "revision", "is_ready", "created_at"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field %q in JSON output", field)
		}
	}
	if result["id"] != issueID.String() {
		t.Errorf("id: got %q, want %q", result["id"], issueID.String())
	}
	if result["role"] != "task" {
		t.Errorf("role: got %q, want %q", result["role"], "task")
	}
	if result["title"] != "Show test task" {
		t.Errorf("title: got %q, want %q", result["title"], "Show test task")
	}
}

func TestRun_TextOutput_IncludesIssueIDAndTitle(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Text display task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, issueID.String()) {
		t.Errorf("expected issue ID in text output, got: %s", output)
	}
	if !strings.Contains(output, "Text display task") {
		t.Errorf("expected title in text output, got: %s", output)
	}
}

func TestRun_TextOutput_OmitsParentWhenNone(t *testing.T) {
	t.Parallel()

	// Given — task with no parent
	svc := setupService(t)
	issueID := createTask(t, svc, "Orphan task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Parent:") {
		t.Error("expected 'Parent:' field to always be present in output")
	}
	if !strings.Contains(output, "(none)") {
		t.Error("expected '(none)' for parentless issue")
	}
}

func TestRun_TextOutput_OmitsClaimWhenUnclaimed(t *testing.T) {
	t.Parallel()

	// Given — unclaimed task
	svc := setupService(t)
	issueID := createTask(t, svc, "Unclaimed task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Claimed by:") {
		t.Error("expected 'Claimed by:' field to always be present in output")
	}
	if !strings.Contains(output, "(none)") {
		t.Error("expected '(none)' for unclaimed issue")
	}
}

func TestRun_TextOutput_ClaimedIssue_RedactsClaimID(t *testing.T) {
	t.Parallel()

	// Given — a claimed task
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task")

	ctx := t.Context()
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "claimer"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(ctx, input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// The claim ID must NOT appear in text output.
	if strings.Contains(output, claimOut.ClaimID) {
		t.Errorf("text output must not contain the claim ID %q, got:\n%s", claimOut.ClaimID, output)
	}

	// The output should still show that the issue is claimed and by whom.
	if !strings.Contains(output, "claimer") {
		t.Errorf("expected claim author 'claimer' in text output, got:\n%s", output)
	}
	if !strings.Contains(output, "Claimed by:") {
		t.Errorf("expected 'Claimed by:' label in text output, got:\n%s", output)
	}
}

func TestRun_JSONOutput_WithRelationship_IncludesRelationships(t *testing.T) {
	t.Parallel()

	// Given — two tasks with a blocked_by relationship
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker task")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: blockedID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		Relationships []struct {
			Type     string `json:"type"`
			TargetID string `json:"target_id"`
		} `json:"relationships"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Relationships) == 0 {
		t.Fatal("expected at least one relationship")
	}
	if result.Relationships[0].Type != "blocked_by" {
		t.Errorf("type: got %q, want %q", result.Relationships[0].Type, "blocked_by")
	}
	if result.Relationships[0].TargetID != blockerID.String() {
		t.Errorf("target_id: got %q, want %q", result.Relationships[0].TargetID, blockerID.String())
	}
}

func TestRun_TextOutput_RelationshipsFormatted(t *testing.T) {
	t.Parallel()

	// Given — task with a relationship
	svc := setupService(t)
	id1 := createTask(t, svc, "Task A")
	id2 := createTask(t, svc, "Task B")

	err := svc.AddRelationship(t.Context(), id1.String(), driving.RelationshipInput{
		TargetID: id2.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: id1.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Blocked by (") {
		t.Error("expected 'Blocked by (' section header in text output")
	}
	if !strings.Contains(output, id2.String()) {
		t.Errorf("expected target ID %s in blocked-by list", id2)
	}
}

func TestRun_JSONOutput_ChildCountForEpic(t *testing.T) {
	t.Parallel()

	// Given — epic with a child task
	svc := setupService(t)
	epicID := createEpic(t, svc, "Parent epic")

	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: epicID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		ChildCount int `json:"child_count"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.ChildCount != 1 {
		t.Errorf("child_count: got %d, want 1", result.ChildCount)
	}
}

func TestRun_JSONOutput_ZeroChildCount_IncludesChildCount(t *testing.T) {
	t.Parallel()

	// Given — a task with no children.
	svc := setupService(t)
	issueID := createTask(t, svc, "Childless task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — child_count must be present even when zero.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["child_count"]; !exists {
		t.Errorf("child_count must be present in JSON output even when 0")
	}
	var result struct {
		ChildCount int `json:"child_count"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.ChildCount != 0 {
		t.Errorf("child_count: got %d, want 0", result.ChildCount)
	}
}

func TestRun_TextOutput_ZeroChildCount_DisplaysChildren(t *testing.T) {
	t.Parallel()

	// Given — a task with no children.
	svc := setupService(t)
	issueID := createTask(t, svc, "Childless text task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — text output must include "Children: 0".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Children (0):") {
		t.Errorf("expected 'Children (0):' in text output, got:\n%s", output)
	}
}

func TestRun_JSONOutput_ClaimedIssue_OmitsClaimID(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task for redaction test")

	claimOut, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "claimer-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	if claimOut.ClaimID == "" {
		t.Fatalf("precondition: claim returned empty claim ID")
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When — show the claimed issue as JSON.
	err = show.Run(t.Context(), input)
	// Then — claim_id must not appear in the JSON output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["claim_id"]; exists {
		t.Errorf("claim_id must not appear in show --json output; bearer token leaked")
	}
	// claim_author and claim_stale_at should still be present.
	if _, exists := raw["claim_author"]; !exists {
		t.Errorf("claim_author should be present for a claimed issue")
	}
	if _, exists := raw["claim_stale_at"]; !exists {
		t.Errorf("claim_stale_at should be present for a claimed issue")
	}
}

func TestRun_JSONOutput_ClaimedIssue_IncludesClaimedAt(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task with timestamp")

	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "claimer-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — claimed_at must appear as an ISO 8601 timestamp.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["claimed_at"]; !exists {
		t.Errorf("claimed_at must be present in JSON output for a claimed issue")
	}
}

func TestRun_JSONOutput_UnclaimedIssue_OmitsClaimedAt(t *testing.T) {
	t.Parallel()

	// Given — an unclaimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Unclaimed task no timestamp")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — claimed_at must be absent for unclaimed issues.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["claimed_at"]; exists {
		t.Errorf("claimed_at must not appear for an unclaimed issue")
	}
}

func TestRun_JSONOutput_EpicWithChildren_IncludesChildrenArray(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks.
	svc := setupService(t)
	epicID := createEpic(t, svc, "Parent epic with children")

	child1, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task alpha",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child 1 failed: %v", err)
	}
	child2, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task beta",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child 2 failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: epicID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — children array must be present with both child IDs.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		ChildCount int `json:"child_count"`
		Children   []struct {
			ID       string `json:"id"`
			Role     string `json:"role"`
			State    string `json:"state"`
			Priority string `json:"priority"`
			Title    string `json:"title"`
		} `json:"children"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.ChildCount != 2 {
		t.Errorf("child_count: got %d, want 2", result.ChildCount)
	}
	if len(result.Children) != 2 {
		t.Fatalf("children: got %d items, want 2", len(result.Children))
	}

	childIDs := map[string]bool{
		child1.Issue.ID().String(): false,
		child2.Issue.ID().String(): false,
	}
	for _, c := range result.Children {
		childIDs[c.ID] = true
	}
	for id, found := range childIDs {
		if !found {
			t.Errorf("expected child ID %s in children array", id)
		}
	}
}

func TestRun_JSONOutput_IssueWithComments_IncludesRecentComments(t *testing.T) {
	t.Parallel()

	// Given — a task with 4 comments (should return 3 most recent).
	svc := setupService(t)
	issueID := createTask(t, svc, "Task with comments")
	author := mustAuthor(t, "commenter")

	for i, body := range []string{"First", "Second", "Third", "Fourth"} {
		_, err := svc.AddComment(t.Context(), driving.AddCommentInput{
			IssueID: issueID.String(),
			Author:  author,
			Body:    body,
		})
		if err != nil {
			t.Fatalf("precondition: add comment %d failed: %v", i+1, err)
		}
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — comments array contains 3 most recent, comment_count is 4.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		CommentCount int `json:"comment_count"`
		Comments     []struct {
			ID        string `json:"id"`
			Author    string `json:"author"`
			Body      string `json:"body"`
			CreatedAt string `json:"created_at"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.CommentCount != 4 {
		t.Errorf("comment_count: got %d, want 4", result.CommentCount)
	}
	if len(result.Comments) != 3 {
		t.Fatalf("comments: got %d items, want 3", len(result.Comments))
	}
	// The three most recent comments should be Second, Third, Fourth.
	wantBodies := []string{"Second", "Third", "Fourth"}
	for i, want := range wantBodies {
		if result.Comments[i].Body != want {
			t.Errorf("comments[%d].body: got %q, want %q", i, result.Comments[i].Body, want)
		}
	}
}

func TestRun_JSONOutput_NoComments_OmitsCommentsArray(t *testing.T) {
	t.Parallel()

	// Given — a task with no comments.
	svc := setupService(t)
	issueID := createTask(t, svc, "No comments task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — comments must be absent from output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["comments"]; exists {
		t.Errorf("comments must not appear in JSON output when there are no comments")
	}
}

func TestRun_JSONOutput_NoChildren_OmitsChildrenArray(t *testing.T) {
	t.Parallel()

	// Given — a task with no children.
	svc := setupService(t)
	issueID := createTask(t, svc, "No children task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then — children array must be absent; child_count still present as 0.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if _, exists := raw["children"]; exists {
		t.Errorf("children array must not appear when there are no children")
	}
	if _, exists := raw["child_count"]; !exists {
		t.Errorf("child_count must still be present even when 0")
	}
}

// --- Word wrapping ---

func TestRun_TextOutput_WrapsDescriptionAtTerminalWidth(t *testing.T) {
	t.Parallel()

	// Given — create an issue with a long description.
	svc := setupService(t)
	longDesc := "The quick brown fox jumps over the lazy dog and then runs through the forest at high speed"
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "Wrap test",
		Description: longDesc,
		Author:      mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service:       svc,
		IssueID:       out.Issue.ID().String(),
		TerminalWidth: 40,
		WriteTo:       &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	// The description should be wrapped — no single line of the description
	// section should exceed the terminal width.
	descIdx := strings.Index(output, "Description:\n")
	if descIdx < 0 {
		t.Fatal("expected Description section in output")
	}
	descSection := output[descIdx+len("Description:\n"):]
	for _, line := range strings.Split(descSection, "\n") {
		if line == "" {
			break // End of description section.
		}
		if len(line) > 40 {
			t.Errorf("line exceeds terminal width of 40: %q (len=%d)", line, len(line))
		}
	}
}

func TestRun_TextOutput_ZeroWidth_NoWrapping(t *testing.T) {
	t.Parallel()

	// Given — create an issue with a long description.
	svc := setupService(t)
	longDesc := "The quick brown fox jumps over the lazy dog and then runs through the forest"
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:        domain.RoleTask,
		Title:       "No wrap test",
		Description: longDesc,
		Author:      mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service:       svc,
		IssueID:       out.Issue.ID().String(),
		TerminalWidth: 0, // Non-TTY: no wrapping.
		WriteTo:       &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, longDesc) {
		t.Errorf("expected unwrapped description in output")
	}
}

// --- Comments section ---

func TestRun_TextOutput_ShowsRecentComments(t *testing.T) {
	t.Parallel()

	// Given — create an issue with 2 comments.
	svc := setupService(t)
	issueID := createTask(t, svc, "Issue with comments")
	author := mustAuthor(t, "comment-author")

	_, err := svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: issueID.String(),
		Author:  author,
		Body:    "First comment body",
	})
	if err != nil {
		t.Fatalf("precondition: add comment failed: %v", err)
	}
	_, err = svc.AddComment(t.Context(), driving.AddCommentInput{
		IssueID: issueID.String(),
		Author:  author,
		Body:    "Second comment body",
	})
	if err != nil {
		t.Fatalf("precondition: add comment failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "First comment body") {
		t.Errorf("expected first comment in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Second comment body") {
		t.Errorf("expected second comment in output, got:\n%s", output)
	}
	if !strings.Contains(output, "comment-author") {
		t.Errorf("expected comment author in output, got:\n%s", output)
	}
	// Comments section header should show count.
	if !strings.Contains(output, "Comments (2)") {
		t.Errorf("expected 'Comments (2)' header, got:\n%s", output)
	}
}

func TestRun_TextOutput_TruncatesTo3MostRecent(t *testing.T) {
	t.Parallel()

	// Given — create an issue with 5 comments.
	svc := setupService(t)
	issueID := createTask(t, svc, "Issue with many comments")
	author := mustAuthor(t, "commenter")

	for i := range 5 {
		_, err := svc.AddComment(t.Context(), driving.AddCommentInput{
			IssueID: issueID.String(),
			Author:  author,
			Body:    fmt.Sprintf("Comment number %d", i+1),
		})
		if err != nil {
			t.Fatalf("precondition: add comment %d failed: %v", i+1, err)
		}
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	// Should show the 3 most recent (3, 4, 5), not the first 2.
	if strings.Contains(output, "Comment number 1") {
		t.Errorf("should not show oldest comment (1)")
	}
	if strings.Contains(output, "Comment number 2") {
		t.Errorf("should not show second oldest comment (2)")
	}
	if !strings.Contains(output, "Comment number 3") {
		t.Errorf("expected comment 3 in output")
	}
	if !strings.Contains(output, "Comment number 5") {
		t.Errorf("expected comment 5 in output")
	}
	// Should show "2 earlier" indicator.
	if !strings.Contains(output, "2 earlier") {
		t.Errorf("expected '2 earlier' indicator, got:\n%s", output)
	}
}

// --- Claimed issue state display ---

// TestRun_JSONOutput_ClaimedIssue_StateIsOpenWithSecondary verifies that a
// claimed issue is rendered with state "open" and secondary_state "claimed" in
// JSON output, reflecting the three-state model where claimed is no longer a
// primary lifecycle state.
func TestRun_JSONOutput_ClaimedIssue_StateIsOpenWithSecondary(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task state check")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "state-checker"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — primary state must be "open"; secondary_state must be "claimed".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		State          string `json:"state"`
		SecondaryState string `json:"secondary_state"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if result.State != "open" {
		t.Errorf("state: got %q, want %q", result.State, "open")
	}
	if result.SecondaryState != "claimed" {
		t.Errorf("secondary_state: got %q, want %q", result.SecondaryState, "claimed")
	}
}

// TestRun_TextOutput_ClaimedIssue_StateDisplaysOpenClaimed verifies that a
// claimed issue's state is displayed as "open (claimed)" in text output, not
// as a standalone "claimed" primary state.
func TestRun_TextOutput_ClaimedIssue_StateDisplaysOpenClaimed(t *testing.T) {
	t.Parallel()

	// Given — a claimed task.
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimed task text state check")
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "state-checker"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — text output must show "open (claimed)" as the state.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "open (claimed)") {
		t.Errorf("expected 'open (claimed)' in text output for claimed issue, got:\n%s", output)
	}
}

func TestRun_TextOutput_NoComments_ShowsCount(t *testing.T) {
	t.Parallel()

	// Given — issue with no comments.
	svc := setupService(t)
	issueID := createTask(t, svc, "No comments issue")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	// When there are no comments, should not show comments section header.
	if strings.Contains(output, "Comments") {
		t.Errorf("should not show Comments section when none exist, got:\n%s", output)
	}
}

// --- Relationship Section Tests ---

func TestRun_TextOutput_ParentShowsTitle(t *testing.T) {
	t.Parallel()

	// Given — a task with a parent epic
	svc := setupService(t)
	epicID := createEpic(t, svc, "Parent epic title")
	childOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: childOut.Issue.ID().String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — parent line should include both ID and title
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, epicID.String()) {
		t.Errorf("expected parent ID %s in output, got:\n%s", epicID, output)
	}
	if !strings.Contains(output, "Parent epic title") {
		t.Errorf("expected parent title in output, got:\n%s", output)
	}
}

func TestRun_TextOutput_ChildrenListShowsTitles(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks
	svc := setupService(t)
	epicID := createEpic(t, svc, "Epic with children")
	_, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child Alpha",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child 1 failed: %v", err)
	}
	_, err = svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child Beta",
		Author:   mustAuthor(t, "test-agent"),
		ParentID: epicID.String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child 2 failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: epicID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — children section should list both child titles
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Children (2)") {
		t.Errorf("expected 'Children (2)' header in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Child Alpha") {
		t.Errorf("expected 'Child Alpha' in children list, got:\n%s", output)
	}
	if !strings.Contains(output, "Child Beta") {
		t.Errorf("expected 'Child Beta' in children list, got:\n%s", output)
	}
}

func TestRun_TextOutput_BlockedByShowsTargetDetails(t *testing.T) {
	t.Parallel()

	// Given — a task blocked by another task
	svc := setupService(t)
	blockerID := createTask(t, svc, "The blocker task")
	blockedID := createTask(t, svc, "Blocked task")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: blockedID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err = show.Run(t.Context(), input)
	// Then — blocked-by section should show the blocker's ID and title
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Blocked by (1)") {
		t.Errorf("expected 'Blocked by (1)' section header, got:\n%s", output)
	}
	if !strings.Contains(output, blockerID.String()) {
		t.Errorf("expected blocker ID %s in output, got:\n%s", blockerID, output)
	}
	if !strings.Contains(output, "The blocker task") {
		t.Errorf("expected blocker title in output, got:\n%s", output)
	}
}

func TestRun_TextOutput_AuthorAppearsBeforeRevision(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Author ordering task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	authorIdx := strings.Index(output, "Author:")
	revisionIdx := strings.Index(output, "Revision:")
	if authorIdx < 0 {
		t.Fatal("expected 'Author:' in text output")
	}
	if revisionIdx < 0 {
		t.Fatal("expected 'Revision:' in text output")
	}
	if authorIdx >= revisionIdx {
		t.Errorf("expected Author to appear before Revision in text output, but Author at %d, Revision at %d", authorIdx, revisionIdx)
	}
}

func TestRun_TextOutput_NoAcceptanceCriteria_ShowsNonePlaceholder(t *testing.T) {
	t.Parallel()

	// Given — task with no acceptance criteria
	svc := setupService(t)
	issueID := createTask(t, svc, "No AC task")

	var buf bytes.Buffer
	input := show.RunInput{
		Service: svc,
		IssueID: issueID.String(),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := show.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Acceptance Criteria:") {
		t.Error("expected 'Acceptance Criteria:' section to always be present in output")
	}
	// After the "Acceptance Criteria:" header, the next line should be "(none)".
	acIdx := strings.Index(output, "Acceptance Criteria:")
	afterAC := output[acIdx+len("Acceptance Criteria:"):]
	// Extract up to the next double-newline (section boundary).
	endIdx := strings.Index(afterAC, "\n\n")
	if endIdx > 0 {
		afterAC = afterAC[:endIdx]
	}
	if !strings.Contains(afterAC, "(none)") {
		t.Errorf("expected '(none)' placeholder after Acceptance Criteria header, got:%s", afterAC)
	}
}
