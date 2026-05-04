package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- dot-np-directory ---

// TestRunDotNpDirectory_DirectoryPresent_Passes verifies that the check passes
// when a .np/ directory exists somewhere in the ancestor chain of WorkDir.
func TestRunDotNpDirectory_DirectoryPresent_Passes(t *testing.T) {
	t.Parallel()

	// Given — a temp directory containing a .np/ subdirectory.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runDotNpDirectory(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunDotNpDirectory_NoDirectory_ReturnsFinding verifies that the check
// returns an error finding when no .np/ directory exists from the given working
// directory up to the filesystem root.
func TestRunDotNpDirectory_NoDirectory_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a temp directory with no .np/ directory in its ancestor chain.
	dir := t.TempDir()
	svc := newTestSvc()

	// When
	result, err := runDotNpDirectory(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (no .np/ directory), got nil")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary in finding")
	}
}

// TestRunDotNpDirectory_FindsInAncestor_Passes verifies that the check
// succeeds when .np/ is in a parent directory, not in the workdir itself.
func TestRunDotNpDirectory_FindsInAncestor_Passes(t *testing.T) {
	t.Parallel()

	// Given — .np/ is in the parent; workDir is a subdirectory.
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	child := filepath.Join(parent, "sub")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatalf("precondition: create subdirectory: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runDotNpDirectory(t.Context(), svc, driving.DoctorInput{WorkDir: child})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass: .np/ in ancestor), got finding: %q", result.Summary)
	}
}

// --- database-exists ---

// TestRunDatabaseExists_FilePresent_Passes verifies that a non-empty database
// file at DBPath causes the check to pass.
func TestRunDatabaseExists_FilePresent_Passes(t *testing.T) {
	t.Parallel()

	// Given — a non-empty file at DBPath.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatalf("precondition: create db file: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runDatabaseExists(t.Context(), svc, driving.DoctorInput{DBPath: dbPath})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunDatabaseExists_FileMissing_ReturnsFinding verifies that a missing
// database file produces an error finding.
func TestRunDatabaseExists_FileMissing_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a path that does not exist.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "does-not-exist.db")
	svc := newTestSvc()

	// When
	result, err := runDatabaseExists(t.Context(), svc, driving.DoctorInput{DBPath: dbPath})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (missing file), got nil")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary in finding")
	}
}

// TestRunDatabaseExists_ZeroByteFile_ReturnsFinding verifies that a zero-byte
// file at DBPath produces an error finding.
func TestRunDatabaseExists_ZeroByteFile_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a zero-byte file at DBPath.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte{}, 0o600); err != nil {
		t.Fatalf("precondition: create zero-byte db file: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runDatabaseExists(t.Context(), svc, driving.DoctorInput{DBPath: dbPath})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (zero-byte file), got nil")
	}
}

// TestRunDatabaseExists_EmptyDBPath_ReturnsFinding verifies that an empty
// DBPath (no .np/ directory found) produces an error finding.
func TestRunDatabaseExists_EmptyDBPath_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — no DB path supplied (dot-np-directory should have cascade-skipped this).
	svc := newTestSvc()

	// When
	result, err := runDatabaseExists(t.Context(), svc, driving.DoctorInput{DBPath: ""})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (empty DBPath), got nil")
	}
}

// TestRunDatabaseExists_DBOpenError_ReturnsFinding verifies that when the file
// exists and is non-empty but SQLite could not open it, the check reports a
// finding describing the SQLite failure.
func TestRunDatabaseExists_DBOpenError_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a non-empty file at DBPath and a simulated open error.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("not a valid sqlite file"), 0o600); err != nil {
		t.Fatalf("precondition: create corrupt db file: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runDatabaseExists(t.Context(), svc, driving.DoctorInput{
		DBPath:      dbPath,
		DBOpenError: os.ErrPermission,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (SQLite could not open file), got nil")
	}
}

// --- storage-integrity ---

// TestRunStorageIntegrity_HealthyDatabase_Passes verifies that the check
// passes when both PRAGMA integrity_check and PRAGMA foreign_key_check report
// no issues.
func TestRunStorageIntegrity_HealthyDatabase_Passes(t *testing.T) {
	t.Parallel()

	// Given — a service backed by a fake repo that reports no integrity issues.
	svc := newSvcWithDB(t, &checkFakeDBRepo{})

	// When
	result, err := runStorageIntegrity(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunStorageIntegrity_IntegrityCheckFails_ReturnsFinding verifies that a
// failed PRAGMA integrity_check produces an error finding.
func TestRunStorageIntegrity_IntegrityCheckFails_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a fake repo that returns an integrity check error.
	svc := newSvcWithDB(t, &checkFakeDBRepo{integrityErr: os.ErrPermission})

	// When
	result, err := runStorageIntegrity(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (integrity check failed), got nil")
	}
}

// TestRunStorageIntegrity_FKViolations_ReturnsFinding verifies that a positive
// foreign-key violation count produces an error finding.
func TestRunStorageIntegrity_FKViolations_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a fake repo that reports foreign-key violations.
	svc := newSvcWithDB(t, &checkFakeDBRepo{fkViolations: 3})

	// When
	result, err := runStorageIntegrity(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (FK violations), got nil")
	}
}

