package relcmd_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// --- RunUncite Tests ---

func TestRunUncite_RemovesCitesRelationship(t *testing.T) {
	t.Parallel()

	// Given: A cites B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskA, service.RelationshipInput{
		Type:     issue.RelCites,
		TargetID: taskB,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unciting A and B.
	err = relcmd.RunUncite(t.Context(), relcmd.RunUnciteInput{
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
		if r.Type() == issue.RelCites && r.TargetID() == taskB {
			t.Error("cites relationship still exists after uncite")
		}
	}
}

func TestRunUncite_RemovesReverseDirection(t *testing.T) {
	t.Parallel()

	// Given: B cites A (reverse direction from the uncite args).
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskB, service.RelationshipInput{
		Type:     issue.RelCites,
		TargetID: taskA,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unciting A and B (A first, but the rel is B cites A).
	err = relcmd.RunUncite(t.Context(), relcmd.RunUnciteInput{
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
		if r.Type() == issue.RelCites && r.TargetID() == taskA {
			t.Error("cites relationship still exists after uncite")
		}
	}
}

func TestRunUncite_NoRelationship_Succeeds(t *testing.T) {
	t.Parallel()

	// Given: no citation relationship between A and B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	var buf bytes.Buffer

	// When: unciting A and B (idempotent).
	err := relcmd.RunUncite(t.Context(), relcmd.RunUnciteInput{
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

func TestRunUncite_JSON_OutputsStructuredResult(t *testing.T) {
	t.Parallel()

	// Given: A cites B.
	svc := setupService(t)
	taskA := createTask(t, svc, "Task A")
	taskB := createTask(t, svc, "Task B")
	author := mustAuthor(t, "test-agent")

	err := svc.AddRelationship(t.Context(), taskA, service.RelationshipInput{
		Type:     issue.RelCites,
		TargetID: taskB,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer

	// When: unciting with JSON output.
	err = relcmd.RunUncite(t.Context(), relcmd.RunUnciteInput{
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
	if result["action"] != "uncited" {
		t.Errorf("action: got %q, want %q", result["action"], "uncited")
	}
}
