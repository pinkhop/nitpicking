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

	svc := core.New(store, store)
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

	svc := core.New(store, store)
	ctx := t.Context()

	// When — GetPrefix is called before Init.
	prefix, err := svc.GetPrefix(ctx)

	// Then — an error is returned; no prefix exists.
	if err == nil {
		t.Fatalf("expected error from GetPrefix on uninitialized database, got prefix %q", prefix)
	}
}

// --- IdempotencyLabel Returns Original Issue Without Mutation ---

func TestBoundary_CreateIssue_IdempotencyLabel_ReturnsOriginalWithoutMutation(t *testing.T) {
	// Given — an issue created with an idempotency label.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	idemLabel, err := domain.NewLabel("idem", "test1")
	if err != nil {
		t.Fatalf("precondition: building idempotency label: %v", err)
	}

	original, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:             domain.RoleTask,
		Title:            "Original title",
		Description:      "Original description",
		Author:           author(t, "alice"),
		IdempotencyLabel: idemLabel,
	})
	if err != nil {
		t.Fatalf("precondition: first create failed: %v", err)
	}

	// When — create is called again with the same idempotency label but
	// different title, description, and priority.
	duplicate, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:             domain.RoleTask,
		Title:            "Different title",
		Description:      "Different description",
		Priority:         domain.P0,
		Author:           author(t, "bob"),
		IdempotencyLabel: idemLabel,
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

// TestBoundary_GetSchemaVersion_NewDatabase_ReturnsThree verifies that a database
// created and initialised via the normal path reports schema version 3.
func TestBoundary_GetSchemaVersion_NewDatabase_ReturnsThree(t *testing.T) {
	// Given — a freshly created and initialised v3 database.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// When — GetSchemaVersion is read through the Doctor diagnostic path.
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then — no schema_migration_required finding, confirming version is 3.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range doctorOut.Findings {
		if f.Category == "schema_migration_required" {
			t.Errorf("unexpected schema_migration_required finding on v3 database: %s", f.Message)
		}
	}
}

// TestBoundary_GetSchemaVersion_V1Database_ReturnsZero verifies that a database
// without a schema_version key (v1 schema) reports version 0 via Doctor.
func TestBoundary_GetSchemaVersion_V1Database_ReturnsZero(t *testing.T) {
	// Given — a v1-style database: schema applied, prefix set, no schema_version key.
	store, _ := createV1Database(t)
	svc := core.New(store, store)
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

// TestBoundary_CheckSchemaVersion_V3Database_ReturnsNil verifies that
// CheckSchemaVersion succeeds on a properly initialised v3 database.
func TestBoundary_CheckSchemaVersion_V3Database_ReturnsNil(t *testing.T) {
	// Given — a freshly created and initialised v3 database.
	dbPath := t.TempDir() + "/v3.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := core.New(store, store)
	ctx := t.Context()
	if err := svc.Init(ctx, "TEST"); err != nil {
		t.Fatalf("precondition: initialising database: %v", err)
	}

	// When — CheckSchemaVersion is called.
	err = store.CheckSchemaVersion(ctx)
	// Then — no error is returned.
	if err != nil {
		t.Errorf("expected nil from CheckSchemaVersion on v3 database, got: %v", err)
	}
}

// TestBoundary_CheckSchemaVersion_V1Database_ReturnsError verifies that
// CheckSchemaVersion returns an error wrapping ErrSchemaMigrationRequired when
// the database has no schema_version key (v1 state).
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

// TestBoundary_CheckSchemaVersion_V2Database_ReturnsError verifies that
// CheckSchemaVersion returns an error wrapping ErrSchemaMigrationRequired on
// a database at schema version 2 (needs the v2→v3 migration).
func TestBoundary_CheckSchemaVersion_V2Database_ReturnsError(t *testing.T) {
	// Given — a v2-style database: schema applied, schema_version=2 set, but
	// not yet migrated to v3. We use a raw connection to insert the old
	// schema_version value without going through the normal init path.
	store, dbPath := createV1Database(t)
	ctx := context.Background()

	// Write schema_version=2 to place the database in the v2-needs-upgrade state.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if err := sqlitex.Execute(conn,
		`INSERT INTO metadata (key, value) VALUES ('schema_version', '2')`, nil); err != nil {
		t.Fatalf("precondition: setting schema_version=2: %v", err)
	}

	// When — CheckSchemaVersion is called.
	err = store.CheckSchemaVersion(ctx)

	// Then — an error is returned and it wraps ErrSchemaMigrationRequired.
	if err == nil {
		t.Fatal("expected error from CheckSchemaVersion on v2 database, got nil")
	}
	if !errors.Is(err, domain.ErrSchemaMigrationRequired) {
		t.Errorf("expected error wrapping ErrSchemaMigrationRequired, got: %T — %v", err, err)
	}
}