// --- schema-version ---

// TestRunSchemaVersion_CurrentVersion_Passes verifies that when the database
// schema version matches the binary's expected version, the check passes.
func TestRunSchemaVersion_CurrentVersion_Passes(t *testing.T) {
	t.Parallel()

	// Given — DB schema version matches the binary's current version.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion})

	// When
	result, err := runSchemaVersion(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunSchemaVersion_OlderVersion_ReturnsFindingWithoutMarker verifies the
// spec's asymmetry: an older-DB error does not include doctorSchemaNewerDBMarker,
// so SkipsAll returns false and subsequent checks still run.
func TestRunSchemaVersion_OlderVersion_ReturnsFindingWithoutMarker(t *testing.T) {
	t.Parallel()

	// Given — DB schema version is older than the binary.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion - 1})

	// When
	result, err := runSchemaVersion(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (older DB), got nil")
	}
	for _, a := range result.Affected {
		if _, ok := a.(doctorSchemaNewerDBMarker); ok {
			t.Error("older-DB finding must not contain doctorSchemaNewerDBMarker")
		}
	}
}

// TestRunSchemaVersion_NewerVersion_ReturnsFindingWithMarker verifies that a
// newer-than-binary schema version includes doctorSchemaNewerDBMarker so the
// SkipsAll callback fires and all remaining checks (including Environment) are
// skipped.
func TestRunSchemaVersion_NewerVersion_ReturnsFindingWithMarker(t *testing.T) {
	t.Parallel()

	// Given — DB schema version is newer than the binary.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: driven.CurrentSchemaVersion + 1})

	// When
	result, err := runSchemaVersion(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (newer DB), got nil")
	}
	var hasMarker bool
	for _, a := range result.Affected {
		if _, ok := a.(doctorSchemaNewerDBMarker); ok {
			hasMarker = true
		}
	}
	if !hasMarker {
		t.Error("newer-DB finding must contain doctorSchemaNewerDBMarker")
	}
}

// --- column-data-validity ---

// TestRunColumnDataValidity_CleanDatabase_Passes verifies that the check
// passes when the database contains no malformed column values.
func TestRunColumnDataValidity_CleanDatabase_Passes(t *testing.T) {
	t.Parallel()

	// Given — a fake repo reporting zero column violations.
	svc := newSvcWithDB(t, &checkFakeDBRepo{})

	// When
	result, err := runColumnDataValidity(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunColumnDataValidity_Violations_ReturnsFinding verifies that malformed
// column values produce a finding whose summary references the violation count.
func TestRunColumnDataValidity_Violations_ReturnsFinding(t *testing.T) {
	t.Parallel()

	// Given — a fake repo reporting four column violations.
	svc := newSvcWithDB(t, &checkFakeDBRepo{columnViolations: 4})

	// When
	result, err := runColumnDataValidity(t.Context(), svc, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (column violations), got nil")
	}
}

// --- end-to-end cascade-trigger tests using real check implementations ---
//
// Each test below exercises one cascade trigger through the full orchestrator
// using the real Run function for the triggering check and a minimal healthy
// workspace for all prerequisites.

// TestDoctorDatabaseChecks_DotNpDirectoryFails_SkipsDBGraphLifecycle verifies
// the end-to-end cascade when dot-np-directory detects no .np/ directory: all
// DB, Graph health, and Issue lifecycle checks are skipped while Environment
// checks still run.
func TestDoctorDatabaseChecks_DotNpDirectoryFails_SkipsDBGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — workDir with no .np/ directory anywhere in its ancestor chain.
	workDir := t.TempDir()
	svc := newTestSvc()
	registry := doctorRegistry()

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, driving.DoctorInput{WorkDir: workDir})
	// Then — 1 error (dot-np-directory), 13 skipped, environment checks pass.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Fatalf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "dot-np-directory" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "dot-np-directory")
	}
	if got := len(out.Skipped); got != 13 {
		t.Errorf("skipped: got %d, want 13: %v", got, skippedSlugs(out.Skipped))
	}
}

// TestDoctorDatabaseChecks_DatabaseExistsFails_SkipsRemainingDBGraphLifecycle
// verifies the end-to-end cascade when database-exists finds no file at DBPath.
func TestDoctorDatabaseChecks_DatabaseExistsFails_SkipsRemainingDBGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — .np/ exists but the db file path is non-existent.
	workDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(workDir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(workDir, ".np", "missing.db")
	svc := newTestSvc()
	registry := doctorRegistry()

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, driving.DoctorInput{
		WorkDir: workDir,
		DBPath:  dbPath,
	})
	// Then — 1 error (database-exists), 12 skipped, dot-np-directory + environment pass.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Fatalf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "database-exists" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "database-exists")
	}
	if got := len(out.Skipped); got != 12 {
		t.Errorf("skipped: got %d, want 12: %v", got, skippedSlugs(out.Skipped))
	}
}

