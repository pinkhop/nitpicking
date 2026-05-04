//go:build boundary

package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"

	goSQLite "zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	adapterSQLite "github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// workspaceEnv holds a service and the workspace paths needed to pass
// WorkDir and DBPath through DoctorInput so the cascade prereqs pass.
type workspaceEnv struct {
	svc     driving.Service
	workDir string
	dbPath  string
}

// setupWorkspaceEnv creates a proper np workspace (including .np/ directory and
// the database file) so that the dot-np-directory and database-exists doctor
// checks pass. This is necessary for boundary tests that exercise checks later
// in the cascade (storage-integrity, schema-version, column-data-validity).
func setupWorkspaceEnv(t *testing.T) *workspaceEnv {
	t.Helper()
	workDir := t.TempDir()
	npDir := filepath.Join(workDir, ".np")
	if err := os.Mkdir(npDir, 0o750); err != nil {
		t.Fatalf("precondition: create .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "nitpicking.db")
	store, err := adapterSQLite.Create(dbPath)
	if err != nil {
		t.Fatalf("precondition: create database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := core.New(store, store)
	if err := svc.Init(t.Context(), "TEST"); err != nil {
		t.Fatalf("precondition: init database: %v", err)
	}
	return &workspaceEnv{svc: svc, workDir: workDir, dbPath: dbPath}
}

// doctorInputFor returns a DoctorInput with the workspace paths from env so
// that all cascade-prerequisite checks pass before the target check runs.
func doctorInputFor(env *workspaceEnv) driving.DoctorInput {
	return driving.DoctorInput{
		WorkDir: env.workDir,
		DBPath:  env.dbPath,
	}
}

// openRawConnForPath opens a raw zombiezen SQLite connection to dbPath.
func openRawConnForPath(t *testing.T, dbPath string) *goSQLite.Conn {
	t.Helper()
	conn, err := goSQLite.OpenConn(dbPath, goSQLite.OpenReadWrite)
	if err != nil {
		t.Fatalf("open raw connection to %s: %v", dbPath, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// execStatement executes a single SQL statement on conn. Fails the test on error.
func execStatement(t *testing.T, conn *goSQLite.Conn, sql string, args ...any) {
	t.Helper()
	err := sqlitex.Execute(conn, sql, &sqlitex.ExecOptions{Args: args})
	if err != nil {
		t.Fatalf("execStatement(%q): %v", sql, err)
	}
}

// --- ForeignKeyCheck detects FK violations ---

// TestBoundary_ForeignKeyCheck_OrphanedRow_ReturnsPositiveCount verifies that
// ForeignKeyCheck (called inside the storage-integrity doctor check) returns a
// positive violation count when a row references a non-existent parent issue.
// The violation is inserted via a raw connection with FK enforcement disabled.
func TestBoundary_ForeignKeyCheck_OrphanedRow_ReturnsPositiveCount(t *testing.T) {
	env := setupWorkspaceEnv(t)
	ctx := t.Context()

	// Given — a normal task so the database is non-empty.
	_ = createIntTask(t, env.svc, "Normal task")

	// Insert an issue row whose parent_id references a non-existent ID.
	// FK enforcement must be disabled for the insertion only; PRAGMA
	// foreign_key_check scans for violations regardless of the FK setting.
	raw := openRawConnForPath(t, env.dbPath)
	execStatement(t, raw, `PRAGMA foreign_keys = OFF`)
	execStatement(t, raw,
		`INSERT INTO issues (issue_id, role, title, state, priority, created_at, parent_id)
		 VALUES (?, 'task', 'Orphan', 'open', 'P2', '2024-01-01T00:00:00Z', ?)`,
		"TEST-orphan1", "TEST-doesnotexist",
	)
	execStatement(t, raw, `PRAGMA foreign_keys = ON`)

	// When — run the doctor with WorkDir/DBPath so storage-integrity is reached.
	out, err := env.svc.Doctor(ctx, doctorInputFor(env))
	// Then — storage-integrity must detect the FK violation.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	for _, f := range out.Errors {
		if f.Check == "storage-integrity" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected storage-integrity error for FK violation, errors=%v warnings=%v",
			out.Errors, out.Warnings)
	}
}

// --- ValidateColumnData detects column violations ---

// TestBoundary_ValidateColumnData_InvalidState_ReturnsPositiveCount verifies
// that ValidateColumnData (called inside column-data-validity) detects a row
// with an enum value outside the allowed state set.
func TestBoundary_ValidateColumnData_InvalidState_ReturnsPositiveCount(t *testing.T) {
	env := setupWorkspaceEnv(t)
	ctx := t.Context()

	// Given — insert a row with an invalid state via raw SQL.
	raw := openRawConnForPath(t, env.dbPath)
	execStatement(t, raw,
		`INSERT INTO issues (issue_id, role, title, state, priority, created_at)
		 VALUES (?, 'task', 'Bad state', 'INVALID_STATE', 'P2', '2024-01-01T00:00:00Z')`,
		"TEST-badst1",
	)

	// When — run the doctor with WorkDir/DBPath so column-data-validity is reached.
	out, err := env.svc.Doctor(ctx, doctorInputFor(env))
	// Then — column-data-validity must detect the invalid state.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	for _, f := range out.Errors {
		if f.Check == "column-data-validity" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected column-data-validity error for invalid state, errors=%v warnings=%v",
			out.Errors, out.Warnings)
	}
}

// TestBoundary_ValidateColumnData_InvalidTimestamp_ReturnsPositiveCount
// verifies that ValidateColumnData detects a timestamp that SQLite's datetime()
// function cannot parse.
func TestBoundary_ValidateColumnData_InvalidTimestamp_ReturnsPositiveCount(t *testing.T) {
	env := setupWorkspaceEnv(t)
	ctx := t.Context()

	// Given — insert a row with a clearly non-datetime created_at.
	raw := openRawConnForPath(t, env.dbPath)
	execStatement(t, raw,
		`INSERT INTO issues (issue_id, role, title, state, priority, created_at)
		 VALUES (?, 'task', 'Bad timestamp', 'open', 'P2', ?)`,
		"TEST-badts1", "NOT-A-DATE",
	)

	// When
	out, err := env.svc.Doctor(ctx, doctorInputFor(env))
	// Then — column-data-validity must detect the malformed timestamp.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	for _, f := range out.Errors {
		if f.Check == "column-data-validity" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected column-data-validity error for invalid timestamp, errors=%v warnings=%v",
			out.Errors, out.Warnings)
	}
}

// TestBoundary_ValidateColumnData_InvalidJSON_ReturnsPositiveCount verifies
// that ValidateColumnData detects malformed JSON in history.changes.
func TestBoundary_ValidateColumnData_InvalidJSON_ReturnsPositiveCount(t *testing.T) {
	env := setupWorkspaceEnv(t)
	ctx := t.Context()

	// Given — create an issue then insert a history row with invalid JSON changes.
	taskOut, err := env.svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Task with bad history",
		Author: "alice",
	})
	if err != nil {
		t.Fatalf("precondition: create task: %v", err)
	}
	raw := openRawConnForPath(t, env.dbPath)
	execStatement(t, raw,
		`INSERT INTO history (issue_id, revision, author, timestamp, event_type, changes)
		 VALUES (?, 99, 'alice', '2024-01-01T00:00:00Z', 'updated', ?)`,
		taskOut.Issue.ID().String(), "NOT VALID JSON {{{",
	)

	// When
	out, err := env.svc.Doctor(ctx, doctorInputFor(env))
	// Then — column-data-validity must detect the malformed JSON.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	for _, f := range out.Errors {
		if f.Check == "column-data-validity" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected column-data-validity error for invalid JSON, errors=%v warnings=%v",
			out.Errors, out.Warnings)
	}
}
