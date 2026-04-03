package jsoncmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Create test helpers ---

// setupCreateService initialises a service backed by in-memory fakes and
// returns it ready for create tests.
func setupCreateService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createParentEpic creates an epic and returns its ID, for use as a parent in
// create tests.
func createParentEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	return out.Issue.ID()
}

// --- RunCreate Tests ---

func TestRunCreate_CreatesTaskAndReturnsJSON(t *testing.T) {
	t.Parallel()

	// Given: a valid JSON object on stdin with minimal required fields.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"role": "task", "title": "Implement auth middleware"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and JSON output contains expected fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["role"] != "task" {
		t.Errorf("role: got %q, want %q", result["role"], "task")
	}
	if result["title"] != "Implement auth middleware" {
		t.Errorf("title: got %q, want %q", result["title"], "Implement auth middleware")
	}
	if result["state"] != "open" {
		t.Errorf("state: got %q, want %q", result["state"], "open")
	}
	if _, ok := result["id"]; !ok {
		t.Error("expected id in JSON output")
	}
}

func TestRunCreate_AllContentFields_PassedToService(t *testing.T) {
	t.Parallel()

	// Given: a JSON object with all optional content fields populated,
	// and --with-claim set via RunCreateInput.
	svc := setupCreateService(t)
	epicID := createParentEpic(t, svc, "Parent epic")

	stdinJSON := `{
		"role": "task",
		"title": "Full featured task",
		"description": "A detailed description",
		"acceptance_criteria": "All tests pass",
		"priority": "P0",
		"parent": "` + epicID.String() + `",
		"labels": ["kind:feat", "area:auth"]
	}`
	stdin := strings.NewReader(stdinJSON)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service:   svc,
		Author:    "bob",
		Stdin:     stdin,
		WriteTo:   &stdout,
		WithClaim: true,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and JSON output reflects the full set of fields.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["title"] != "Full featured task" {
		t.Errorf("title: got %q, want %q", result["title"], "Full featured task")
	}
	if result["priority"] != "P0" {
		t.Errorf("priority: got %q, want %q", result["priority"], "P0")
	}
	// --with-claim was set, so claim_id should be present.
	if _, ok := result["claim_id"]; !ok {
		t.Error("expected claim_id in JSON output when WithClaim=true")
	}
}

func TestRunCreate_DeferredField_Rejected(t *testing.T) {
	t.Parallel()

	// Given: a JSON object with the removed "deferred" field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"role": "task", "title": "Deferred task", "deferred": true}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "charlie",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned because "deferred" is an unknown field.
	if err == nil {
		t.Fatal("expected error for deferred field, got nil")
	}
}

func TestRunCreate_EmptyStdin_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: an empty stdin.
	svc := setupCreateService(t)

	stdin := strings.NewReader("")
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for empty stdin, got nil")
	}
}

func TestRunCreate_MissingRole_DefaultsToTask(t *testing.T) {
	t.Parallel()

	// Given: JSON without the "role" field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "No role specified"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and role defaults to task.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["role"] != "task" {
		t.Errorf("role: got %q, want %q", result["role"], "task")
	}
}

func TestRunCreate_MissingTitle_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON without the required "title" field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"role": "task"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned because title is required.
	if err == nil {
		t.Fatal("expected error for missing title field, got nil")
	}
}

func TestRunCreate_UnknownField_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON with an unknown field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"role": "task", "title": "Valid", "unknown": true}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned because unknown fields are rejected.
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestRunCreate_InvalidLabels_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: JSON with a label that doesn't follow key:value format.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"role": "task", "title": "Bad labels", "labels": ["no-colon"]}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned because the label format is invalid.
	if err == nil {
		t.Fatal("expected error for invalid label format, got nil")
	}
}

func TestRunCreate_LabelRemove_SilentlyIgnored(t *testing.T) {
	t.Parallel()

	// Given: JSON with label_remove, which is a json update field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "Task with label_remove", "label_remove": ["kind"]}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error — label_remove is accepted and silently ignored.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCreate_Comment_CreatesCommentOnNewIssue(t *testing.T) {
	t.Parallel()

	// Given: JSON with a comment field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "Task with comment", "comment": "Initial reasoning"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and a comment was added to the new issue.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	issueID, ok := result["id"].(string)
	if !ok {
		t.Fatal("expected id in JSON output")
	}

	comments, err := svc.ListComments(t.Context(), driving.ListCommentsInput{
		IssueID: issueID,
	})
	if err != nil {
		t.Fatalf("list comments failed: %v", err)
	}
	if len(comments.Comments) != 1 {
		t.Fatalf("comment count: got %d, want 1", len(comments.Comments))
	}
	if comments.Comments[0].Body != "Initial reasoning" {
		t.Errorf("comment body: got %q, want %q", comments.Comments[0].Body, "Initial reasoning")
	}
}

func TestRunCreate_ClaimInJSON_SilentlyIgnored(t *testing.T) {
	t.Parallel()

	// Given: JSON with claim=true, but WithClaim is false.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "Task with claim in JSON", "claim": true}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service:   svc,
		Author:    "alice",
		Stdin:     stdin,
		WriteTo:   &stdout,
		WithClaim: false,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and the issue is NOT claimed (claim in JSON is ignored).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["state"] != "open" {
		t.Errorf("state: got %q, want %q (claim in JSON should be ignored)", result["state"], "open")
	}
	if _, ok := result["claim_id"]; ok {
		t.Error("claim_id should not be present when WithClaim is false")
	}
}

func TestRunCreate_WithClaimFlag_ClaimsIssue(t *testing.T) {
	t.Parallel()

	// Given: JSON without claim field, but WithClaim is true.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "Task to claim via flag"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service:   svc,
		Author:    "alice",
		Stdin:     stdin,
		WriteTo:   &stdout,
		WithClaim: true,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error, and the issue is claimed.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}
	if result["state"] != "claimed" {
		t.Errorf("state: got %q, want %q", result["state"], "claimed")
	}
	if _, ok := result["claim_id"]; !ok {
		t.Error("expected claim_id in JSON output when WithClaim=true")
	}
}

func TestRunCreate_IdempotencyKey_Rejected(t *testing.T) {
	t.Parallel()

	// Given: JSON with the removed idempotency_key field.
	svc := setupCreateService(t)

	stdin := strings.NewReader(`{"title": "Task with idem key", "idempotency_key": "key-123"}`)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)

	// Then: an error is returned because idempotency_key is an unknown field.
	if err == nil {
		t.Fatal("expected error for idempotency_key field, got nil")
	}
}

func TestRunCreate_UnifiedSchema_AllFieldsAccepted(t *testing.T) {
	t.Parallel()

	// Given: a JSON object with all fields that appear in either json create
	// or json update, to verify that a single object can be piped to both.
	svc := setupCreateService(t)

	stdinJSON := `{
		"role": "task",
		"title": "Unified schema test",
		"description": "desc",
		"acceptance_criteria": "ac",
		"priority": "P2",
		"labels": ["kind:test"],
		"label_remove": ["old-key"],
		"comment": "unified comment",
		"claim": true
	}`
	stdin := strings.NewReader(stdinJSON)
	var stdout bytes.Buffer

	input := jsoncmd.RunCreateInput{
		Service: svc,
		Author:  "alice",
		Stdin:   stdin,
		WriteTo: &stdout,
	}

	// When
	err := jsoncmd.RunCreate(t.Context(), input)
	// Then: no error — all fields are accepted.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