// --- SetSchemaVersion ---

// TestBoundary_SetSchemaVersion_WritesVersionToMetadataTable verifies that
// SetSchemaVersion inserts the schema_version key and that CheckSchemaVersion
// subsequently passes for a database that was at v1 after writing version 3.
func TestBoundary_SetSchemaVersion_WritesVersionToMetadataTable(t *testing.T) {
	// Given — a v1-style database (schema applied, prefix set, no schema_version key).
	store, _ := createV1Database(t)
	ctx := context.Background()

	// Confirm the database is v1 before the migration.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion before migration")
	}

	// When — SetSchemaVersion is called within a transaction to record v3.
	err := store.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		return uow.Database().SetSchemaVersion(ctx, 3)
	})
	// Then — no error from the transaction.
	if err != nil {
		t.Fatalf("unexpected error from SetSchemaVersion: %v", err)
	}

	// And — CheckSchemaVersion now reports success.
	if err := store.CheckSchemaVersion(ctx); err != nil {
		t.Errorf("expected CheckSchemaVersion to pass after SetSchemaVersion(3), got: %v", err)
	}
}

// --- MigrateV1ToV2 ---

// createV1DatabaseWithClaimedIssues creates a v1-style database with:
//   - Two issues with state='claimed' (simulating v1 claimed lifecycle state)
//   - One issue with state='open'
//   - History rows with event_type='claimed' and 'released'
//
// Returns the store and the database path. The caller must close the store.
func createV1DatabaseWithClaimedIssues(t *testing.T) (*sqlite.Store, string) {
	t.Helper()

	dbPath := t.TempDir() + "/v1-claimed.db"

	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Use raw SQLite to insert v1 state, bypassing domain constructors.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	now := "2024-01-01T00:00:00Z"

	// Insert the prefix and skip schema_version to simulate v1 state.
	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('prefix', 'TEST')`, nil); err != nil {
		t.Fatalf("precondition: inserting prefix: %v", err)
	}

	// Two issues with state='claimed' (v1 lifecycle state).
	for _, id := range []string{"TEST-aaaaa", "TEST-bbbbb"} {
		err = sqlitex.Execute(conn,
			`INSERT INTO issues (issue_id, role, title, state, created_at) VALUES (?, 'task', 'Claimed task', 'claimed', ?)`,
			&sqlitex.ExecOptions{Args: []any{id, now}})
		if err != nil {
			t.Fatalf("precondition: inserting claimed issue %s: %v", id, err)
		}
	}

	// One issue with state='open'.
	err = sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at) VALUES ('TEST-ccccc', 'task', 'Open task', 'open', ?)`,
		&sqlitex.ExecOptions{Args: []any{now}})
	if err != nil {
		t.Fatalf("precondition: inserting open issue: %v", err)
	}

	// History rows: 'claimed' and 'released' event types (removed in v2).
	for _, row := range []struct {
		issueID   string
		eventType string
	}{
		{"TEST-aaaaa", "claimed"},
		{"TEST-aaaaa", "released"},
		{"TEST-bbbbb", "claimed"},
		{"TEST-ccccc", "created"},
	} {
		err = sqlitex.Execute(conn,
			`INSERT INTO history (issue_id, revision, author, timestamp, event_type, changes) VALUES (?, 0, 'tester', ?, ?, '[]')`,
			&sqlitex.ExecOptions{Args: []any{row.issueID, now, row.eventType}})
		if err != nil {
			t.Fatalf("precondition: inserting history row %s/%s: %v", row.issueID, row.eventType, err)
		}
	}

	return store, dbPath
}

