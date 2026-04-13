//go:build boundary

package sqlite_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// setupStoreAndSvc creates a fresh SQLite database and returns both the Store
// (for direct transaction tests) and a Service (for convenient issue creation).
func setupStoreAndSvc(t *testing.T) (*sqlite.Store, driving.Service) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store, store)
	if err := svc.Init(t.Context(), "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return store, svc
}

// setupStoreAndSvcAtPath is like setupStoreAndSvc but also returns the DB path
// so additional Store instances can be opened against the same file.
func setupStoreAndSvcAtPath(t *testing.T) (string, *sqlite.Store, driving.Service) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("creating database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store, store)
	if err := svc.Init(t.Context(), "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return dbPath, store, svc
}

// --- WithTransaction Commits on Success ---

func TestBoundary_WithTransaction_CommitsOnSuccess(t *testing.T) {
	// Given — create an issue via the service, capturing its ID
	store, svc := setupStoreAndSvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Committed task", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When — read the issue inside a new transaction (verifying the create committed)
	var foundTitle string
	err = store.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		iss, getErr := uow.Issues().GetIssue(ctx, createOut.Issue.ID(), false)
		if getErr != nil {
			return getErr
		}
		foundTitle = iss.Title()
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundTitle != "Committed task" {
		t.Errorf("title: got %q, want %q", foundTitle, "Committed task")
	}
}

// --- WithTransaction Rolls Back on Error ---

func TestBoundary_WithTransaction_RollsBackOnError(t *testing.T) {
	// Given
	store, svc := setupStoreAndSvc(t)
	ctx := t.Context()

	sentinel := errors.New("deliberate rollback")

	// First, create a valid issue so we know the DB works.
	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Baseline task", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When — attempt an update inside a transaction that returns an error
	err = store.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		iss, getErr := uow.Issues().GetIssue(ctx, createOut.Issue.ID(), false)
		if getErr != nil {
			return getErr
		}
		updated, titleErr := iss.WithTitle("Should not persist")
		if titleErr != nil {
			return titleErr
		}
		if updateErr := uow.Issues().UpdateIssue(ctx, updated); updateErr != nil {
			return updateErr
		}
		return sentinel
	})
	// Then — error propagated and title is unchanged
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}

	var title string
	_ = store.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		iss, getErr := uow.Issues().GetIssue(ctx, createOut.Issue.ID(), false)
		if getErr != nil {
			return getErr
		}
		title = iss.Title()
		return nil
	})
	if title != "Baseline task" {
		t.Errorf("title should be unchanged after rollback: got %q, want %q", title, "Baseline task")
	}
}

// --- WithReadTransaction Allows Reads ---

func TestBoundary_WithReadTransaction_CanReadData(t *testing.T) {
	// Given — create an issue first
	store, svc := setupStoreAndSvc(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Readable task", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}

	// When — read the issue inside a read-only transaction
	var readTitle string
	err = store.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		iss, getErr := uow.Issues().GetIssue(ctx, createOut.Issue.ID(), false)
		if getErr != nil {
			return getErr
		}
		readTitle = iss.Title()
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readTitle != "Readable task" {
		t.Errorf("title: got %q, want %q", readTitle, "Readable task")
	}
}

// --- Concurrent Read Transactions Do Not Block ---

func TestBoundary_ConcurrentReads_DoNotBlock(t *testing.T) {
	// Given — create a store and seed an issue; open a second Store to the same DB
	dbPath, store, svc := setupStoreAndSvcAtPath(t)
	ctx := t.Context()

	createOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Concurrent read target", Author: author(t, "alice"),
	})
	if err != nil {
		t.Fatalf("precondition: create failed: %v", err)
	}
	issueID := createOut.Issue.ID()

	store2, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("opening second store: %v", err)
	}
	defer store2.Close()

	// When — two goroutines read concurrently from separate stores
	var wg sync.WaitGroup
	errs := make([]error, 2)

	for i, s := range []*sqlite.Store{store, store2} {
		wg.Go(func() {
			errs[i] = s.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
				_, getErr := uow.Issues().GetIssue(ctx, issueID, false)
				return getErr
			})
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Then — both reads completed without blocking
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent reads blocked for more than 5 seconds")
	}

	for i, e := range errs {
		if e != nil {
			t.Errorf("reader %d: unexpected error: %v", i, e)
		}
	}
}

// --- Concurrent Write Transactions Serialize Correctly ---

func TestBoundary_ConcurrentWrites_SerializeCorrectly(t *testing.T) {
	// Given — two stores pointing at the same database
	dbPath, _, svc := setupStoreAndSvcAtPath(t)
	ctx := t.Context()

	// Open two separate stores for concurrent writes.
	writerStore1, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("opening writer store 1: %v", err)
	}
	defer writerStore1.Close()

	writerStore2, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("opening writer store 2: %v", err)
	}
	defer writerStore2.Close()

	// When — two goroutines each create a different issue concurrently via service
	var wg sync.WaitGroup
	createErrs := make([]error, 2)
	issueIDs := make([]domain.ID, 2)

	writerAuthor := author(t, "writer")
	for i, title := range []string{"Concurrent write A", "Concurrent write B"} {
		wg.Go(func() {
			out, createErr := svc.CreateIssue(ctx, driving.CreateIssueInput{
				Role: domain.RoleTask, Title: title, Author: writerAuthor,
			})
			createErrs[i] = createErr
			if createErr == nil {
				issueIDs[i] = out.Issue.ID()
			}
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Then — both writes completed (serialized by SQLite WAL)
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent writes blocked for more than 10 seconds")
	}

	for i, e := range createErrs {
		if e != nil {
			t.Errorf("writer %d: unexpected error: %v", i, e)
		}
	}

	// Verify both issues exist.
	for i, id := range issueIDs {
		if id.IsZero() {
			continue
		}
		showOut, showErr := svc.ShowIssue(ctx, id.String())
		if showErr != nil {
			t.Errorf("writer %d issue %s: not found: %v", i, id, showErr)
		} else if showOut.State != domain.StateOpen {
			t.Errorf("writer %d issue %s: unexpected state %v", i, id, showOut.State)
		}
	}
}

// --- UnitOfWork Exposes All Repository Interfaces ---

func TestBoundary_UnitOfWork_ExposesAllRepositories(t *testing.T) {
	// Given
	store, _ := setupStoreAndSvc(t)
	ctx := t.Context()

	// When — access every repository accessor within a transaction
	var (
		hasIssues        bool
		hasComments      bool
		hasClaims        bool
		hasRelationships bool
		hasHistory       bool
		hasDatabase      bool
	)
	err := store.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		hasIssues = uow.Issues() != nil
		hasComments = uow.Comments() != nil
		hasClaims = uow.Claims() != nil
		hasRelationships = uow.Relationships() != nil
		hasHistory = uow.History() != nil
		hasDatabase = uow.Database() != nil
		return nil
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasIssues {
		t.Error("Issues() returned nil")
	}
	if !hasComments {
		t.Error("Comments() returned nil")
	}
	if !hasClaims {
		t.Error("Claims() returned nil")
	}
	if !hasRelationships {
		t.Error("Relationships() returned nil")
	}
	if !hasHistory {
		t.Error("History() returned nil")
	}
	if !hasDatabase {
		t.Error("Database() returned nil")
	}
}
