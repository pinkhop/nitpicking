//go:build boundary

package sqlite_test

import (
	"context"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Create Followed by Open Preserves Metadata ---

func TestBoundary_CreateThenOpen_PreservesPrefix(t *testing.T) {
	// Given — a database created with prefix "LIFE" and one domain.
	dbPath := t.TempDir() + "/lifecycle.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: create database failed: %v", err)
	}

	svc := core.New(store)
	ctx := t.Context()
	if err := svc.Init(ctx, "LIFE"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Persisted task",
		Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}
	issueID := createOut.Issue.ID()
	store.Close()

	// When — reopen the same database file with Open.
	store2, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open database failed: %v", err)
	}
	t.Cleanup(func() { store2.Close() })

	svc2 := core.New(store2)

	// Then — the prefix and issue data survive the close→open cycle.
	showOut, err := svc2.ShowIssue(ctx, issueID.String())
	if err != nil {
		t.Fatalf("show issue after reopen failed: %v", err)
	}
	if showOut.Title != "Persisted task" {
		t.Errorf("title: got %q, want %q", showOut.Title, "Persisted task")
	}

	// Creating a new issue should use the same prefix.
	createOut2, err := svc2.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Second task after reopen",
		Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("create after reopen failed: %v", err)
	}
	// The issue ID should start with the original prefix.
	if got := createOut2.Issue.ID().String(); len(got) < 4 || got[:4] != "LIFE" {
		t.Errorf("issue ID prefix: got %q, want prefix %q", got, "LIFE")
	}
}

// --- Vacuum After Mutations Leaves Database Readable ---

func TestBoundary_Vacuum_AfterMutations_LeavesDBReadable(t *testing.T) {
	// Given — a database with several issues, some deleted.
	store, svc := setupStoreAndSvc(t)
	ctx := t.Context()

	keepID := createIntTask(t, svc, "Keep this task")

	deleteOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Delete this task", Author: author(t, "alice"),
		Claim: true,
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	err = svc.DeleteIssue(ctx, driving.DeleteInput{
		IssueID: deleteOut.Issue.ID().String(), ClaimID: deleteOut.ClaimID,
	})
	if err != nil {
		t.Fatalf("precondition: delete failed: %v", err)
	}

	// Run GC to physically remove the deleted domain.
	_, err = svc.GC(ctx, driving.GCInput{IncludeClosed: false})
	if err != nil {
		t.Fatalf("precondition: GC failed: %v", err)
	}

	// When — vacuum the database.
	err = store.Vacuum(ctx)
	// Then — vacuum succeeds and the surviving data is still readable.
	if err != nil {
		t.Fatalf("vacuum failed: %v", err)
	}

	showOut, err := svc.ShowIssue(ctx, keepID.String())
	if err != nil {
		t.Fatalf("show issue after vacuum failed: %v", err)
	}
	if showOut.Title != "Keep this task" {
		t.Errorf("title after vacuum: got %q, want %q", showOut.Title, "Keep this task")
	}

	// List should still work.
	listOut, err := svc.ListIssues(ctx, driving.ListIssuesInput{Limit: 10})
	if err != nil {
		t.Fatalf("list after vacuum failed: %v", err)
	}
	if len(listOut.Items) != 1 {
		t.Errorf("expected 1 issue after vacuum, got %d", len(listOut.Items))
	}
}

// --- Foreign Key Enforcement Rejects Invalid References ---

func TestBoundary_ForeignKeys_RejectInvalidParentReference(t *testing.T) {
	// Given — a database with foreign keys enabled (default via prepareConn).
	store, _ := setupStoreAndSvc(t)
	ctx := t.Context()

	// Build a valid issue with a parent_id referencing a nonexistent domain.
	fakeParent, err := domain.ParseID("TEST-zzzzz")
	if err != nil {
		t.Fatalf("precondition: parse ID failed: %v", err)
	}

	// When — attempt to insert the issue via a transaction.
	err = store.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		id, genErr := domain.GenerateID("TEST", nil)
		if genErr != nil {
			return genErr
		}
		iss, taskErr := domain.NewTask(domain.NewTaskParams{
			ID:       id,
			Title:    "FK violation test",
			ParentID: fakeParent,
		})
		if taskErr != nil {
			return taskErr
		}
		return uow.Issues().CreateIssue(ctx, iss)
	})

	// Then — the insert should fail due to foreign key constraint.
	if err == nil {
		t.Errorf("expected foreign key violation for invalid parent_id, got nil error")
	}
}

// --- Transaction With Cancelled Context ---

func TestBoundary_WithTransaction_CancelledContext_ReturnsError(t *testing.T) {
	// Given — a store with a pre-cancelled context.
	store, _ := setupStoreAndSvc(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately.

	// When — attempt to start a transaction with the cancelled context.
	err := store.WithTransaction(ctx, func(_ driven.UnitOfWork) error {
		t.Fatal("callback should not execute with cancelled context")
		return nil
	})

	// Then — the transaction should fail (cannot take a connection).
	if err == nil {
		t.Errorf("expected error from WithTransaction with cancelled context, got nil")
	}
}
