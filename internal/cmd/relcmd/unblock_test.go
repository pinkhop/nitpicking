package relcmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// --- RunUnblock Tests ---

func TestRunUnblock_RemovesBlockedByRelationship(t *testing.T) {
	t.Parallel()

	// Given: A is blocked_by B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskA, service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: taskB,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unblocking A and B.
	err = relcmd.RunUnblock(t.Context(), relcmd.RunUnblockInput{
		Service: svc,
		A:       taskA,
		B:       taskB,
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship is gone.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskA)
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	for _, r := range shown.Relationships {
		if r.Type() == issue.RelBlockedBy && r.TargetID() == taskB {
			t.Error("blocked_by relationship still exists after unblock")
		}
	}
}

func TestRunUnblock_RemovesReverseDirection(t *testing.T) {
	t.Parallel()

	// Given: B is blocked_by A (reverse direction from the unblock args).
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskB, service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: taskA,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unblocking A and B (A first, but the rel is B blocked_by A).
	err = relcmd.RunUnblock(t.Context(), relcmd.RunUnblockInput{
		Service: svc,
		A:       taskA,
		B:       taskB,
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error and the relationship is gone.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	shown, err := svc.ShowIssue(t.Context(), taskB)
	if err != nil {
		t.Fatalf("show issue failed: %v", err)
	}
	for _, r := range shown.Relationships {
		if r.Type() == issue.RelBlockedBy && r.TargetID() == taskA {
			t.Error("blocked_by relationship still exists after unblock")
		}
	}
}

func TestRunUnblock_NoRelationship_Succeeds(t *testing.T) {
	t.Parallel()

	// Given: no blocking relationship between A and B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: unblocking A and B (idempotent).
	err := relcmd.RunUnblock(t.Context(), relcmd.RunUnblockInput{
		Service: svc,
		A:       taskA,
		B:       taskB,
		Author:  author,
		WriteTo: &buf,
	})
	// Then: no error (idempotent operation).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunUnblock_JSON_OutputsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given: A is blocked_by B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskA, service.RelationshipInput{
		Type:     issue.RelBlockedBy,
		TargetID: taskB,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unblocking with JSON output.
	err = relcmd.RunUnblock(t.Context(), relcmd.RunUnblockInput{
		Service: svc,
		A:       taskA,
		B:       taskB,
		Author:  author,
		JSON:    true,
		WriteTo: &buf,
	})
	// Then: valid JSON with expected action field.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result["action"] != "unblocked" {
		t.Errorf("action: got %q, want %q", result["action"], "unblocked")
	}
}