// TestBoundary_MigrateV1ToV2_V1WithClaimedIssues_MigratesAtomically verifies
// that MigrateV1ToV2 converts claimed issues to open, removes obsolete history
// rows, and marks the database as v2 (which requires a further v2→v3 migration
// before CheckSchemaVersion will report success).
func TestBoundary_MigrateV1ToV2_V1WithClaimedIssues_MigratesAtomically(t *testing.T) {
	// Given — a v1 database with two claimed issues and three obsolete history rows.
	store, dbPath := createV1DatabaseWithClaimedIssues(t)
	ctx := context.Background()

	// Confirm the database is v1 before migration.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion before migration")
	}

	// When — MigrateV1ToV2 is called.
	result, err := store.MigrateV1ToV2(ctx)
	// Then — no error is returned.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV1ToV2: %v", err)
	}

	// The result counts reflect the rows affected.
	if result.ClaimedIssuesConverted != 2 {
		t.Errorf("ClaimedIssuesConverted: got %d, want 2", result.ClaimedIssuesConverted)
	}
	if result.HistoryRowsRemoved != 3 {
		t.Errorf("HistoryRowsRemoved: got %d, want 3", result.HistoryRowsRemoved)
	}

	// Confirm the migration wrote schema_version=2 by querying the raw database.
	// CheckSchemaVersion is not used here because v2 is no longer the current
	// version — the database still needs the v2→v3 migration.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening raw connection for verification: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var schemaVersion int
	if err := sqlitex.Execute(conn, `SELECT value FROM metadata WHERE key = 'schema_version'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			schemaVersion = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("reading schema_version after migration: %v", err)
	}
	if schemaVersion != 2 {
		t.Errorf("expected schema_version=2 after MigrateV1ToV2, got %d", schemaVersion)
	}

	var claimedCount int
	if err := sqlitex.Execute(conn, `SELECT COUNT(*) FROM issues WHERE state = 'claimed'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			claimedCount = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("counting claimed issues after migration: %v", err)
	}
	if claimedCount != 0 {
		t.Errorf("expected 0 claimed issues after migration, got %d", claimedCount)
	}

	var obsoleteHistoryCount int
	if err := sqlitex.Execute(conn, `SELECT COUNT(*) FROM history WHERE event_type IN ('claimed', 'released')`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			obsoleteHistoryCount = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("counting obsolete history rows after migration: %v", err)
	}
	if obsoleteHistoryCount != 0 {
		t.Errorf("expected 0 obsolete history rows after migration, got %d", obsoleteHistoryCount)
	}
}

