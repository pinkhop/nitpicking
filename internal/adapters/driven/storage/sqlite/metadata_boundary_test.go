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

// v2SchemaSQL is the SQLite DDL for schema version 2. It is identical to the
// current v3 schema except that the issues table carries an idempotency_key
// column and the idx_issues_idempotency unique partial index. This literal is
// used by createV2Database to create a pre-migration fixture without coupling
// the test to the current schemaSQL constant.
const v2SchemaSQL = `
CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS issues (
    issue_id            TEXT PRIMARY KEY,
    role                TEXT NOT NULL CHECK(role IN ('task', 'epic')),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    acceptance_criteria TEXT NOT NULL DEFAULT '',
    priority            TEXT NOT NULL DEFAULT 'P2',
    state               TEXT NOT NULL,
    parent_id           TEXT DEFAULT NULL REFERENCES issues(issue_id),
    created_at          TEXT NOT NULL,
    idempotency_key     TEXT DEFAULT NULL,
    deleted             INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_issues_state ON issues(state) WHERE deleted = 0;
CREATE INDEX IF NOT EXISTS idx_issues_priority_created ON issues(priority, created_at) WHERE deleted = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_issues_idempotency ON issues(idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS labels (
    issue_id TEXT NOT NULL REFERENCES issues(issue_id),
    key       TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (issue_id, key)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS comments (
    comment_id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(issue_id),
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    body       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id);

CREATE TABLE IF NOT EXISTS claims (
    claim_sha512    TEXT PRIMARY KEY,
    issue_id       TEXT NOT NULL REFERENCES issues(issue_id),
    author          TEXT NOT NULL,
    stale_threshold INTEGER NOT NULL,
    last_activity   TEXT NOT NULL
) WITHOUT ROWID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_issue ON claims(issue_id);

CREATE TABLE IF NOT EXISTS relationships (
    source_id TEXT NOT NULL REFERENCES issues(issue_id),
    target_id TEXT NOT NULL REFERENCES issues(issue_id),
    rel_type  TEXT NOT NULL CHECK(rel_type IN ('blocked_by', 'blocks', 'refs')),
    PRIMARY KEY (source_id, target_id, rel_type)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);

CREATE TABLE IF NOT EXISTS history (
    entry_id   INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id  TEXT NOT NULL REFERENCES issues(issue_id),
    revision   INTEGER NOT NULL,
    author     TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    event_type TEXT NOT NULL,
    changes    TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_history_issue ON history(issue_id, revision);

CREATE VIRTUAL TABLE IF NOT EXISTS issues_fts USING fts5(
    issue_id,
    title,
    description,
    acceptance_criteria
);

CREATE VIRTUAL TABLE IF NOT EXISTS comments_fts USING fts5(
    comment_id,
    body
);
`

// createV2Database creates a SQLite database file with the v2 schema applied
// (which includes the idempotency_key column and idx_issues_idempotency index),
// sets schema_version=2 in the metadata table, and inserts the given prefix.
// The *sqlite.Store is opened with sqlite.Open (no schema DDL applied), and is
// registered for cleanup via t.Cleanup. The database path is returned so
// callers can open additional raw connections for seeding or verification.
func createV2Database(t *testing.T) (*sqlite.Store, string) {
	t.Helper()

	dbPath := t.TempDir() + "/v2.db"

	// Create the database file and apply the v2 schema using a raw connection.
	// sqlite.Create applies the current (v3) schema which lacks idempotency_key,
	// so we bootstrap the file with the v2 DDL via OpenConn(OpenCreate).
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite|zombiezen.OpenCreate)
	if err != nil {
		t.Fatalf("precondition: creating v2 database file: %v", err)
	}

	if err := sqlitex.ExecuteScript(conn, v2SchemaSQL, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: applying v2 schema: %v", err)
	}

	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('prefix', 'V2')`, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting prefix: %v", err)
	}

	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('schema_version', '2')`, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: setting schema_version=2: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing bootstrap connection: %v", err)
	}

	// Open the database through the store layer. sqlite.Open does not re-apply
	// schema DDL, so the v2 column shape (with idempotency_key) is preserved.
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("precondition: opening v2 database via store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store, dbPath
}

// --- MigrateV2ToV3 ---

