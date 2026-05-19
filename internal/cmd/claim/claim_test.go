package claim_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/claim"
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

func createTaskWithLabels(t *testing.T, svc driving.Service, title string, labels []driving.LabelInput) domain.ID {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
		Labels: labels,
	})
	if err != nil {
		t.Fatalf("precondition: create issue with labels failed: %v", err)
	}
	return out.Issue.ID()
}

// --- RunClaimByID Tests ---

func TestRunClaimByID_ValidIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Claimable task")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		JSON:    false,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(issueID.String())) {
		t.Errorf("expected issue ID in output, got: %s", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Claim ID:")) {
		t.Errorf("expected 'Claim ID:' in output, got: %s", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("test-agent")) {
		t.Errorf("expected author name in output, got: %s", output)
	}
	if !bytes.Contains(buf.Bytes(), []byte("Expires at:")) {
		t.Errorf("expected 'Expires at:' in output, got: %s", output)
	}
}

func TestRunClaimByID_JSONOutput_ReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "JSON claim task")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, buf.String())
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	if _, ok := result["claim_id"]; !ok {
		t.Error("expected claim_id field in JSON output")
	}
	if result["author"] != "test-agent" {
		t.Errorf("author: got %q, want %q", result["author"], "test-agent")
	}
	if _, ok := result["created_at"]; !ok {
		t.Error("expected created_at field in JSON output")
	}
	if _, ok := result["expires_at"]; !ok {
		t.Error("expected expires_at field in JSON output")
	}
}

func TestRunClaimByID_WithDuration_PassesThrough(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Custom duration task")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service:  svc,
		IssueID:  issueID.String(),
		Author:   mustAuthor(t, "test-agent"),
		Duration: 4 * time.Hour,
		JSON:     true,
		WriteTo:  &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
}

func TestRunClaimByID_AlreadyClaimed_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — claim the issue first
	svc := setupService(t)
	issueID := createTask(t, svc, "Already claimed task")

	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "first-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: first claim failed: %v", err)
	}

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "second-agent"),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err = claim.RunClaimByID(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for already-claimed issue")
	}
}

// --- Guard-Rail Assertion Tests ---

func TestRunClaimByID_WithRoleGuardRail_MatchingRole_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a task issue
	svc := setupService(t)
	issueID := createTask(t, svc, "Task for role guard-rail")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		Role:    "task",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunClaimByID_WithRoleGuardRail_WrongRole_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a task issue, but we filter for epic
	svc := setupService(t)
	issueID := createTask(t, svc, "Task mismatched role")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		Role:    "epic",
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for role guard-rail failure")
	}
	errMsg := err.Error()
	if !bytes.Contains([]byte(errMsg), []byte(issueID.String())) {
		t.Errorf("error should name the issue ID, got: %s", errMsg)
	}
	if !bytes.Contains([]byte(errMsg), []byte("role")) {
		t.Errorf("error should mention role, got: %s", errMsg)
	}
}

func TestRunClaimByID_WithLabelGuardRail_MatchingLabel_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — a task with kind:bug label
	svc := setupService(t)
	issueID := createTaskWithLabels(t, svc, "Bug task", []driving.LabelInput{
		{Key: "kind", Value: "bug"},
	})

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind", Value: "bug"},
		},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunClaimByID_WithLabelGuardRail_WrongValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a task with kind:bug label, but we filter for kind:feature
	svc := setupService(t)
	issueID := createTaskWithLabels(t, svc, "Bug task wrong filter", []driving.LabelInput{
		{Key: "kind", Value: "bug"},
	})

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind", Value: "feature"},
		},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for label guard-rail failure")
	}
	errMsg := err.Error()
	if !bytes.Contains([]byte(errMsg), []byte(issueID.String())) {
		t.Errorf("error should name the issue ID, got: %s", errMsg)
	}
	if !bytes.Contains([]byte(errMsg), []byte("kind:feature")) {
		t.Errorf("error should name the unmet label condition, got: %s", errMsg)
	}
}

func TestRunClaimByID_WithLabelGuardRail_MissingKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a task with no labels, but we filter for kind:*
	svc := setupService(t)
	issueID := createTask(t, svc, "Task no labels")

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind"}, // wildcard — Value is empty
		},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error for label guard-rail failure")
	}
	errMsg := err.Error()
	if !bytes.Contains([]byte(errMsg), []byte(issueID.String())) {
		t.Errorf("error should name the issue ID, got: %s", errMsg)
	}
	if !bytes.Contains([]byte(errMsg), []byte("kind:*")) {
		t.Errorf("error should name the unmet label condition, got: %s", errMsg)
	}
}

func TestRunClaimByID_WithLabelGuardRail_Wildcard_MatchesAnyValue(t *testing.T) {
	t.Parallel()

	// Given — a task with kind:bug label, and we filter for kind:* (wildcard)
	svc := setupService(t)
	issueID := createTaskWithLabels(t, svc, "Bug task wildcard", []driving.LabelInput{
		{Key: "kind", Value: "bug"},
	})

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service: svc,
		IssueID: issueID.String(),
		Author:  mustAuthor(t, "test-agent"),
		LabelFilters: []driving.LabelFilterInput{
			{Key: "kind"}, // wildcard — Value is empty
		},
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- RunClaimByID ExpiresAt Tests ---

func TestRunClaimByID_WithExpiresAt_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Expires-at task")
	expiresAt := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)

	var buf bytes.Buffer
	input := claim.RunClaimByIDInput{
		Service:   svc,
		IssueID:   issueID.String(),
		Author:    mustAuthor(t, "test-agent"),
		ExpiresAt: expiresAt,
		JSON:      true,
		WriteTo:   &buf,
	}

	// When
	err := claim.RunClaimByID(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	gotExpiresAt, err := time.Parse(time.RFC3339, result["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at not valid RFC3339: %v", err)
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Errorf("expires_at: got %v, want %v", gotExpiresAt, expiresAt)
	}
}

// --- RunClaimReady Tests ---

func TestRunClaimReady_WithReadyIssue_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Ready task")

	var buf bytes.Buffer
	input := claim.RunClaimReadyInput{
		Service: svc,
		Author:  mustAuthor(t, "test-agent"),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimReady(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
}

func TestRunClaimReady_WithExpiresAt_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc := setupService(t)
	issueID := createTask(t, svc, "Ready expires-at task")
	expiresAt := time.Now().Add(5 * time.Hour).UTC().Truncate(time.Second)

	var buf bytes.Buffer
	input := claim.RunClaimReadyInput{
		Service:   svc,
		Author:    mustAuthor(t, "test-agent"),
		ExpiresAt: expiresAt,
		JSON:      true,
		WriteTo:   &buf,
	}

	// When
	err := claim.RunClaimReady(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if result["issue_id"] != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result["issue_id"], issueID.String())
	}
	gotExpiresAt, err := time.Parse(time.RFC3339, result["expires_at"].(string))
	if err != nil {
		t.Fatalf("expires_at not valid RFC3339: %v", err)
	}
	if !gotExpiresAt.Equal(expiresAt) {
		t.Errorf("expires_at: got %v, want %v", gotExpiresAt, expiresAt)
	}
}

func TestRunClaimReady_NoReadyIssues_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — no issues created
	svc := setupService(t)

	var buf bytes.Buffer
	input := claim.RunClaimReadyInput{
		Service: svc,
		Author:  mustAuthor(t, "test-agent"),
		JSON:    true,
		WriteTo: &buf,
	}

	// When
	err := claim.RunClaimReady(t.Context(), input)

	// Then
	if err == nil {
		t.Fatal("expected error when no ready issues exist")
	}
}