// TestBoundary_MigrateV1ToV2_V1NoClaimedIssues_SetsVersionWithZeroCounts
// verifies that MigrateV1ToV2 succeeds on a v1 database with no claimed issues,
// reporting zero for both conversion counts, and writes schema_version=2.
func TestBoundary_MigrateV1ToV2_V1NoClaimedIssues_SetsVersionWithZeroCounts(t *testing.T) {
	// Given — a v1-style database with no claimed issues.
	store, dbPath := createV1Database(t)
	ctx := context.Background()

	// Confirm v1 state.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion before migration")
	}

	// When — MigrateV1ToV2 is called on a database with no claimed issues.
	result, err := store.MigrateV1ToV2(ctx)
	// Then — no error; counts are both zero.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV1ToV2: %v", err)
	}
	if result.ClaimedIssuesConverted != 0 {
		t.Errorf("ClaimedIssuesConverted: got %d, want 0", result.ClaimedIssuesConverted)
	}
	if result.HistoryRowsRemoved != 0 {
		t.Errorf("HistoryRowsRemoved: got %d, want 0", result.HistoryRowsRemoved)
	}

	// Confirm schema_version=2 was written. CheckSchemaVersion is not used here
	// because v2 is no longer the current version — a further v2→v3 migration is
	// still required.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening raw connection for version check: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var schemaVersion int
	if err := sqlitex.Execute(conn, `SELECT value FROM metadata WHERE key = 'schema_version'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			schemaVersion = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("reading schema_version after migration: %v", err)
	}
	if schemaVersion != 2 {
		t.Errorf("expected schema_version=2 after MigrateV1ToV2, got %d", schemaVersion)
	}
}

// TestBoundary_MigrateV1ToV2_LegacyRelTypes_TranslatedAndDropped verifies that
// MigrateV1ToV2 translates legacy v0.2.0 relationship types stored in the on-disk
// database: "cites" rows are renamed to "refs" and "cited_by" rows are deleted.
// The test inserts the legacy rows via a raw connection with CHECK constraints
// disabled (PRAGMA ignore_check_constraints = ON) to simulate what a v0.2.0
// database would contain, then runs the migration and asserts the expected
// post-migration state.
func TestBoundary_MigrateV1ToV2_LegacyRelTypes_TranslatedAndDropped(t *testing.T) {
	// Given — a v1-style database containing "cites" and "cited_by" relationship
	// rows alongside a "blocked_by" row that must be preserved unchanged.
	store, dbPath := createV1DatabaseWithLegacyRelTypes(t)
	ctx := context.Background()

	// Confirm v1 state before migration.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion before migration")
	}

	// When — MigrateV1ToV2 is called.
	result, err := store.MigrateV1ToV2(ctx)
	// Then — no error and the legacy relationship count is reported.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV1ToV2: %v", err)
	}
	// Two rows are affected: one "cites" translated to "refs" and one "cited_by" dropped.
	if result.LegacyRelationshipsTranslated != 2 {
		t.Errorf("LegacyRelationshipsTranslated: got %d, want 2", result.LegacyRelationshipsTranslated)
	}

	// Confirm the on-disk state via a raw query.
	// Note: CheckSchemaVersion is not called here because v2 is no longer the
	// current version — the database still requires the v2→v3 migration.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening raw connection for verification: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// "cites" must no longer exist.
	var citesCount int
	if err := sqlitex.Execute(conn, `SELECT COUNT(*) FROM relationships WHERE rel_type = 'cites'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			citesCount = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("counting cites rows after migration: %v", err)
	}
	if citesCount != 0 {
		t.Errorf("expected 0 'cites' rows after migration, got %d", citesCount)
	}

	// "cited_by" must no longer exist.
	var citedByCount int
	if err := sqlitex.Execute(conn, `SELECT COUNT(*) FROM relationships WHERE rel_type = 'cited_by'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *zombiezen.Stmt) error {
			citedByCount = stmt.ColumnInt(0)
			return nil
		},
	}); err != nil {
		t.Fatalf("counting cited_by rows after migration: %v", err)
	}
	if citedByCount != 0 {
		t.Errorf("expected 0 'cited_by' rows after migration, got %d", citedByCount)
	}

	// The "cites(A,B)" row must now be a "refs(A,B)" row.
	var refsCount int
	if err := sqlitex.Execute(conn,
		`SELECT COUNT(*) FROM relationships WHERE rel_type = 'refs' AND source_id = 'LEG-aaaaa' AND target_id = 'LEG-bbbbb'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				refsCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("counting translated refs rows after migration: %v", err)
	}
	if refsCount != 1 {
		t.Errorf("expected 1 translated 'refs' row (LEG-aaaaa→LEG-bbbbb), got %d", refsCount)
	}

	// The original "blocked_by" row must be untouched.
	var blockedByCount int
	if err := sqlitex.Execute(conn,
		`SELECT COUNT(*) FROM relationships WHERE rel_type = 'blocked_by'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				blockedByCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("counting blocked_by rows after migration: %v", err)
	}
	if blockedByCount != 1 {
		t.Errorf("expected 1 preserved 'blocked_by' row, got %d", blockedByCount)
	}
}

// createV1DatabaseWithLegacyRelTypes creates a v1-style database containing
// relationship rows with legacy v0.2.0 rel_type values ("cites" and "cited_by")
// as well as a modern "blocked_by" row. CHECK constraints are temporarily
// disabled so the legacy values can be inserted into the narrowed schema.
// The store and database path are returned; the store is closed via t.Cleanup.
func createV1DatabaseWithLegacyRelTypes(t *testing.T) (*sqlite.Store, string) {
	t.Helper()

	dbPath := t.TempDir() + "/v1-legacy-rels.db"

	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}
	defer func() { _ = conn.Close() }()

	now := "2024-01-01T00:00:00Z"

	// Insert the prefix and skip schema_version to reproduce v1 state.
	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('prefix', 'LEG')`, nil); err != nil {
		t.Fatalf("precondition: inserting prefix: %v", err)
	}

	// Insert three issues that will be related.
	for _, row := range []struct {
		id    string
		title string
	}{
		{"LEG-aaaaa", "Task A (citer)"},
		{"LEG-bbbbb", "Task B (cited)"},
		{"LEG-ccccc", "Task C (blocker)"},
	} {
		if err := sqlitex.Execute(conn,
			`INSERT INTO issues (issue_id, role, title, state, created_at) VALUES (?, 'task', ?, 'open', ?)`,
			&sqlitex.ExecOptions{Args: []any{row.id, row.title, now}}); err != nil {
			t.Fatalf("precondition: inserting issue %s: %v", row.id, err)
		}
	}

	// Temporarily disable CHECK constraints so we can insert legacy rel_type values.
	if err := sqlitex.Execute(conn, `PRAGMA ignore_check_constraints = ON`, nil); err != nil {
		t.Fatalf("precondition: disabling check constraints: %v", err)
	}

	// Insert a "cites" row (legacy; should be translated to "refs").
	if err := sqlitex.Execute(conn,
		`INSERT INTO relationships (source_id, target_id, rel_type) VALUES ('LEG-aaaaa', 'LEG-bbbbb', 'cites')`,
		nil); err != nil {
		t.Fatalf("precondition: inserting cites relationship: %v", err)
	}

	// Insert a "cited_by" row (legacy; should be dropped during migration).
	if err := sqlitex.Execute(conn,
		`INSERT INTO relationships (source_id, target_id, rel_type) VALUES ('LEG-bbbbb', 'LEG-aaaaa', 'cited_by')`,
		nil); err != nil {
		t.Fatalf("precondition: inserting cited_by relationship: %v", err)
	}

	// Insert a modern "blocked_by" row (should be preserved unchanged).
	if err := sqlitex.Execute(conn,
		`INSERT INTO relationships (source_id, target_id, rel_type) VALUES ('LEG-ccccc', 'LEG-aaaaa', 'blocked_by')`,
		nil); err != nil {
		t.Fatalf("precondition: inserting blocked_by relationship: %v", err)
	}

	// Re-enable CHECK constraints.
	if err := sqlitex.Execute(conn, `PRAGMA ignore_check_constraints = OFF`, nil); err != nil {
		t.Fatalf("precondition: re-enabling check constraints: %v", err)
	}

	return store, dbPath
}