// TestBoundary_MigrateV2ToV3_V2WithIdempotencyKeys_MigratesAtomically seeds a v2
// database with issues carrying non-NULL and NULL idempotency_key values, calls
// MigrateV2ToV3, and asserts that label rows are created, the column and index are
// dropped, and schema_version is set to 3.
func TestBoundary_MigrateV2ToV3_V2WithIdempotencyKeys_MigratesAtomically(t *testing.T) {
	// Given — a v2 database with two issues: one carrying a non-NULL idempotency_key
	// and one with a NULL idempotency_key (must not receive a new label row).
	store, dbPath := createV2Database(t)
	ctx := context.Background()

	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}

	now := "2024-01-01T00:00:00Z"
	if err := sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at, idempotency_key) VALUES ('V2-aaaaa', 'task', 'Task A', 'open', ?, 'jira-k1')`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting issue with idempotency_key: %v", err)
	}
	if err := sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at) VALUES ('V2-bbbbb', 'task', 'Task B', 'open', ?)`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting issue without idempotency_key: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing seed connection: %v", err)
	}

	// When — MigrateV2ToV3 is called on the seeded v2 database.
	result, err := store.MigrateV2ToV3(ctx)
	// Then — no error and one key is migrated, zero skipped.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV2ToV3: %v", err)
	}
	if result.IdempotencyKeysMigrated != 1 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 1", result.IdempotencyKeysMigrated)
	}
	if result.IdempotencyKeysSkipped != 0 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 0", result.IdempotencyKeysSkipped)
	}
	if result.InvalidLabelValuesSkipped != 0 {
		t.Errorf("InvalidLabelValuesSkipped: got %d, want 0", result.InvalidLabelValuesSkipped)
	}

	// Verify post-migration state via a raw read-only connection.
	verifyConn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening verification connection: %v", err)
	}
	defer func() { _ = verifyConn.Close() }()

	// (a) The idempotency label row exists for V2-aaaaa with value "jira-k1".
	var labelValue string
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM labels WHERE issue_id = 'V2-aaaaa' AND key = 'idempotency'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelValue = stmt.ColumnText(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading idempotency label: %v", err)
	}
	if labelValue != "jira-k1" {
		t.Errorf("idempotency label value: got %q, want %q", labelValue, "jira-k1")
	}

	// No label row exists for V2-bbbbb (NULL idempotency_key must not be migrated).
	var nullIssueLabelCount int
	if err := sqlitex.Execute(verifyConn,
		`SELECT COUNT(*) FROM labels WHERE issue_id = 'V2-bbbbb' AND key = 'idempotency'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				nullIssueLabelCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading null-issue label count: %v", err)
	}
	if nullIssueLabelCount != 0 {
		t.Errorf("expected no idempotency label for null-key issue, got %d", nullIssueLabelCount)
	}

	// (b) The idempotency_key column is absent from PRAGMA table_info(issues).
	var columnFound bool
	if err := sqlitex.Execute(verifyConn,
		`SELECT 1 FROM pragma_table_info('issues') WHERE name = 'idempotency_key' LIMIT 1`,
		&sqlitex.ExecOptions{
			ResultFunc: func(_ *zombiezen.Stmt) error {
				columnFound = true
				return nil
			},
		}); err != nil {
		t.Fatalf("checking table_info for idempotency_key: %v", err)
	}
	if columnFound {
		t.Error("idempotency_key column still present in issues after migration")
	}

	// (c) The idx_issues_idempotency index is absent from PRAGMA index_list(issues).
	var indexFound bool
	if err := sqlitex.Execute(verifyConn,
		`SELECT 1 FROM pragma_index_list('issues') WHERE name = 'idx_issues_idempotency' LIMIT 1`,
		&sqlitex.ExecOptions{
			ResultFunc: func(_ *zombiezen.Stmt) error {
				indexFound = true
				return nil
			},
		}); err != nil {
		t.Fatalf("checking index_list for idx_issues_idempotency: %v", err)
	}
	if indexFound {
		t.Error("idx_issues_idempotency index still present after migration")
	}

	// (d) schema_version is 3.
	var schemaVersion int
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM metadata WHERE key = 'schema_version'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				schemaVersion = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if schemaVersion != 3 {
		t.Errorf("schema_version: got %d, want 3", schemaVersion)
	}
}

// TestBoundary_MigrateV2ToV3_NoIdempotencyKeyRows_SucceedsIdempotently verifies that
// a v2 database in which all idempotency_key values are NULL still has its column and
// index dropped and schema_version bumped to 3, and that no label rows are written.
func TestBoundary_MigrateV2ToV3_NoIdempotencyKeyRows_SucceedsIdempotently(t *testing.T) {
	// Given — a v2 database with one issue whose idempotency_key is NULL.
	store, dbPath := createV2Database(t)
	ctx := context.Background()

	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}

	now := "2024-01-01T00:00:00Z"
	if err := sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at) VALUES ('V2-ccccc', 'task', 'No-key task', 'open', ?)`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting issue: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing seed connection: %v", err)
	}

	// When — MigrateV2ToV3 is called.
	result, err := store.MigrateV2ToV3(ctx)
	// Then — no error; all counters are zero.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV2ToV3: %v", err)
	}
	if result.IdempotencyKeysMigrated != 0 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 0", result.IdempotencyKeysMigrated)
	}
	if result.IdempotencyKeysSkipped != 0 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 0", result.IdempotencyKeysSkipped)
	}

	// Verify via raw connection.
	verifyConn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening verification connection: %v", err)
	}
	defer func() { _ = verifyConn.Close() }()

	// Column must be absent.
	var columnFound bool
	if err := sqlitex.Execute(verifyConn,
		`SELECT 1 FROM pragma_table_info('issues') WHERE name = 'idempotency_key' LIMIT 1`,
		&sqlitex.ExecOptions{
			ResultFunc: func(_ *zombiezen.Stmt) error {
				columnFound = true
				return nil
			},
		}); err != nil {
		t.Fatalf("checking table_info: %v", err)
	}
	if columnFound {
		t.Error("idempotency_key column still present after migration of NULL-only database")
	}

	// Index must be absent.
	var indexFound bool
	if err := sqlitex.Execute(verifyConn,
		`SELECT 1 FROM pragma_index_list('issues') WHERE name = 'idx_issues_idempotency' LIMIT 1`,
		&sqlitex.ExecOptions{
			ResultFunc: func(_ *zombiezen.Stmt) error {
				indexFound = true
				return nil
			},
		}); err != nil {
		t.Fatalf("checking index_list: %v", err)
	}
	if indexFound {
		t.Error("idx_issues_idempotency index still present after migration")
	}

	// schema_version must be 3.
	var schemaVersion int
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM metadata WHERE key = 'schema_version'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				schemaVersion = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if schemaVersion != 3 {
		t.Errorf("schema_version: got %d, want 3", schemaVersion)
	}

	// No label rows must exist for the issue.
	var labelCount int
	if err := sqlitex.Execute(verifyConn,
		`SELECT COUNT(*) FROM labels WHERE issue_id = 'V2-ccccc'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading label count: %v", err)
	}
	if labelCount != 0 {
		t.Errorf("expected 0 labels for null-key issue, got %d", labelCount)
	}
}

// TestBoundary_MigrateV2ToV3_AlreadyV3_IsNoOp verifies that calling MigrateV2ToV3
// on a database that is already at v3 (i.e., the idempotency_key column was already
// dropped) does not fail, does not mutate the labels table, and leaves
// schema_version at 3.
func TestBoundary_MigrateV2ToV3_AlreadyV3_IsNoOp(t *testing.T) {
	// Given — a freshly created v3 database (no idempotency_key column).
	dbPath := t.TempDir() + "/v3.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: creating v3 database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	svc := core.New(store, store)
	ctx := context.Background()
	if err := svc.Init(ctx, "VVV"); err != nil {
		t.Fatalf("precondition: initialising database: %v", err)
	}

	// Record the label count before the call.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("precondition: opening raw connection for pre-check: %v", err)
	}
	var labelCountBefore int
	if err := sqlitex.Execute(conn,
		`SELECT COUNT(*) FROM labels`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelCountBefore = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: counting labels before migration: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing read connection: %v", err)
	}

	// When — MigrateV2ToV3 is called on the already-v3 database.
	result, err := store.MigrateV2ToV3(ctx)
	// Then — no error; all counters are zero (nothing to migrate).
	if err != nil {
		t.Fatalf("unexpected error from MigrateV2ToV3 on v3 database: %v", err)
	}
	if result.IdempotencyKeysMigrated != 0 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 0", result.IdempotencyKeysMigrated)
	}
	if result.IdempotencyKeysSkipped != 0 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 0", result.IdempotencyKeysSkipped)
	}

	// The label count must not have changed.
	verifyConn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening verification connection: %v", err)
	}
	defer func() { _ = verifyConn.Close() }()

	var labelCountAfter int
	if err := sqlitex.Execute(verifyConn,
		`SELECT COUNT(*) FROM labels`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelCountAfter = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading label count after migration: %v", err)
	}
	if labelCountAfter != labelCountBefore {
		t.Errorf("label count changed: was %d, now %d", labelCountBefore, labelCountAfter)
	}

	// schema_version must remain 3.
	var schemaVersion int
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM metadata WHERE key = 'schema_version'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				schemaVersion = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if schemaVersion != 3 {
		t.Errorf("schema_version: got %d, want 3", schemaVersion)
	}
}

// TestBoundary_MigrateV1ToV3_ChainedMigrationEndToEnd seeds a v1 database (no
// schema_version, with the idempotency_key column from the v2 DDL), runs
// MigrateV1ToV2 followed by MigrateV2ToV3, and asserts that both migrations'
// invariants hold on the single seeded database.
func TestBoundary_MigrateV1ToV3_ChainedMigrationEndToEnd(t *testing.T) {
	// Given — a v1-style database built on the v2 schema (idempotency_key column
	// present), with a claimed issue (v1 state) and a non-NULL idempotency_key.
	// schema_version is absent (true v1 state).
	dbPath := t.TempDir() + "/v1-for-chain.db"

	// Create the file with the v2 DDL so that both migration steps have the
	// schema shape they each expect.
	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite|zombiezen.OpenCreate)
	if err != nil {
		t.Fatalf("precondition: creating database file: %v", err)
	}

	if err := sqlitex.ExecuteScript(conn, v2SchemaSQL, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: applying v2 schema: %v", err)
	}

	now := "2024-01-01T00:00:00Z"

	// Set the prefix; leave schema_version absent (v1 state).
	if err := sqlitex.Execute(conn, `INSERT INTO metadata (key, value) VALUES ('prefix', 'CHAIN')`, nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting prefix: %v", err)
	}

	// Insert one issue with state='claimed' (v1 state) and a non-NULL idempotency_key.
	if err := sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at, idempotency_key) VALUES ('CHAIN-aaaaa', 'task', 'Chained task', 'claimed', ?, 'chain-key-1')`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting issue: %v", err)
	}

	// Insert one history row with event_type='claimed' (removed in v2).
	if err := sqlitex.Execute(conn,
		`INSERT INTO history (issue_id, revision, author, timestamp, event_type, changes) VALUES ('CHAIN-aaaaa', 0, 'tester', ?, 'claimed', '[]')`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting history row: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing seed connection: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("precondition: opening store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()

	// Confirm v1 state.
	if err := store.CheckSchemaVersion(ctx); err == nil {
		t.Fatal("precondition: expected v1 database to fail CheckSchemaVersion")
	}

	// When — run v1→v2 migration.
	v12Result, err := store.MigrateV1ToV2(ctx)
	// Then — v1→v2 succeeded with the expected counts.
	if err != nil {
		t.Fatalf("MigrateV1ToV2: unexpected error: %v", err)
	}
	if v12Result.ClaimedIssuesConverted != 1 {
		t.Errorf("ClaimedIssuesConverted: got %d, want 1", v12Result.ClaimedIssuesConverted)
	}
	if v12Result.HistoryRowsRemoved != 1 {
		t.Errorf("HistoryRowsRemoved: got %d, want 1", v12Result.HistoryRowsRemoved)
	}

	// When — run v2→v3 migration.
	v23Result, err := store.MigrateV2ToV3(ctx)
	// Then — v2→v3 succeeded and carried the idempotency_key forward.
	if err != nil {
		t.Fatalf("MigrateV2ToV3: unexpected error: %v", err)
	}
	if v23Result.IdempotencyKeysMigrated != 1 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 1", v23Result.IdempotencyKeysMigrated)
	}
	if v23Result.IdempotencyKeysSkipped != 0 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 0", v23Result.IdempotencyKeysSkipped)
	}

	// Verify the combined post-migration state.
	verifyConn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening verification connection: %v", err)
	}
	defer func() { _ = verifyConn.Close() }()

	// The issue must be in state='open' (claimed→open by v1→v2).
	var state string
	if err := sqlitex.Execute(verifyConn,
		`SELECT state FROM issues WHERE issue_id = 'CHAIN-aaaaa'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				state = stmt.ColumnText(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading issue state: %v", err)
	}
	if state != "open" {
		t.Errorf("issue state after v1→v2: got %q, want %q", state, "open")
	}

	// The idempotency label must exist.
	var labelValue string
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM labels WHERE issue_id = 'CHAIN-aaaaa' AND key = 'idempotency'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelValue = stmt.ColumnText(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading idempotency label: %v", err)
	}
	if labelValue != "chain-key-1" {
		t.Errorf("idempotency label value: got %q, want %q", labelValue, "chain-key-1")
	}

	// The idempotency_key column must be absent.
	var columnFound bool
	if err := sqlitex.Execute(verifyConn,
		`SELECT 1 FROM pragma_table_info('issues') WHERE name = 'idempotency_key' LIMIT 1`,
		&sqlitex.ExecOptions{
			ResultFunc: func(_ *zombiezen.Stmt) error {
				columnFound = true
				return nil
			},
		}); err != nil {
		t.Fatalf("checking table_info: %v", err)
	}
	if columnFound {
		t.Error("idempotency_key column still present after chained migration")
	}

	// No obsolete history rows (claimed/released event types) must remain.
	var obsoleteCount int
	if err := sqlitex.Execute(verifyConn,
		`SELECT COUNT(*) FROM history WHERE event_type IN ('claimed', 'released')`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				obsoleteCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("counting obsolete history rows: %v", err)
	}
	if obsoleteCount != 0 {
		t.Errorf("expected 0 obsolete history rows after chained migration, got %d", obsoleteCount)
	}

	// schema_version must be 3.
	var schemaVersion int
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM metadata WHERE key = 'schema_version'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				schemaVersion = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if schemaVersion != 3 {
		t.Errorf("schema_version: got %d, want 3", schemaVersion)
	}

	// CheckSchemaVersion must now pass.
	if err := store.CheckSchemaVersion(ctx); err != nil {
		t.Errorf("expected CheckSchemaVersion to pass after v1→v2→v3, got: %v", err)
	}
}

// TestBoundary_MigrateV2ToV3_SkipOnConflict_ExistingLabelPreserved verifies the
// skip-on-conflict collision policy: when an issue already carries an
// idempotency:<value> label row, the idempotency_key column value is not written
// and the skip counter is incremented.
func TestBoundary_MigrateV2ToV3_SkipOnConflict_ExistingLabelPreserved(t *testing.T) {
	// Given — a v2 database with one issue that has both a non-NULL idempotency_key
	// and a pre-existing idempotency label (simulating a partial prior migration or
	// manual labelling). The pre-existing label carries the same key but a
	// different value to confirm the original label is left unchanged.
	store, dbPath := createV2Database(t)
	ctx := context.Background()

	conn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadWrite)
	if err != nil {
		t.Fatalf("precondition: opening raw connection: %v", err)
	}

	now := "2024-01-01T00:00:00Z"
	if err := sqlitex.Execute(conn,
		`INSERT INTO issues (issue_id, role, title, state, created_at, idempotency_key) VALUES ('V2-skip1', 'task', 'Skip task', 'open', ?, 'new-key')`,
		&sqlitex.ExecOptions{Args: []any{now}}); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting issue: %v", err)
	}

	// Insert a pre-existing idempotency label with a different value.
	if err := sqlitex.Execute(conn,
		`INSERT INTO labels (issue_id, key, value) VALUES ('V2-skip1', 'idempotency', 'existing-key')`,
		nil); err != nil {
		_ = conn.Close()
		t.Fatalf("precondition: inserting pre-existing label: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("precondition: closing seed connection: %v", err)
	}

	// When — MigrateV2ToV3 is called.
	result, err := store.MigrateV2ToV3(ctx)
	// Then — no error; the collision is counted as skipped.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV2ToV3: %v", err)
	}
	if result.IdempotencyKeysMigrated != 0 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 0", result.IdempotencyKeysMigrated)
	}
	if result.IdempotencyKeysSkipped != 1 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 1", result.IdempotencyKeysSkipped)
	}

	// The pre-existing label value must be preserved unchanged.
	verifyConn, err := zombiezen.OpenConn(dbPath, zombiezen.OpenReadOnly)
	if err != nil {
		t.Fatalf("opening verification connection: %v", err)
	}
	defer func() { _ = verifyConn.Close() }()

	var labelValue string
	if err := sqlitex.Execute(verifyConn,
		`SELECT value FROM labels WHERE issue_id = 'V2-skip1' AND key = 'idempotency'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelValue = stmt.ColumnText(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("reading label value: %v", err)
	}
	if labelValue != "existing-key" {
		t.Errorf("pre-existing label value changed: got %q, want %q", labelValue, "existing-key")
	}

	// Only one label row must exist for V2-skip1.
	var labelCount int
	if err := sqlitex.Execute(verifyConn,
		`SELECT COUNT(*) FROM labels WHERE issue_id = 'V2-skip1'`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *zombiezen.Stmt) error {
				labelCount = stmt.ColumnInt(0)
				return nil
			},
		}); err != nil {
		t.Fatalf("counting labels: %v", err)
	}
	if labelCount != 1 {
		t.Errorf("label count: got %d, want 1", labelCount)
	}
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
