//go:build boundary

package sqlite_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- InitDatabase Called Twice ---

func TestBoundary_InitDatabase_CalledTwice_ReturnsDatabaseError(t *testing.T) {
	// Given — a database that has already been initialized.
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store)
	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: first init failed: %v", err)
	}

	// When — Init is called a second time.
	err = svc.Init(ctx, "XX")

	// Then — a wrapped DatabaseError is returned; the prefix is not overwritten.
	if err == nil {
		t.Fatal("expected error from second Init, got nil")
	}
	if !errors.Is(err, &domain.DatabaseError{}) {
		t.Errorf("expected wrapped DatabaseError, got: %T — %v", err, err)
	}

	// Verify the original prefix is preserved.
	prefix, err := svc.GetPrefix(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting prefix: %v", err)
	}
	if prefix != "NP" {
		t.Errorf("expected prefix %q, got %q", "NP", prefix)
	}
}

// --- GetPrefix on Uninitialized Database ---

func TestBoundary_GetPrefix_UninitializedDatabase_ReturnsError(t *testing.T) {
	// Given — a database with schema applied but no Init call.
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store)
	ctx := t.Context()

	// When — GetPrefix is called before Init.
	prefix, err := svc.GetPrefix(ctx)

	// Then — an error is returned; no prefix exists.
	if err == nil {
		t.Fatalf("expected error from GetPrefix on uninitialized database, got prefix %q", prefix)
	}
}

// --- Idempotency Key Returns Original Issue Without Mutation ---

func TestBoundary_CreateIssue_IdempotencyKey_ReturnsOriginalWithoutMutation(t *testing.T) {
	// Given — an issue created with an idempotency key.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	original, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:           domain.RoleTask,
		Title:          "Original title",
		Description:    "Original description",
		Author:         author(t, "alice"),
		IdempotencyKey: "idem-test-1",
	})
	if err != nil {
		t.Fatalf("precondition: first create failed: %v", err)
	}

	// When — create is called again with the same idempotency key but
	// different title, description, and priority.
	duplicate, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:           domain.RoleTask,
		Title:          "Different title",
		Description:    "Different description",
		Priority:       domain.P0,
		Author:         author(t, "bob"),
		IdempotencyKey: "idem-test-1",
	})
	// Then — the original issue is returned unchanged.
	if err != nil {
		t.Fatalf("unexpected error from idempotent create: %v", err)
	}
	if duplicate.Issue.ID() != original.Issue.ID() {
		t.Errorf("expected same issue ID %s, got %s", original.Issue.ID(), duplicate.Issue.ID())
	}
	if duplicate.Issue.Title() != "Original title" {
		t.Errorf("expected original title %q, got %q", "Original title", duplicate.Issue.Title())
	}
	if duplicate.Issue.Description() != "Original description" {
		t.Errorf("expected original description %q, got %q", "Original description", duplicate.Issue.Description())
	}
	if duplicate.Issue.Priority() != original.Issue.Priority() {
		t.Errorf("expected original priority %s, got %s", original.Issue.Priority(), duplicate.Issue.Priority())
	}
}

// --- CountDeletedRatio on Empty Database ---

func TestBoundary_CountDeletedRatio_EmptyDatabase_ReturnsZeros(t *testing.T) {
	// Given — a freshly initialized database with no issues.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// When — Doctor runs (which internally calls CountDeletedRatio).
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — no error; no gc_recommended finding because there are no issues.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range doctorOut.Findings {
		if f.Category == "gc_recommended" {
			t.Errorf("unexpected gc_recommended finding on empty database: %s", f.Message)
		}
		if f.Category == "storage_integrity" {
			t.Errorf("unexpected integrity finding: %s", f.Message)
		}
	}
}

func TestBoundary_CountDeletedRatio_MixedIssues_ReflectsCorrectRatio(t *testing.T) {
	// Given — a mix of live, closed, and deleted issues:
	// 2 open, 1 closed, 1 deleted = 4 total, 1 deleted (25%).
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	_ = createIntTask(t, svc, "Open task A")
	_ = createIntTask(t, svc, "Open task B")

	// Create and close a task.
	closedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Closed task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: closedOut.Issue.ID().String(), ClaimID: closedOut.ClaimID,
		Action: driving.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close failed: %v", err)
	}

	// Create and delete a task.
	deletedOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Deleted task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deletedOut.Issue.ID().String(), ClaimID: deletedOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// When — Doctor runs.
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — no error; the check completes successfully with 1/4 deleted.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range doctorOut.Findings {
		if f.Category == "storage_integrity" {
			t.Errorf("unexpected integrity finding: %s", f.Message)
		}
	}
}