// TestDoctorDatabaseChecks_StorageIntegrityFails_SkipsRemainingDBGraphLifecycle
// verifies the end-to-end cascade when storage-integrity detects an integrity
// problem in a connected database.
func TestDoctorDatabaseChecks_StorageIntegrityFails_SkipsRemainingDBGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — a workspace where integrity_check returns an error.
	svc := newSvcWithDB(t, &checkFakeDBRepo{
		integrityErr:  os.ErrPermission,
		schemaVersion: driven.CurrentSchemaVersion,
	})
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, doctorRegistry(), input)
	// Then — 1 error (storage-integrity), 11 skipped, dot-np-directory +
	// database-exists + environment pass.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Fatalf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "storage-integrity" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "storage-integrity")
	}
	if got := len(out.Skipped); got != 11 {
		t.Errorf("skipped: got %d, want 11: %v", got, skippedSlugs(out.Skipped))
	}
}

// TestDoctorDatabaseChecks_SchemaVersionNewerDB_SkipsAllIncludingEnvironment
// verifies the spec's hardest cascade: a newer-than-binary schema version skips
// every remaining check including Environment checks.
func TestDoctorDatabaseChecks_SchemaVersionNewerDB_SkipsAllIncludingEnvironment(t *testing.T) {
	t.Parallel()

	// Given — database schema version is newer than the binary.
	svc := newSvcWithDB(t, &checkFakeDBRepo{
		schemaVersion: driven.CurrentSchemaVersion + 1,
	})
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, doctorRegistry(), input)
	// Then — 1 error (schema-version with newer-DB marker), 12 skipped
	// (everything after schema-version including Environment).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Fatalf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "schema-version" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "schema-version")
	}
	if got := len(out.Skipped); got != 12 {
		t.Errorf("skipped: got %d, want 12: %v", got, skippedSlugs(out.Skipped))
	}
}

// TestDoctorDatabaseChecks_ColumnDataValidityFails_SkipsRemainingDBGraphLifecycle
// verifies the cascade when column-data-validity detects malformed values.
func TestDoctorDatabaseChecks_ColumnDataValidityFails_SkipsRemainingDBGraphLifecycle(t *testing.T) {
	t.Parallel()

	// Given — column data has violations; all earlier checks pass.
	svc := newSvcWithDB(t, &checkFakeDBRepo{
		schemaVersion:    driven.CurrentSchemaVersion,
		columnViolations: 2,
	})
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, doctorRegistry(), input)
	// Then — 1 error (column-data-validity), 9 skipped (remaining DB +
	// Graph health + Issue lifecycle); database-cascade-five + Environment pass.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(out.Errors); got != 1 {
		t.Fatalf("errors: got %d, want 1", got)
	}
	if out.Errors[0].Check != "column-data-validity" {
		t.Errorf("error check: got %q, want %q", out.Errors[0].Check, "column-data-validity")
	}
	if got := len(out.Skipped); got != 9 {
		t.Errorf("skipped: got %d, want 9: %v", got, skippedSlugs(out.Skipped))
	}
	// Per the issue's acceptance criterion: column-data-validity failure must
	// specifically skip closed-parent-with-open-child and invalid-parent-reference
	// (the two non-cascade Database checks). Verify by slug, not just by count.
	skipped := skippedSlugs(out.Skipped)
	for _, want := range []string{"closed-parent-with-open-child", "invalid-parent-reference"} {
		if !slices.Contains(skipped, want) {
			t.Errorf("expected %q in skipped list, got %v", want, skipped)
		}
	}
}

// --- test infrastructure ---

// checkFakeDBRepo is a hand-written DatabaseRepository fake used exclusively
// by the doctor check unit tests. Each field controls the return value of the
// corresponding method; unset fields default to healthy (zero, nil).
type checkFakeDBRepo struct {
	integrityErr     error
	fkViolations     int
	schemaVersion    int
	columnViolations int
}

