//go:build boundary

package sqlite_test

import (
	"context"
	"errors"
	"testing"

	zombiezen "zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// createV1Database creates a SQLite database with the v2 schema applied and a
// prefix set, but no schema_version key in the metadata table — simulating a
// database created before schema versioning was introduced. The Store is
// returned; callers must close it via t.Cleanup.
func createV1Database(t *testing.T) (*sqlite.Store, string) {
	t.Helper()

	dbPath := t.TempDir() + "/v1.db"

	// Create the database and apply the schema.
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Insert the prefix directly and skip schema_version, reproducing the
	// v1 state where the metadata table exists but carries no schema_version key.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('prefix', 'V1')`, nil); err != nil {
		t.Fatalf("precondition: inserting prefix: %v", err)
	}

	return store, dbPath
}

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

// --- GetSchemaVersion ---

// TestBoundary_GetSchemaVersion_NewDatabase_ReturnsTwo verifies that a database
// created and initialised via the normal path reports schema version 2.
func TestBoundary_GetSchemaVersion_NewDatabase_ReturnsTwo(t *testing.T) {
	// Given — a freshly created and initialised v2 database.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// When — GetSchemaVersion is read through the Doctor diagnostic path.
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — no schema_migration_required finding, confirming version is 2.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range doctorOut.Findings {
		if f.Category == "schema_migration_required" {
			t.Errorf("unexpected schema_migration_required finding on v2 database: %s", f.Message)
		}
	}
}

// TestBoundary_GetSchemaVersion_V1Database_ReturnsZero verifies that a database
// without a schema_version key (v1 schema) reports version 0 via Doctor.
func TestBoundary_GetSchemaVersion_V1Database_ReturnsZero(t *testing.T) {
	// Given — a v1-style database: schema applied, prefix set, no schema_version key.
	store, _ := createV1Database(t)
	svc := core.New(store)
	ctx := t.Context()

	// When — Doctor runs (reads schema version without requiring v2).
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — a schema_migration_required finding is present, confirming the
	// version was read as 0.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, f := range doctorOut.Findings {
		if f.Category == "schema_migration_required" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected schema_migration_required finding for v1 database, got none")
	}
}

// --- CheckSchemaVersion ---

// TestBoundary_CheckSchemaVersion_V2Database_ReturnsNil verifies that
// CheckSchemaVersion succeeds on a properly initialised v2 database.
func TestBoundary_CheckSchemaVersion_V2Database_ReturnsNil(t *testing.T) {
	// Given — a freshly created and initialised v2 database.
	dbPath := t.TempDir() + "/v2.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := core.New(store)
	ctx := t.Context()
	if err := svc.Init(ctx, "TEST"); err != nil {
		t.Fatalf("precondition: initialising database: %v", err)
	}

	// When — CheckSchemaVersion is called.
	err = store.CheckSchemaVersion(ctx)
	// Then — no error is returned.
	if err != nil {
		t.Errorf("expected nil from CheckSchemaVersion on v2 database, got: %v", err)
	}
}

// TestBoundary_CheckSchemaVersion_V1Database_ReturnsError verifies that
// CheckSchemaVersion returns an error wrapping ErrSchemaMigrationRequired when
// the database has no schema_version key.
func TestBoundary_CheckSchemaVersion_V1Database_ReturnsError(t *testing.T) {
	// Given — a v1-style database without a schema_version key.
	store, _ := createV1Database(t)
	ctx := context.Background()

	// When — CheckSchemaVersion is called.
	err := store.CheckSchemaVersion(ctx)

	// Then — an error is returned and it wraps ErrSchemaMigrationRequired.
	if err == nil {
		t.Fatal("expected error from CheckSchemaVersion on v1 database, got nil")
	}
	if !errors.Is(err, domain.ErrSchemaMigrationRequired) {
		t.Errorf("expected error wrapping ErrSchemaMigrationRequired, got: %T — %v", err, err)
	}
}

// --- SetSchemaVersion ---

// TestBoundary_SetSchemaVersion_WritesVersionToMetadataTable verifies that
// SetSchemaVersion inserts the schema_version key and that CheckSchemaVersion
// subsequently passes for a database that started at v1.
func TestBoundary_SetSchemaVersion_WritesVersionToMetadataTable(t *testing.T) {
	// Given — a v1-style database (schema applied, prefix set, no schema_version key).
	store, _ := createV1Database(t)
	ctx := context.Background()

	// Confirm the database is v1 before the migration.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion before migration")
	}

	// When — SetSchemaVersion is called within a transaction to record v2.
	err := store.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		return uow.Database().SetSchemaVersion(ctx, 2)
	})
	// Then — no error from the transaction.
	if err != nil {
		t.Fatalf("unexpected error from SetSchemaVersion: %v", err)
	}

	// And — CheckSchemaVersion now reports success.
	if err := store.CheckSchemaVersion(ctx); err != nil {
		t.Errorf("expected CheckSchemaVersion to pass after SetSchemaVersion(2), got: %v", err)
	}
}