// TestBoundary_MigrateV1ToV2_V2DatabaseSeedData_ReturnsNilWithZeroCounts verifies
// that calling MigrateV1ToV2 on a database that already has schema_version=2 is
// safe — it applies no changes and returns zero counts. The caller should check
// CheckSchemaVersion first to avoid unnecessary work, but the migration is idempotent
// with respect to its own data operations.
//
// Note: a freshly-initialised database is now at v3. This test constructs a
// v2-state database explicitly by setting schema_version=2 after creation so
// that it has the idempotency_key column the migration expects.
func TestBoundary_MigrateV1ToV2_V2DatabaseSeedData_ReturnsNilWithZeroCounts(t *testing.T) {
	// Given — a v1-style database promoted to schema_version=2 via raw SQL,
	// but with no claimed issues or legacy history rows, so conversion counts are zero.
	store, dbPath := createV1Database(t)
	ctx := context.Background()

	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}
	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('schema_version', '2')`, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: setting schema_version=2: %v", err)
	}
	_ = conn.Close()

	// When — MigrateV1ToV2 is called on a v2 database with no claimed issues.
	result, err := store.MigrateV1ToV2(ctx)
	// Then — no error; counts are zero (nothing to convert).
	if err != nil {
		t.Fatalf("unexpected error from MigrateV1ToV2 on v2 database: %v", err)
	}
	if result.ClaimedIssuesConverted != 0 {
		t.Errorf("ClaimedIssuesConverted: got %d, want 0", result.ClaimedIssuesConverted)
	}
	if result.HistoryRowsRemoved != 0 {
		t.Errorf("HistoryRowsRemoved: got %d, want 0", result.HistoryRowsRemoved)
	}
}