func (r *checkFakeDBRepo) IntegrityCheck(_ context.Context) error { return r.integrityErr }
func (r *checkFakeDBRepo) ForeignKeyCheck(_ context.Context) (int, error) {
	return r.fkViolations, nil
}

func (r *checkFakeDBRepo) ValidateColumnData(_ context.Context) (int, error) {
	return r.columnViolations, nil
}

func (r *checkFakeDBRepo) GetSchemaVersion(_ context.Context) (int, error) {
	return r.schemaVersion, nil
}
func (r *checkFakeDBRepo) SetSchemaVersion(_ context.Context, _ int) error       { return nil }
func (r *checkFakeDBRepo) InitDatabase(_ context.Context, _ string) error        { return nil }
func (r *checkFakeDBRepo) GetPrefix(_ context.Context) (string, error)           { return "TST", nil }
func (r *checkFakeDBRepo) GC(_ context.Context, _ bool) (int, int, error)        { return 0, 0, nil }
func (r *checkFakeDBRepo) CountDeletedRatio(_ context.Context) (int, int, error) { return 0, 0, nil }
func (r *checkFakeDBRepo) ClearAllData(_ context.Context) error                  { return nil }
func (r *checkFakeDBRepo) RestoreIssueRaw(_ context.Context, _ domain.BackupIssueRecord) error {
	return nil
}

func (r *checkFakeDBRepo) RestoreCommentRaw(_ context.Context, _ string, _ domain.BackupCommentRecord) error {
	return nil
}

func (r *checkFakeDBRepo) RestoreClaimRaw(_ context.Context, _ string, _ domain.BackupClaimRecord) error {
	return nil
}

func (r *checkFakeDBRepo) RestoreRelationshipRaw(_ context.Context, _ string, _ domain.BackupRelationshipRecord) error {
	return nil
}

func (r *checkFakeDBRepo) RestoreHistoryRaw(_ context.Context, _ string, _ domain.BackupHistoryRecord) error {
	return nil
}

func (r *checkFakeDBRepo) RestoreLabelRaw(_ context.Context, _ string, _ domain.BackupLabelRecord) error {
	return nil
}
func (r *checkFakeDBRepo) RebuildFTS(_ context.Context) error { return nil }

// checkFakeUnitOfWork wraps a checkFakeDBRepo to satisfy driven.UnitOfWork.
// Database() is the primary dependency for the cascade-prerequisite checks.
// Issues() returns an empty graphFakeIssueRepo so that graph-integrity checks
// that run after the cascade (closed-parent-with-open-child, invalid-parent-
// reference) pass cleanly on a healthy database fixture with no issues.
type checkFakeUnitOfWork struct{ db *checkFakeDBRepo }

func (u *checkFakeUnitOfWork) Database() driven.DatabaseRepository { return u.db }
func (u *checkFakeUnitOfWork) Issues() driven.IssueRepository      { return &graphFakeIssueRepo{} }
func (u *checkFakeUnitOfWork) Comments() driven.CommentRepository  { return nil }
func (u *checkFakeUnitOfWork) Claims() driven.ClaimRepository      { return nil }
func (u *checkFakeUnitOfWork) Relationships() driven.RelationshipRepository {
	return nil
}
func (u *checkFakeUnitOfWork) History() driven.HistoryRepository { return nil }

// checkFakeTransactor satisfies driven.Transactor by delegating every
// transaction to a single checkFakeUnitOfWork backed by the given
// checkFakeDBRepo. Vacuum is a no-op.
type checkFakeTransactor struct{ db *checkFakeDBRepo }

func (t *checkFakeTransactor) WithTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(&checkFakeUnitOfWork{db: t.db})
}

func (t *checkFakeTransactor) WithReadTransaction(_ context.Context, fn func(driven.UnitOfWork) error) error {
	return fn(&checkFakeUnitOfWork{db: t.db})
}
func (t *checkFakeTransactor) Vacuum(_ context.Context) error { return nil }

// newSvcWithDB creates a serviceImpl with a fake transactor backed by the
// given checkFakeDBRepo. Used by tests that exercise the DB-connected doctor
// check functions (storage-integrity, schema-version, column-data-validity).
func newSvcWithDB(t *testing.T, db *checkFakeDBRepo) *serviceImpl {
	t.Helper()
	return &serviceImpl{tx: &checkFakeTransactor{db: db}}
}

// Ensure checkFakeDBRepo satisfies the driven.DatabaseRepository interface at
// compile time. The driven.IssueRepository and other repository interfaces are
// satisfied by returning nil from checkFakeUnitOfWork — the check functions
// under test never call those accessors.
var _ driven.DatabaseRepository = (*checkFakeDBRepo)(nil)
