package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// Store provides SQLite-backed persistence for the nitpicking domain.
type Store struct {
	pool *sqlitex.Pool
}

// prepareConn configures per-connection pragmas. Called once per pooled
// connection on first use.
func prepareConn(conn *sqlite.Conn) error {
	// SetBusyTimeout is the default on zombiezen connections, but we set an
	// explicit 5-second ceiling so concurrent processes wait rather than
	// fail immediately.
	conn.SetBusyTimeout(5 * time.Second)

	if err := sqlitex.ExecuteTransient(conn, "PRAGMA foreign_keys = ON;", nil); err != nil {
		return fmt.Errorf("pragma foreign_keys: %w", err)
	}
	return nil
}

// Create creates a new SQLite database at dbPath and applies the schema.
// Intended to be called once during database initialisation ("np init").
// Subsequent access should use Open, which skips DDL.
func Create(dbPath string) (*Store, error) {
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize:    1,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "create database", Err: err}
	}

	conn, err := pool.Take(context.Background())
	if err != nil {
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "take connection for schema", Err: errors.Join(err, closeErr)}
	}

	err = sqlitex.ExecuteScript(conn, schemaSQL, nil)
	pool.Put(conn)
	if err != nil {
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "apply schema", Err: errors.Join(err, closeErr)}
	}

	return &Store{pool: pool}, nil
}

// Open opens an existing SQLite database at dbPath. It does not apply schema
// DDL — the database must already have been created with Create.
func Open(dbPath string) (*Store, error) {
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize:    1,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "open database", Err: err}
	}
	return &Store{pool: pool}, nil
}

// Close releases all pooled connections.
func (s *Store) Close() error {
	return s.pool.Close()
}

// Vacuum reclaims disk space and defragments the database file. Must be
// called outside any transaction — takes a connection from the pool, runs
// VACUUM, and returns it.
func (s *Store) Vacuum(ctx context.Context) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return &domain.DatabaseError{Op: "take connection for vacuum", Err: err}
	}
	defer s.pool.Put(conn)

	if err := sqlitex.Execute(conn, `VACUUM`, nil); err != nil {
		return &domain.DatabaseError{Op: "vacuum", Err: err}
	}
	return nil
}

// --- Transactor ---

// WithTransaction executes fn within an IMMEDIATE transaction. IMMEDIATE
// acquires a write lock at BEGIN so that busy-handler retries happen at a
// point where no deadlock is possible.
func (s *Store) WithTransaction(ctx context.Context, fn func(uow driven.UnitOfWork) error) (err error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return &domain.DatabaseError{Op: "take connection", Err: err}
	}
	defer s.pool.Put(conn)

	endFn, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return &domain.DatabaseError{Op: "begin transaction", Err: err}
	}
	defer endFn(&err)

	return fn(&connUnitOfWork{conn: conn})
}

// WithReadTransaction executes fn within a deferred (read-only) transaction.
func (s *Store) WithReadTransaction(ctx context.Context, fn func(uow driven.UnitOfWork) error) (err error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return &domain.DatabaseError{Op: "take connection", Err: err}
	}
	defer s.pool.Put(conn)

	endFn := sqlitex.Transaction(conn)
	defer endFn(&err)

	return fn(&connUnitOfWork{conn: conn})
}

// connUnitOfWork wraps a *sqlite.Conn to implement driven.UnitOfWork.
type connUnitOfWork struct {
	conn *sqlite.Conn
}

func (u *connUnitOfWork) Issues() driven.IssueRepository               { return &issueRepo{conn: u.conn} }
func (u *connUnitOfWork) Comments() driven.CommentRepository           { return &commentRepo{conn: u.conn} }
func (u *connUnitOfWork) Claims() driven.ClaimRepository               { return &claimRepo{conn: u.conn} }
func (u *connUnitOfWork) Relationships() driven.RelationshipRepository { return &relRepo{conn: u.conn} }
func (u *connUnitOfWork) History() driven.HistoryRepository            { return &histRepo{conn: u.conn} }
func (u *connUnitOfWork) Database() driven.DatabaseRepository          { return &dbRepo{conn: u.conn} }

// --- DatabaseRepository ---

type dbRepo struct{ conn *sqlite.Conn }

// currentSchemaVersion is the schema version written to the metadata table when
// a new database is initialised. Existing databases without a schema_version
// key are treated as v1 and must be upgraded with 'np admin upgrade'.
const currentSchemaVersion = 2

func (r *dbRepo) InitDatabase(_ context.Context, prefix string) error {
	// Insert the prefix.
	err := sqlitex.Execute(r.conn, `INSERT INTO metadata (key, value) VALUES ('prefix', ?)`, &sqlitex.ExecOptions{
		Args: []any{prefix},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "init database", Err: err}
	}

	// Record the current schema version so the upgrade check can distinguish
	// freshly created databases (v2) from old databases (v1, no key).
	err = sqlitex.Execute(r.conn, `INSERT INTO metadata (key, value) VALUES ('schema_version', ?)`, &sqlitex.ExecOptions{
		Args: []any{currentSchemaVersion},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "init database schema version", Err: err}
	}
	return nil
}

func (r *dbRepo) GetPrefix(_ context.Context) (string, error) {
	prefix, err := sqlitex.ResultText(r.conn.Prep(`SELECT value FROM metadata WHERE key = 'prefix'`))
	if err != nil {
		return "", &domain.DatabaseError{Op: "get prefix", Err: err}
	}
	return prefix, nil
}

// GetSchemaVersion returns the schema version from the metadata table. When the
// schema_version key is absent (v1 database), it returns 0. When present with
// value "2", it returns 2.
func (r *dbRepo) GetSchemaVersion(_ context.Context) (int, error) {
	var version int
	var found bool

	err := sqlitex.Execute(r.conn, `SELECT value FROM metadata WHERE key = 'schema_version'`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			version = stmt.ColumnInt(0)
			return nil
		},
	})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "get schema version", Err: err}
	}

	if !found {
		// Absent schema_version key means v1 schema.
		return 0, nil
	}
	return version, nil
}

// SetSchemaVersion writes version to the metadata table's schema_version key,
// inserting the row when absent or replacing it when already present. Used by
// the upgrade command to record a completed v1→v2 migration within the same
// transaction as the data changes.
func (r *dbRepo) SetSchemaVersion(_ context.Context, version int) error {
	err := sqlitex.Execute(r.conn,
		`INSERT INTO metadata (key, value) VALUES ('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		&sqlitex.ExecOptions{Args: []any{version}})
	if err != nil {
		return &domain.DatabaseError{Op: "set schema version", Err: err}
	}
	return nil
}

// CheckSchemaVersion opens a read transaction, fetches the schema version, and
// returns domain.ErrSchemaMigrationRequired (wrapped in a DatabaseError) when
// the version is below 2. Callers — typically root-command startup hooks —
// use this to gate all database-touching commands on a migrated database.
func (s *Store) CheckSchemaVersion(ctx context.Context) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return &domain.DatabaseError{Op: "take connection for schema check", Err: err}
	}
	defer s.pool.Put(conn)

	repo := &dbRepo{conn: conn}
	version, err := repo.GetSchemaVersion(ctx)
	if err != nil {
		return err
	}

	if version < 2 {
		return &domain.DatabaseError{
			Op:  "schema version check",
			Err: fmt.Errorf("%w: database is at v1 schema — run 'np admin upgrade' to migrate", domain.ErrSchemaMigrationRequired),
		}
	}
	return nil
}

// MigrateV1ToV2 upgrades a v1 database to v2 schema in a single atomic
// transaction, satisfying the driven.Migrator interface. It is safe to call on
// a v2 database — CheckSchemaVersion should be called first so that callers
// can report "up to date" without executing the migration body.
//
// The migration performs three steps within one IMMEDIATE transaction:
//  1. Updates every issue with state="claimed" to state="open", so that the
//     primary state column contains only lifecycle states. The active claims
//     are already stored in the claims table and remain untouched.
//  2. Deletes history rows whose event_type is "claimed" or "released" — these
//     event types were removed in v2; the claims table replaces them.
//  3. Sets schema_version=2 in the metadata table via SetSchemaVersion, marking
//     the migration as complete.
//
// If any step fails the transaction is rolled back and the database is
// unchanged. Returns a driven.MigrationResult with the number of rows affected
// by each step.
func (s *Store) MigrateV1ToV2(ctx context.Context) (driven.MigrationResult, error) {
	var result driven.MigrationResult

	err := s.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		conn := uow.(*connUnitOfWork).conn

		// Step 1: Convert claimed→open on the issues table.
		if err := sqlitex.Execute(conn, `UPDATE issues SET state = 'open' WHERE state = 'claimed'`, nil); err != nil {
			return &domain.DatabaseError{Op: "migrate: convert claimed issues", Err: err}
		}
		result.ClaimedIssuesConverted = int(conn.Changes())

		// Step 2: Remove history rows for removed event types.
		if err := sqlitex.Execute(conn, `DELETE FROM history WHERE event_type IN ('claimed', 'released')`, nil); err != nil {
			return &domain.DatabaseError{Op: "migrate: remove history rows", Err: err}
		}
		result.HistoryRowsRemoved = int(conn.Changes())

		// Step 3: Record the completed migration.
		return uow.Database().SetSchemaVersion(ctx, 2)
	})
	if err != nil {
		return driven.MigrationResult{}, err
	}

	return result, nil
}

func (r *dbRepo) GC(_ context.Context, includeClosed bool) (int, int, error) {
	gcQueries := []struct {
		op    string
		query string
	}{
		{"gc labels", `DELETE FROM labels WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc comments fts", `DELETE FROM comments_fts WHERE comment_id IN (SELECT comment_id FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1))`},
		{"gc comments", `DELETE FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc history", `DELETE FROM history WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc claims", `DELETE FROM claims WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc relationships", `DELETE FROM relationships WHERE source_id IN (SELECT issue_id FROM issues WHERE deleted = 1) OR target_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc issues fts", `DELETE FROM issues_fts WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc parent refs", `UPDATE issues SET parent_id = NULL WHERE parent_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc issues", `DELETE FROM issues WHERE deleted = 1`},
	}

	for _, q := range gcQueries {
		if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
			return 0, 0, &domain.DatabaseError{Op: q.op, Err: err}
		}
	}
	// Capture deleted issue count from the final DELETE on the issues table.
	deletedCount := r.conn.Changes()

	var closedCount int
	if includeClosed {
		closedQueries := []struct {
			op    string
			query string
		}{
			{"gc closed labels", `DELETE FROM labels WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed comments fts", `DELETE FROM comments_fts WHERE comment_id IN (SELECT comment_id FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed'))`},
			{"gc closed comments", `DELETE FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed history", `DELETE FROM history WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed claims", `DELETE FROM claims WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed relationships", `DELETE FROM relationships WHERE source_id IN (SELECT issue_id FROM issues WHERE state = 'closed') OR target_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed issues fts", `DELETE FROM issues_fts WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed parent refs", `UPDATE issues SET parent_id = NULL WHERE parent_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed issues", `DELETE FROM issues WHERE state = 'closed'`},
		}

		for _, q := range closedQueries {
			if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
				return 0, 0, &domain.DatabaseError{Op: q.op, Err: err}
			}
		}
		// Capture closed issue count from the final DELETE on the issues table.
		closedCount = r.conn.Changes()
	}

	return deletedCount, closedCount, nil
}

func (r *dbRepo) IntegrityCheck(_ context.Context) error {
	var result string
	err := sqlitex.Execute(r.conn, `PRAGMA integrity_check`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			result = stmt.ColumnText(0)
			return nil
		},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "integrity check", Err: err}
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}
	return nil
}

func (r *dbRepo) CountVirtualLabelsInTable(_ context.Context) (int, error) {
	count, err := queryInt(r.conn,
		`SELECT COUNT(*) FROM labels WHERE key = ?`, domain.VirtualKeyIdempotency)
	if err != nil {
		return 0, &domain.DatabaseError{Op: "count virtual labels in table", Err: err}
	}
	return count, nil
}

func (r *dbRepo) CountDeletedRatio(_ context.Context) (total, deleted int, err error) {
	err = sqlitex.Execute(r.conn,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN deleted = 1 THEN 1 ELSE 0 END), 0) FROM issues`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				total = stmt.ColumnInt(0)
				deleted = stmt.ColumnInt(1)
				return nil
			},
		})
	if err != nil {
		return 0, 0, &domain.DatabaseError{Op: "count deleted ratio", Err: err}
	}
	return total, deleted, nil
}

// --- Restore Operations ---

func (r *dbRepo) ClearAllData(_ context.Context) error {
	// Disable foreign keys for the duration of the bulk delete so that
	// table deletion order does not matter.
	clearQueries := []struct {
		op    string
		query string
	}{
		{"disable fk", `PRAGMA foreign_keys = OFF`},
		{"clear comments_fts", `DELETE FROM comments_fts`},
		{"clear issues_fts", `DELETE FROM issues_fts`},
		{"clear history", `DELETE FROM history`},
		{"clear claims", `DELETE FROM claims`},
		{"clear relationships", `DELETE FROM relationships`},
		{"clear labels", `DELETE FROM labels`},
		{"clear comments", `DELETE FROM comments`},
		{"clear issues", `DELETE FROM issues`},
		{"clear metadata", `DELETE FROM metadata`},
		{"enable fk", `PRAGMA foreign_keys = ON`},
	}

	for _, q := range clearQueries {
		if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
			return &domain.DatabaseError{Op: q.op, Err: err}
		}
	}
	return nil
}

func (r *dbRepo) RestoreIssueRaw(_ context.Context, rec domain.BackupIssueRecord) error {
	var parentID any
	if rec.ParentID != "" {
		parentID = rec.ParentID
	}
	var idemKey any
	if rec.IdempotencyKey != "" {
		idemKey = rec.IdempotencyKey
	}

	err := sqlitex.Execute(r.conn,
		`INSERT INTO issues (issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		&sqlitex.ExecOptions{
			Args: []any{
				rec.IssueID, rec.Role, rec.Title, rec.Description, rec.AcceptanceCriteria,
				rec.Priority, rec.State, parentID, rec.CreatedAt.Format(time.RFC3339Nano),
				idemKey,
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore issue", Err: err}
	}
	return nil
}

func (r *dbRepo) RestoreCommentRaw(_ context.Context, issueID string, rec domain.BackupCommentRecord) error {
	err := sqlitex.Execute(r.conn,
		`INSERT INTO comments (comment_id, issue_id, author, created_at, body) VALUES (?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{rec.CommentID, issueID, rec.Author, rec.CreatedAt.Format(time.RFC3339Nano), rec.Body},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore comment", Err: err}
	}
	return nil
}

func (r *dbRepo) RestoreClaimRaw(_ context.Context, issueID string, rec domain.BackupClaimRecord) error {
	err := sqlitex.Execute(r.conn,
		`INSERT INTO claims (claim_sha512, issue_id, author, stale_threshold, last_activity) VALUES (?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{rec.ClaimSHA512, issueID, rec.Author, rec.StaleThreshold, rec.LastActivity.Format(time.RFC3339Nano)},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore claim", Err: err}
	}
	return nil
}

func (r *dbRepo) RestoreRelationshipRaw(_ context.Context, sourceID string, rec domain.BackupRelationshipRecord) error {
	err := sqlitex.Execute(r.conn,
		`INSERT INTO relationships (source_id, target_id, rel_type) VALUES (?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{sourceID, rec.TargetID, rec.RelType},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore relationship", Err: err}
	}
	return nil
}

func (r *dbRepo) RestoreHistoryRaw(_ context.Context, issueID string, rec domain.BackupHistoryRecord) error {
	changesJSON, err := json.Marshal(rec.Changes)
	if err != nil {
		return fmt.Errorf("marshalling history changes: %w", err)
	}

	err = sqlitex.Execute(r.conn,
		`INSERT INTO history (entry_id, issue_id, revision, author, timestamp, event_type, changes) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{rec.EntryID, issueID, rec.Revision, rec.Author, rec.Timestamp.Format(time.RFC3339Nano), rec.EventType, string(changesJSON)},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore history", Err: err}
	}
	return nil
}

func (r *dbRepo) RestoreLabelRaw(_ context.Context, issueID string, rec domain.BackupLabelRecord) error {
	err := sqlitex.Execute(r.conn,
		`INSERT INTO labels (issue_id, key, value) VALUES (?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{issueID, rec.Key, rec.Value},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "restore label", Err: err}
	}
	return nil
}

func (r *dbRepo) RebuildFTS(_ context.Context) error {
	ftsQueries := []struct {
		op    string
		query string
	}{
		{"clear issues_fts", `DELETE FROM issues_fts`},
		{"rebuild issues_fts", `INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria) SELECT issue_id, title, description, acceptance_criteria FROM issues`},
		{"clear comments_fts", `DELETE FROM comments_fts`},
		{"rebuild comments_fts", `INSERT INTO comments_fts (comment_id, body) SELECT comment_id, body FROM comments`},
	}

	for _, q := range ftsQueries {
		if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
			return &domain.DatabaseError{Op: q.op, Err: err}
		}
	}
	return nil
}

// --- IssueRepository ---

type issueRepo struct{ conn *sqlite.Conn }

func (r *issueRepo) CreateIssue(_ context.Context, t domain.Issue) error {
	parentID := nullable(t.ParentID())

	// Resolve idempotency key: the virtual label takes precedence over
	// the constructor field, allowing callers to set it via --label.
	var idemKey any
	if val, hasVirtual := t.Labels().Get(domain.VirtualKeyIdempotency); hasVirtual {
		idemKey = val
	} else if t.IdempotencyKey() != "" {
		idemKey = t.IdempotencyKey()
	}

	err := sqlitex.Execute(r.conn,
		`INSERT INTO issues (issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{
				t.ID().String(), t.Role().String(), t.Title(), t.Description(), t.AcceptanceCriteria(),
				t.Priority().String(), t.State().String(), parentID, t.CreatedAt().Format(time.RFC3339Nano),
				idemKey, boolToInt(t.IsDeleted()),
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "create issue", Err: err}
	}

	// Save labels — skip virtual keys that are backed by columns.
	for k, v := range t.Labels().All() {
		if domain.IsVirtualLabelKey(k) {
			continue
		}
		if err := sqlitex.Execute(r.conn, `INSERT INTO labels (issue_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "create label", Err: err}
		}
	}

	// FTS sync.
	err = sqlitex.Execute(r.conn, `INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String(), t.Title(), t.Description(), t.AcceptanceCriteria()},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "fts insert", Err: err}
	}

	return nil
}

func (r *issueRepo) GetIssue(_ context.Context, id domain.ID, includeDeleted bool) (domain.Issue, error) {
	query := `SELECT issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM issues WHERE issue_id = ?`
	if !includeDeleted {
		query += ` AND deleted = 0`
	}

	t, err := scanIssueRow(r.conn, query, id.String())
	if err != nil {
		if isNotFound(err) {
			return domain.Issue{}, domain.ErrNotFound
		}
		return domain.Issue{}, &domain.DatabaseError{Op: "get issue", Err: err}
	}

	// Load labels.
	labels, err := r.loadLabels(id.String())
	if err != nil {
		return domain.Issue{}, err
	}

	// Inject the idempotency-key virtual label if the column is populated.
	if t.IdempotencyKey() != "" {
		lbl, _ := domain.NewLabel(domain.VirtualKeyIdempotency, t.IdempotencyKey())
		labels = labels.Set(lbl)
	}

	t = t.WithLabels(labels)

	return t, nil
}

func (r *issueRepo) UpdateIssue(_ context.Context, t domain.Issue) error {
	parentID := nullable(t.ParentID())

	// Sync the idempotency-key virtual label to the column. If the label
	// is present, use its value; otherwise, derive from IdempotencyKey().
	var idemKey any
	if val, hasVirtual := t.Labels().Get(domain.VirtualKeyIdempotency); hasVirtual {
		idemKey = val
	} else if t.IdempotencyKey() != "" {
		idemKey = t.IdempotencyKey()
	}

	err := sqlitex.Execute(r.conn,
		`UPDATE issues SET title = ?, description = ?, acceptance_criteria = ?, priority = ?, state = ?, parent_id = ?, deleted = ?, idempotency_key = ? WHERE issue_id = ?`,
		&sqlitex.ExecOptions{
			Args: []any{
				t.Title(), t.Description(), t.AcceptanceCriteria(), t.Priority().String(),
				t.State().String(), parentID, boolToInt(t.IsDeleted()), idemKey, t.ID().String(),
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "update issue", Err: err}
	}

	// Replace labels — skip virtual keys that are backed by columns.
	if err := sqlitex.Execute(r.conn, `DELETE FROM labels WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	}); err != nil {
		return &domain.DatabaseError{Op: "delete labels", Err: err}
	}
	for k, v := range t.Labels().All() {
		if domain.IsVirtualLabelKey(k) {
			continue
		}
		if err := sqlitex.Execute(r.conn, `INSERT INTO labels (issue_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "update label", Err: err}
		}
	}

	// FTS sync — delete old entry and insert updated one.
	if err := sqlitex.Execute(r.conn, `DELETE FROM issues_fts WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	}); err != nil {
		return &domain.DatabaseError{Op: "delete FTS entry", Err: err}
	}
	if err := sqlitex.Execute(r.conn, `INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String(), t.Title(), t.Description(), t.AcceptanceCriteria()},
	}); err != nil {
		return &domain.DatabaseError{Op: "insert FTS entry", Err: err}
	}

	return nil
}

func (r *issueRepo) ListIssues(_ context.Context, filter driven.IssueFilter, orderBy driven.IssueOrderBy, direction driven.SortDirection, limit int) ([]driven.IssueListItem, bool, error) {
	limit = driven.NormalizeLimit(limit)
	where, args := buildIssueWhere(filter)

	orderClause := issueOrderClause(orderBy, direction)

	// Fetch limit+1 rows to detect whether more results exist.
	fetchLimit := limit + 1
	if limit < 0 {
		fetchLimit = -1 // unlimited
	}
	query := `SELECT t.issue_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted, t.parent_id,
		(EXISTS(SELECT 1 FROM relationships r JOIN issues b ON r.target_id = b.issue_id
			WHERE r.source_id = t.issue_id AND r.rel_type = 'blocked_by'
			AND b.state != 'closed' AND b.deleted = 0)
		OR EXISTS(
			WITH RECURSIVE anc(aid) AS (
				SELECT t.parent_id
				UNION ALL
				SELECT p.parent_id FROM issues p JOIN anc a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT 1 FROM anc a
			JOIN issues anc_i ON anc_i.issue_id = a.aid
			WHERE EXISTS(SELECT 1 FROM relationships r JOIN issues b ON r.target_id = b.issue_id
				WHERE r.source_id = anc_i.issue_id AND r.rel_type = 'blocked_by'
				AND b.state != 'closed' AND b.deleted = 0)
		)) AS is_blocked,
		COALESCE((SELECT GROUP_CONCAT(r.target_id, ',')
			FROM relationships r JOIN issues b ON r.target_id = b.issue_id
			WHERE r.source_id = t.issue_id AND r.rel_type = 'blocked_by'
			AND b.state != 'closed' AND b.deleted = 0
		), '') AS blocker_ids,
		parent.created_at AS parent_created_at
		FROM issues t` + issueParentJoin() + ` ` + where + orderClause
	if fetchLimit > 0 {
		query += ` LIMIT ?`
		args = append(args, fetchLimit)
	}

	var items []driven.IssueListItem
	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := domain.ParseID(stmt.ColumnText(0))
			role, _ := domain.ParseRole(stmt.ColumnText(1))
			state, _ := domain.ParseState(stmt.ColumnText(2))
			priority, _ := domain.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))
			parentID, _ := domain.ParseID(stmt.ColumnText(7))
			parentCreatedAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(10))

			item := driven.IssueListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), ParentID: parentID,
				ParentCreatedAt: parentCreatedAt,
				CreatedAt:       createdAt,
				IsDeleted:       stmt.ColumnInt(6) != 0,
				IsBlocked:       stmt.ColumnInt(8) != 0,
			}

			// Parse comma-separated blocker IDs from the GROUP_CONCAT subquery.
			if raw := stmt.ColumnText(9); raw != "" {
				item.BlockerIDs = parseIDList(raw)
			}

			items = append(items, item)
			return nil
		},
	})
	if err != nil {
		return nil, false, &domain.DatabaseError{Op: "list issues", Err: err}
	}

	hasMore := limit > 0 && len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func (r *issueRepo) SearchIssues(_ context.Context, query string, filter driven.IssueFilter, orderBy driven.IssueOrderBy, direction driven.SortDirection, limit int) ([]driven.IssueListItem, bool, error) {
	limit = driven.NormalizeLimit(limit)
	where, args := buildIssueWhere(filter)

	ftsWhere := ` AND t.issue_id IN (SELECT issue_id FROM issues_fts WHERE issues_fts MATCH ?)`
	args = append(args, sanitizeFTS5Query(query))

	orderClause := issueOrderClause(orderBy, direction)
	fetchLimit := limit + 1
	if limit < 0 {
		fetchLimit = -1
	}
	selectQuery := `SELECT t.issue_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted, t.parent_id,
		(EXISTS(SELECT 1 FROM relationships r JOIN issues b ON r.target_id = b.issue_id
			WHERE r.source_id = t.issue_id AND r.rel_type = 'blocked_by'
			AND b.state != 'closed' AND b.deleted = 0)
		OR EXISTS(
			WITH RECURSIVE anc(aid) AS (
				SELECT t.parent_id
				UNION ALL
				SELECT p.parent_id FROM issues p JOIN anc a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT 1 FROM anc a
			JOIN issues anc_i ON anc_i.issue_id = a.aid
			WHERE EXISTS(SELECT 1 FROM relationships r JOIN issues b ON r.target_id = b.issue_id
				WHERE r.source_id = anc_i.issue_id AND r.rel_type = 'blocked_by'
				AND b.state != 'closed' AND b.deleted = 0)
		)) AS is_blocked,
		COALESCE((SELECT GROUP_CONCAT(r.target_id, ',')
			FROM relationships r JOIN issues b ON r.target_id = b.issue_id
			WHERE r.source_id = t.issue_id AND r.rel_type = 'blocked_by'
			AND b.state != 'closed' AND b.deleted = 0
		), '') AS blocker_ids,
		parent.created_at AS parent_created_at
		FROM issues t` + issueParentJoin() + ` ` + where + ftsWhere + orderClause
	if fetchLimit > 0 {
		selectQuery += ` LIMIT ?`
		args = append(args, fetchLimit)
	}

	var items []driven.IssueListItem
	err := sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := domain.ParseID(stmt.ColumnText(0))
			role, _ := domain.ParseRole(stmt.ColumnText(1))
			state, _ := domain.ParseState(stmt.ColumnText(2))
			priority, _ := domain.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))
			parentID, _ := domain.ParseID(stmt.ColumnText(7))
			parentCreatedAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(10))

			item := driven.IssueListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), ParentID: parentID,
				ParentCreatedAt: parentCreatedAt,
				CreatedAt:       createdAt,
				IsDeleted:       stmt.ColumnInt(6) != 0,
				IsBlocked:       stmt.ColumnInt(8) != 0,
			}
			if raw := stmt.ColumnText(9); raw != "" {
				item.BlockerIDs = parseIDList(raw)
			}
			items = append(items, item)
			return nil
		},
	})
	if err != nil {
		return nil, false, &domain.DatabaseError{Op: "search issues", Err: err}
	}

	hasMore := limit > 0 && len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func (r *issueRepo) GetChildStatuses(_ context.Context, epicID domain.ID) ([]domain.ChildStatus, error) {
	var children []domain.ChildStatus
	err := sqlitex.Execute(r.conn, `
		SELECT state,
			EXISTS(
				SELECT 1 FROM relationships r
				JOIN issues b ON r.target_id = b.issue_id
				WHERE r.source_id = c.issue_id
				AND r.rel_type = 'blocked_by'
				AND b.state != 'closed'
				AND b.deleted = 0
			) AS is_blocked
		FROM issues c
		WHERE c.parent_id = ? AND c.deleted = 0`, &sqlitex.ExecOptions{
		Args: []any{epicID.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			state, _ := domain.ParseState(stmt.ColumnText(0))
			children = append(children, domain.ChildStatus{
				State:     state,
				IsBlocked: stmt.ColumnInt(1) != 0,
			})
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get child statuses", Err: err}
	}
	return children, nil
}

func (r *issueRepo) GetDescendants(_ context.Context, epicID domain.ID) ([]domain.DescendantInfo, error) {
	return r.getDescendantsRecursive(epicID)
}

func (r *issueRepo) getDescendantsRecursive(parentID domain.ID) ([]domain.DescendantInfo, error) {
	type childInfo struct {
		id      domain.ID
		role    domain.Role
		claimed bool
		author  string
	}
	var childInfos []childInfo

	err := sqlitex.Execute(r.conn,
		`SELECT t.issue_id, t.role, COALESCE(c.author, '') as claim_author FROM issues t LEFT JOIN claims c ON t.issue_id = c.issue_id WHERE t.parent_id = ? AND t.deleted = 0`,
		&sqlitex.ExecOptions{
			Args: []any{parentID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				id, _ := domain.ParseID(stmt.ColumnText(0))
				role, _ := domain.ParseRole(stmt.ColumnText(1))
				claimAuthor := stmt.ColumnText(2)
				childInfos = append(childInfos, childInfo{id: id, role: role, claimed: claimAuthor != "", author: claimAuthor})
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get descendants", Err: err}
	}

	var descendants []domain.DescendantInfo
	for _, ci := range childInfos {
		descendants = append(descendants, domain.DescendantInfo{
			ID: ci.id, IsClaimed: ci.claimed, ClaimedBy: ci.author,
		})
		if ci.role == domain.RoleEpic {
			sub, err := r.getDescendantsRecursive(ci.id)
			if err != nil {
				return nil, err
			}
			descendants = append(descendants, sub...)
		}
	}

	return descendants, nil
}

func (r *issueRepo) HasChildren(_ context.Context, epicID domain.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues WHERE parent_id = ? AND deleted = 0`, epicID.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "has children", Err: err}
	}
	return count > 0, nil
}

func (r *issueRepo) GetAncestorStatuses(_ context.Context, id domain.ID) ([]domain.AncestorStatus, error) {
	var ancestors []domain.AncestorStatus
	current := id.String()

	for {
		var parentID string
		var found bool
		err := sqlitex.Execute(r.conn, `SELECT parent_id FROM issues WHERE issue_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
			Args: []any{current},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				if !stmt.ColumnIsNull(0) {
					parentID = stmt.ColumnText(0)
					found = parentID != ""
				}
				return nil
			},
		})
		if err != nil || !found {
			break
		}

		var stateStr string
		var stateFound bool
		var isBlocked bool
		err = sqlitex.Execute(r.conn, `SELECT state,
			EXISTS(SELECT 1 FROM relationships r JOIN issues b ON r.target_id = b.issue_id
				WHERE r.source_id = ? AND r.rel_type = 'blocked_by'
				AND b.state != 'closed' AND b.deleted = 0)
			FROM issues WHERE issue_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
			Args: []any{parentID, parentID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				stateStr = stmt.ColumnText(0)
				isBlocked = stmt.ColumnInt(1) != 0
				stateFound = true
				return nil
			},
		})
		if err != nil || !stateFound {
			break
		}

		state, _ := domain.ParseState(stateStr)
		ancestorID, _ := domain.ParseID(parentID)
		ancestors = append(ancestors, domain.AncestorStatus{ID: ancestorID, State: state, IsBlocked: isBlocked})
		current = parentID
	}

	return ancestors, nil
}

func (r *issueRepo) GetParentID(_ context.Context, id domain.ID) (domain.ID, error) {
	var parentID string
	var found bool
	var isNull bool

	err := sqlitex.Execute(r.conn, `SELECT parent_id FROM issues WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{id.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			isNull = stmt.ColumnIsNull(0)
			if !isNull {
				parentID = stmt.ColumnText(0)
			}
			return nil
		},
	})
	if err != nil {
		return domain.ID{}, &domain.DatabaseError{Op: "get parent ID", Err: err}
	}
	if !found {
		return domain.ID{}, domain.ErrNotFound
	}
	if isNull || parentID == "" {
		return domain.ID{}, nil
	}
	return domain.ParseID(parentID)
}

func (r *issueRepo) IssueIDExists(_ context.Context, id domain.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues WHERE issue_id = ?`, id.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "check issue exists", Err: err}
	}
	return count > 0, nil
}

func (r *issueRepo) ListDistinctLabels(_ context.Context) ([]domain.Label, error) {
	var lbls []domain.Label
	err := sqlitex.Execute(r.conn,
		`SELECT DISTINCT d.key, d.value FROM labels d
		 JOIN issues t ON d.issue_id = t.issue_id
		 WHERE t.deleted = 0
		 ORDER BY d.key, d.value`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				lbl, err := domain.NewLabel(stmt.ColumnText(0), stmt.ColumnText(1))
				if err != nil {
					return err
				}
				lbls = append(lbls, lbl)
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list distinct labels", Err: err}
	}

	// Include virtual labels from the idempotency_key column.
	err = sqlitex.Execute(r.conn,
		`SELECT DISTINCT idempotency_key FROM issues WHERE deleted = 0 AND idempotency_key IS NOT NULL ORDER BY idempotency_key`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				lbl, lErr := domain.NewLabel(domain.VirtualKeyIdempotency, stmt.ColumnText(0))
				if lErr != nil {
					return lErr
				}
				lbls = append(lbls, lbl)
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list distinct virtual labels", Err: err}
	}

	return lbls, nil
}

func (r *issueRepo) GetIssueByIdempotencyKey(_ context.Context, key string) (domain.Issue, error) {
	t, err := scanIssueRow(r.conn,
		`SELECT issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM issues WHERE idempotency_key = ?`, key)
	if err != nil {
		if isNotFound(err) {
			return domain.Issue{}, domain.ErrNotFound
		}
		return domain.Issue{}, &domain.DatabaseError{Op: "get by idempotency key", Err: err}
	}
	return t, nil
}

// GetIssueSummary returns aggregate issue counts by state and computed
// readiness/blocked status in a single query. The ready and blocked
// sub-selects mirror the filter logic in buildIssueWhere.
func (r *issueRepo) GetIssueSummary(_ context.Context) (driven.IssueSummary, error) {
	var s driven.IssueSummary

	// Claims are transient local bookkeeping; a claimed issue retains its open
	// primary state in the issues table. The query therefore counts only the
	// three canonical lifecycle states.
	query := `SELECT
		COALESCE(SUM(CASE WHEN state = 'open' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN state = 'deferred' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN state = 'closed' THEN 1 ELSE 0 END), 0),
		-- Ready: open + no active claim + (task OR childless epic) + no unresolved blockers + no blocked/deferred ancestors
		COALESCE(SUM(CASE WHEN state = 'open'
			AND NOT EXISTS (
				SELECT 1 FROM claims cl
				WHERE cl.issue_id = t.issue_id
				  AND datetime(cl.last_activity, '+' || (cl.stale_threshold / 1000000000) || ' seconds') > datetime('now')
			)
			AND (role = 'task' OR NOT EXISTS (
				SELECT 1 FROM issues c WHERE c.parent_id = t.issue_id AND c.deleted = 0
			))
			AND NOT EXISTS (
				SELECT 1 FROM relationships r
				JOIN issues bt ON r.target_id = bt.issue_id
				WHERE r.source_id = t.issue_id
				  AND r.rel_type = 'blocked_by'
				  AND bt.deleted = 0
				  AND bt.state != 'closed'
			)
			AND NOT EXISTS (
				WITH RECURSIVE ancestors(aid) AS (
					SELECT t.parent_id
					UNION ALL
					SELECT p.parent_id FROM issues p JOIN ancestors a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
				)
				SELECT 1 FROM ancestors a
				JOIN issues anc ON anc.issue_id = a.aid
				WHERE anc.state = 'deferred'
				   OR EXISTS (
					SELECT 1 FROM relationships r
					JOIN issues bt ON r.target_id = bt.issue_id
					WHERE r.source_id = anc.issue_id
					  AND r.rel_type = 'blocked_by'
					  AND bt.deleted = 0
					  AND bt.state != 'closed'
				)
			)
		THEN 1 ELSE 0 END), 0),
		-- Blocked: direct unresolved blocker OR blocked ancestor
		COALESCE(SUM(CASE WHEN (
			EXISTS (
				SELECT 1 FROM relationships r
				JOIN issues bt ON r.target_id = bt.issue_id
				WHERE r.source_id = t.issue_id
				  AND r.rel_type = 'blocked_by'
				  AND bt.deleted = 0
				  AND bt.state != 'closed'
			)
			OR EXISTS (
				WITH RECURSIVE ancestors(aid) AS (
					SELECT t.parent_id
					UNION ALL
					SELECT p.parent_id FROM issues p JOIN ancestors a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
				)
				SELECT 1 FROM ancestors a
				JOIN issues anc ON anc.issue_id = a.aid
				WHERE EXISTS (
					SELECT 1 FROM relationships r
					JOIN issues bt ON r.target_id = bt.issue_id
					WHERE r.source_id = anc.issue_id
					  AND r.rel_type = 'blocked_by'
					  AND bt.deleted = 0
					  AND bt.state != 'closed'
				)
			)
		) AND state != 'closed' THEN 1 ELSE 0 END), 0)
	FROM issues t WHERE deleted = 0`

	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			s.Open = stmt.ColumnInt(0)
			s.Deferred = stmt.ColumnInt(1)
			s.Closed = stmt.ColumnInt(2)
			s.Ready = stmt.ColumnInt(3)
			s.Blocked = stmt.ColumnInt(4)
			return nil
		},
	})
	if err != nil {
		return driven.IssueSummary{}, &domain.DatabaseError{Op: "get issue summary", Err: err}
	}
	return s, nil
}

func (r *issueRepo) loadLabels(issueID string) (domain.LabelSet, error) {
	fs := domain.NewLabelSet()
	err := sqlitex.Execute(r.conn, `SELECT key, value FROM labels WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{issueID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			f, _ := domain.NewLabel(stmt.ColumnText(0), stmt.ColumnText(1))
			fs = fs.Set(f)
			return nil
		},
	})
	if err != nil {
		return domain.NewLabelSet(), &domain.DatabaseError{Op: "load labels", Err: err}
	}
	return fs, nil
}

// --- CommentRepository ---

type commentRepo struct{ conn *sqlite.Conn }

func (r *commentRepo) CreateComment(_ context.Context, n domain.Comment) (int64, error) {
	err := sqlitex.Execute(r.conn, `INSERT INTO comments (issue_id, author, created_at, body) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{n.IssueID().String(), n.Author().String(), n.CreatedAt().Format(time.RFC3339Nano), n.Body()},
	})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "create comment", Err: err}
	}

	commentID := r.conn.LastInsertRowID()

	// FTS sync.
	if err := sqlitex.Execute(r.conn, `INSERT INTO comments_fts (comment_id, body) VALUES (?, ?)`, &sqlitex.ExecOptions{
		Args: []any{commentID, n.Body()},
	}); err != nil {
		return 0, &domain.DatabaseError{Op: "insert comment FTS entry", Err: err}
	}

	return commentID, nil
}

func (r *commentRepo) GetComment(_ context.Context, id int64) (domain.Comment, error) {
	var result domain.Comment
	var found bool

	err := sqlitex.Execute(r.conn, `SELECT issue_id, author, created_at, body FROM comments WHERE comment_id = ?`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			issueID, _ := domain.ParseID(stmt.ColumnText(0))
			author, _ := domain.NewAuthor(stmt.ColumnText(1))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(2))

			var err error
			result, err = domain.NewComment(domain.NewCommentParams{
				ID: id, IssueID: issueID, Author: author, CreatedAt: createdAt, Body: stmt.ColumnText(3),
			})
			return err
		},
	})
	if err != nil {
		return domain.Comment{}, &domain.DatabaseError{Op: "get comment", Err: err}
	}
	if !found {
		return domain.Comment{}, domain.ErrNotFound
	}
	return result, nil
}

func (r *commentRepo) ListComments(_ context.Context, issueID domain.ID, filter driven.CommentFilter, limit int) ([]domain.Comment, bool, error) {
	limit = driven.NormalizeLimit(limit)

	where := `WHERE issue_id = ?`
	args := []any{issueID.String()}

	if !filter.Author.IsZero() {
		where += ` AND author = ?`
		args = append(args, filter.Author.String())
	}
	if !filter.CreatedAfter.IsZero() {
		where += ` AND created_at > ?`
		args = append(args, filter.CreatedAfter.Format(time.RFC3339Nano))
	}
	if filter.AfterCommentID > 0 {
		where += ` AND comment_id > ?`
		args = append(args, filter.AfterCommentID)
	}

	// Fetch limit+1 rows to detect whether more results exist beyond the limit.
	fetchLimit := limit + 1
	if limit < 0 {
		fetchLimit = -1
	}

	query := `SELECT comment_id, issue_id, author, created_at, body FROM comments ` + where + ` ORDER BY comment_id LIMIT ?`
	args = append(args, fetchLimit)

	var comments []domain.Comment
	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := domain.NewComment(domain.NewCommentParams{
				ID: stmt.ColumnInt64(0), IssueID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			comments = append(comments, n)
			return nil
		},
	})
	if err != nil {
		return nil, false, &domain.DatabaseError{Op: "list comments", Err: err}
	}

	hasMore := limit > 0 && len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}

	return comments, hasMore, nil
}

func (r *commentRepo) SearchComments(_ context.Context, query string, filter driven.CommentFilter, limit int) ([]domain.Comment, bool, error) {
	limit = driven.NormalizeLimit(limit)

	where := `WHERE c.comment_id IN (SELECT comment_id FROM comments_fts WHERE comments_fts MATCH ?)`
	args := []any{sanitizeFTS5Query(query)}

	scopeSQL, scopeArgs := buildCommentIssueScope(filter)
	if scopeSQL != "" {
		where += ` AND ` + scopeSQL
		args = append(args, scopeArgs...)
	}

	authorSQL, authorArgs := buildCommentAuthorFilter(filter)
	if authorSQL != "" {
		where += ` AND ` + authorSQL
		args = append(args, authorArgs...)
	}

	// Fetch limit+1 rows to detect whether more results exist beyond the limit.
	fetchLimit := limit + 1
	if limit < 0 {
		fetchLimit = -1
	}

	selectQuery := `SELECT c.comment_id, c.issue_id, c.author, c.created_at, c.body FROM comments c ` + where + ` ORDER BY c.comment_id LIMIT ?`
	args = append(args, fetchLimit)

	var comments []domain.Comment
	err := sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := domain.NewComment(domain.NewCommentParams{
				ID: stmt.ColumnInt64(0), IssueID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			comments = append(comments, n)
			return nil
		},
	})
	if err != nil {
		return nil, false, &domain.DatabaseError{Op: "search comments", Err: err}
	}

	hasMore := limit > 0 && len(comments) > limit
	if hasMore {
		comments = comments[:limit]
	}

	return comments, hasMore, nil
}

// buildCommentIssueScope constructs a SQL condition that scopes comments to
// issues matching any of the provided issue-level criteria. The criteria are
// OR'd together — a comment is included if its issue matches any scope.
// Returns empty string if no issue scoping is specified.
func buildCommentIssueScope(filter driven.CommentFilter) (string, []any) {
	var conditions []string
	var args []any

	// Direct issue ID.
	if !filter.IssueID.IsZero() {
		conditions = append(conditions, `c.issue_id = ?`)
		args = append(args, filter.IssueID.String())
	}

	// Multiple issue IDs.
	for _, id := range filter.IssueIDs {
		conditions = append(conditions, `c.issue_id = ?`)
		args = append(args, id.String())
	}

	// Parent scope: comments on direct children of the specified parent.
	for _, parentID := range filter.ParentIDs {
		conditions = append(conditions, `c.issue_id IN (SELECT issue_id FROM issues WHERE parent_id = ?)`)
		args = append(args, parentID.String())
		// Also include the parent itself.
		conditions = append(conditions, `c.issue_id = ?`)
		args = append(args, parentID.String())
	}

	// Tree scope: comments on all issues in the subtree.
	for _, treeID := range filter.TreeIDs {
		conditions = append(conditions, `c.issue_id IN (
			WITH RECURSIVE tree(tid) AS (
				SELECT ? UNION ALL
				SELECT i.issue_id FROM issues i JOIN tree t ON i.parent_id = t.tid
			) SELECT tid FROM tree
		)`)
		args = append(args, treeID.String())
	}

	// Label scope: comments on issues with matching labels.
	for _, lf := range filter.LabelFilters {
		if domain.IsVirtualLabelKey(lf.Key) {
			cond, condArgs := buildVirtualLabelCondition(lf)
			// Rewrite column references from t. to issues table.
			cond = strings.ReplaceAll(cond, "t.", "i.")
			conditions = append(conditions, `c.issue_id IN (SELECT i.issue_id FROM issues i WHERE `+cond+`)`)
			args = append(args, condArgs...)
		} else if lf.Value == "" {
			conditions = append(conditions, `c.issue_id IN (SELECT f.issue_id FROM labels f WHERE f.key = ?)`)
			args = append(args, lf.Key)
		} else {
			conditions = append(conditions, `c.issue_id IN (SELECT f.issue_id FROM labels f WHERE f.key = ? AND f.value = ?)`)
			args = append(args, lf.Key, lf.Value)
		}
	}

	// Follow-refs: expand scope to include referenced issues.
	// This is applied after the other scopes by adding a UNION.
	if filter.FollowRefs && len(conditions) > 0 {
		// The original scope selects issue IDs.
		innerScope := strings.Join(conditions, " OR ")
		// Add referenced issues.
		conditions = append(conditions,
			`c.issue_id IN (SELECT r.target_id FROM relationships r WHERE r.source_id IN (SELECT i2.issue_id FROM issues i2 JOIN comments c2 ON c2.issue_id = i2.issue_id WHERE `+innerScope+`))`)
		// Note: args for the inner scope are duplicated, but we re-append them.
		args = append(args, args...)
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return `(` + strings.Join(conditions, " OR ") + `)`, args
}

// buildCommentAuthorFilter constructs a SQL condition for filtering comments
// by author. Returns empty string if no author filter is specified.
func buildCommentAuthorFilter(filter driven.CommentFilter) (string, []any) {
	var conditions []string
	var args []any

	if !filter.Author.IsZero() {
		conditions = append(conditions, `c.author = ?`)
		args = append(args, filter.Author.String())
	}

	for _, a := range filter.Authors {
		conditions = append(conditions, `c.author = ?`)
		args = append(args, a.String())
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return `(` + strings.Join(conditions, " OR ") + `)`, args
}

// --- ClaimRepository ---

type claimRepo struct{ conn *sqlite.Conn }

func (r *claimRepo) CreateClaim(_ context.Context, c domain.Claim) error {
	// The schema still uses last_activity and stale_threshold columns;
	// populate them from the new claimedAt/staleAt fields.
	claimDuration := c.StaleAt().Sub(c.ClaimedAt())
	err := sqlitex.Execute(r.conn, `INSERT OR REPLACE INTO claims (claim_sha512, issue_id, author, stale_threshold, last_activity) VALUES (?, ?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{c.ID(), c.IssueID().String(), c.Author().String(), int64(claimDuration), c.ClaimedAt().Format(time.RFC3339Nano)},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "create claim", Err: err}
	}
	return nil
}

func (r *claimRepo) GetClaimByIssue(_ context.Context, issueID domain.ID) (domain.Claim, error) {
	return r.scanClaim(`SELECT claim_sha512, issue_id, author, stale_threshold, last_activity FROM claims WHERE issue_id = ?`, issueID.String())
}

// resolveClaimID maps a claim identifier to the key stored in the claims
// table. New claims are stored as SHA-512 hashes (128 hex chars); legacy
// claims may still be stored as plaintext (26 or 32 chars). This function
// first checks for a direct match (supporting both legacy plaintext and
// hash IDs), then falls back to hashing and looking up by hash.
func (r *claimRepo) resolveClaimID(claimID string) string {
	// If the ID is already a 128-char hash, use it directly.
	if len(claimID) == 128 {
		return claimID
	}

	// Check if the plaintext ID exists directly (legacy claim).
	var found bool
	_ = sqlitex.Execute(r.conn, `SELECT 1 FROM claims WHERE claim_sha512 = ?`, &sqlitex.ExecOptions{
		Args:       []any{claimID},
		ResultFunc: func(_ *sqlite.Stmt) error { found = true; return nil },
	})
	if found {
		return claimID
	}

	// Otherwise hash it to match a new-style stored hash.
	return domain.HashClaimID(claimID)
}

func (r *claimRepo) GetClaimByID(_ context.Context, claimID string) (domain.Claim, error) {
	resolvedID := r.resolveClaimID(claimID)
	return r.scanClaim(`SELECT claim_sha512, issue_id, author, stale_threshold, last_activity FROM claims WHERE claim_sha512 = ?`, resolvedID)
}

func (r *claimRepo) InvalidateClaim(_ context.Context, claimID string) error {
	// claimID may be a plaintext token or a hash — resolve to hash.
	resolvedID := r.resolveClaimID(claimID)
	err := sqlitex.Execute(r.conn, `DELETE FROM claims WHERE claim_sha512 = ?`, &sqlitex.ExecOptions{
		Args: []any{resolvedID},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "invalidate claim", Err: err}
	}
	if r.conn.Changes() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *claimRepo) UpdateClaimStaleAt(_ context.Context, claimID string, staleAt time.Time) error {
	resolvedID := r.resolveClaimID(claimID)

	// The schema still stores (last_activity, stale_threshold). Compute the
	// new stale_threshold as staleAt − last_activity using SQL's strftime,
	// which handles both RFC3339 and SQLite datetime('now') formats.
	// The result is in nanoseconds (seconds × 1e9) to match the Go
	// time.Duration convention used by the stale_threshold column.
	err := sqlitex.Execute(r.conn,
		`UPDATE claims SET stale_threshold = (? - CAST(strftime('%s', last_activity) AS INTEGER)) * 1000000000 WHERE claim_sha512 = ?`,
		&sqlitex.ExecOptions{
			Args: []any{staleAt.Unix(), resolvedID},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "update claim stale_at", Err: err}
	}
	if r.conn.Changes() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// DeleteExpiredClaims removes all claim rows whose stale-at timestamp (computed
// as last_activity + stale_threshold) is on or before now. Returns the number
// of rows deleted. Active claims are left untouched.
func (r *claimRepo) DeleteExpiredClaims(_ context.Context, now time.Time) (int, error) {
	// The stale_threshold column stores nanoseconds; SQLite's strftime('%s', …)
	// works in whole seconds. Integer division truncates sub-second remainders,
	// so a claim may expire up to ~1 second earlier in SQL than in Go's
	// domain.Claim.IsStale. This is acceptable because claim thresholds are
	// always measured in hours (minimum 1h, default 2h, maximum 24h).
	err := sqlitex.Execute(r.conn,
		`DELETE FROM claims
		 WHERE CAST(strftime('%s', last_activity) AS INTEGER) + stale_threshold / 1000000000
		       <= CAST(strftime('%s', ?) AS INTEGER)`,
		&sqlitex.ExecOptions{
			Args: []any{now.UTC().Format(time.RFC3339Nano)},
		},
	)
	if err != nil {
		return 0, &domain.DatabaseError{Op: "delete expired claims", Err: err}
	}
	return r.conn.Changes(), nil
}

func (r *claimRepo) ListStaleClaims(_ context.Context, now time.Time) ([]domain.Claim, error) {
	var stale []domain.Claim
	err := sqlitex.Execute(r.conn, `SELECT claim_sha512, issue_id, author, stale_threshold, last_activity FROM claims`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(2))
			claimedAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			staleAt := claimedAt.Add(time.Duration(stmt.ColumnInt64(3)))
			c := domain.ReconstructClaim(stmt.ColumnText(0), tid, author, claimedAt, staleAt)
			if c.IsStale(now) {
				stale = append(stale, c)
			}
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list stale claims", Err: err}
	}
	return stale, nil
}

func (r *claimRepo) ListActiveClaims(_ context.Context, now time.Time) ([]domain.Claim, error) {
	var active []domain.Claim
	err := sqlitex.Execute(r.conn, `SELECT claim_sha512, issue_id, author, stale_threshold, last_activity FROM claims`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(2))
			claimedAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			staleAt := claimedAt.Add(time.Duration(stmt.ColumnInt64(3)))
			c := domain.ReconstructClaim(stmt.ColumnText(0), tid, author, claimedAt, staleAt)
			if !c.IsStale(now) {
				active = append(active, c)
			}
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list active claims", Err: err}
	}
	return active, nil
}

func (r *claimRepo) scanClaim(query string, args ...any) (domain.Claim, error) {
	var result domain.Claim
	var found bool

	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(2))
			claimedAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			staleAt := claimedAt.Add(time.Duration(stmt.ColumnInt64(3)))
			result = domain.ReconstructClaim(stmt.ColumnText(0), tid, author, claimedAt, staleAt)
			return nil
		},
	})
	if err != nil {
		return domain.Claim{}, &domain.DatabaseError{Op: "get claim", Err: err}
	}
	if !found {
		return domain.Claim{}, domain.ErrNotFound
	}
	return result, nil
}

// --- RelationshipRepository ---

type relRepo struct{ conn *sqlite.Conn }

func (r *relRepo) CreateRelationship(_ context.Context, rel domain.Relationship) (bool, error) {
	// For symmetric types, check if the reverse direction already exists.
	if rel.Type().IsSymmetric() {
		var exists bool
		err := sqlitex.Execute(r.conn,
			`SELECT 1 FROM relationships WHERE source_id = ? AND target_id = ? AND rel_type = ?`,
			&sqlitex.ExecOptions{
				Args: []any{rel.TargetID().String(), rel.SourceID().String(), rel.Type().String()},
				ResultFunc: func(_ *sqlite.Stmt) error {
					exists = true
					return nil
				},
			})
		if err != nil {
			return false, &domain.DatabaseError{Op: "create relationship", Err: err}
		}
		if exists {
			return false, nil // Reverse direction exists — idempotent.
		}
	}

	err := sqlitex.Execute(r.conn, `INSERT OR IGNORE INTO relationships (source_id, target_id, rel_type) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{rel.SourceID().String(), rel.TargetID().String(), rel.Type().String()},
	})
	if err != nil {
		return false, &domain.DatabaseError{Op: "create relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) DeleteRelationship(_ context.Context, sourceID, targetID domain.ID, relType domain.RelationType) (bool, error) {
	query := `DELETE FROM relationships WHERE source_id = ? AND target_id = ? AND rel_type = ?`
	if relType.IsSymmetric() {
		// For symmetric types, delete whichever direction is stored.
		query = `DELETE FROM relationships WHERE rel_type = ? AND ((source_id = ? AND target_id = ?) OR (source_id = ? AND target_id = ?))`
	}

	var args []any
	if relType.IsSymmetric() {
		args = []any{relType.String(), sourceID.String(), targetID.String(), targetID.String(), sourceID.String()}
	} else {
		args = []any{sourceID.String(), targetID.String(), relType.String()}
	}

	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{Args: args})
	if err != nil {
		return false, &domain.DatabaseError{Op: "delete relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) ListRelationships(_ context.Context, issueID domain.ID) ([]domain.Relationship, error) {
	var rels []domain.Relationship
	id := issueID.String()
	err := sqlitex.Execute(r.conn, `SELECT source_id, target_id, rel_type FROM relationships WHERE source_id = ? OR target_id = ?`, &sqlitex.ExecOptions{
		Args: []any{id, id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			src, _ := domain.ParseID(stmt.ColumnText(0))
			tgt, _ := domain.ParseID(stmt.ColumnText(1))
			rt, _ := domain.ParseRelationType(stmt.ColumnText(2))

			// For symmetric types where this issue is the target, present the
			// relationship from this issue's perspective (swap source/target).
			if rt.IsSymmetric() && tgt == issueID {
				rel, _ := domain.NewRelationship(issueID, src, rt)
				rels = append(rels, rel)
			} else {
				rel, _ := domain.NewRelationship(src, tgt, rt)
				rels = append(rels, rel)
			}
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list relationships", Err: err}
	}
	return rels, nil
}

func (r *relRepo) GetBlockerStatuses(_ context.Context, issueID domain.ID) ([]domain.BlockerStatus, error) {
	var statuses []domain.BlockerStatus
	err := sqlitex.Execute(r.conn,
		`SELECT t.state, t.deleted, t.role, t.issue_id FROM relationships r JOIN issues t ON r.target_id = t.issue_id WHERE r.source_id = ? AND r.rel_type = 'blocked_by'`,
		&sqlitex.ExecOptions{
			Args: []any{issueID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				state, _ := domain.ParseState(stmt.ColumnText(0))
				deleted := stmt.ColumnInt(1) != 0

				statuses = append(statuses, domain.BlockerStatus{
					IsClosed:  state == domain.StateClosed,
					IsDeleted: deleted,
				})
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get blocker statuses", Err: err}
	}
	return statuses, nil
}

// --- HistoryRepository ---

type histRepo struct{ conn *sqlite.Conn }

func (r *histRepo) AppendHistory(_ context.Context, entry history.Entry) (int64, error) {
	changesJSON, _ := json.Marshal(entry.Changes())

	err := sqlitex.Execute(r.conn,
		`INSERT INTO history (issue_id, revision, author, timestamp, event_type, changes) VALUES (?, ?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{
				entry.IssueID().String(), entry.Revision(), entry.Author().String(),
				entry.Timestamp().Format(time.RFC3339Nano), entry.EventType().String(), string(changesJSON),
			},
		})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "append history", Err: err}
	}
	return r.conn.LastInsertRowID(), nil
}

func (r *histRepo) ListHistory(_ context.Context, issueID domain.ID, filter driven.HistoryFilter, limit int) ([]history.Entry, bool, error) {
	limit = driven.NormalizeLimit(limit)

	where := `WHERE issue_id = ?`
	args := []any{issueID.String()}

	if !filter.Author.IsZero() {
		where += ` AND author = ?`
		args = append(args, filter.Author.String())
	}
	if !filter.After.IsZero() {
		where += ` AND timestamp > ?`
		args = append(args, filter.After.Format(time.RFC3339Nano))
	}
	if !filter.Before.IsZero() {
		where += ` AND timestamp < ?`
		args = append(args, filter.Before.Format(time.RFC3339Nano))
	}

	// Fetch limit+1 rows to detect whether more results exist beyond the limit.
	fetchLimit := limit + 1
	if limit < 0 {
		fetchLimit = -1
	}

	query := `SELECT entry_id, issue_id, revision, author, timestamp, event_type, changes FROM history ` + where + ` ORDER BY revision LIMIT ?`
	args = append(args, fetchLimit)

	var entries []history.Entry
	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := domain.ParseID(stmt.ColumnText(1))
			author, _ := domain.NewAuthor(stmt.ColumnText(3))
			ts, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			eventType, _ := history.ParseEventType(stmt.ColumnText(5))

			var changes []history.FieldChange
			_ = json.Unmarshal([]byte(stmt.ColumnText(6)), &changes)

			entries = append(entries, history.NewEntry(history.NewEntryParams{
				ID: stmt.ColumnInt64(0), IssueID: tid, Revision: stmt.ColumnInt(2), Author: author,
				Timestamp: ts, EventType: eventType, Changes: changes,
			}))
			return nil
		},
	})
	if err != nil {
		return nil, false, &domain.DatabaseError{Op: "list history", Err: err}
	}

	hasMore := limit > 0 && len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	return entries, hasMore, nil
}

func (r *histRepo) CountHistory(_ context.Context, issueID domain.ID) (int, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM history WHERE issue_id = ?`, issueID.String())
	if err != nil {
		return 0, &domain.DatabaseError{Op: "count history", Err: err}
	}
	return count, nil
}

func (r *histRepo) GetLatestHistory(_ context.Context, issueID domain.ID) (history.Entry, error) {
	var result history.Entry
	var found bool

	err := sqlitex.Execute(r.conn,
		`SELECT entry_id, issue_id, revision, author, timestamp, event_type, changes FROM history WHERE issue_id = ? ORDER BY revision DESC LIMIT 1`,
		&sqlitex.ExecOptions{
			Args: []any{issueID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				tid, _ := domain.ParseID(stmt.ColumnText(1))
				author, _ := domain.NewAuthor(stmt.ColumnText(3))
				ts, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
				eventType, _ := history.ParseEventType(stmt.ColumnText(5))

				var changes []history.FieldChange
				_ = json.Unmarshal([]byte(stmt.ColumnText(6)), &changes)

				result = history.NewEntry(history.NewEntryParams{
					ID: stmt.ColumnInt64(0), IssueID: tid, Revision: stmt.ColumnInt(2), Author: author,
					Timestamp: ts, EventType: eventType, Changes: changes,
				})
				return nil
			},
		})
	if err != nil {
		return history.Entry{}, &domain.DatabaseError{Op: "get latest history", Err: err}
	}
	if !found {
		return history.Entry{}, domain.ErrNotFound
	}
	return result, nil
}

// --- Helper functions ---

// scanIssueRow executes a query expected to return a single issue row and
// scans it into a domain Issue.
func scanIssueRow(conn *sqlite.Conn, query string, args ...any) (domain.Issue, error) {
	var t domain.Issue
	var found bool

	err := sqlitex.Execute(conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true

			idStr := stmt.ColumnText(0)
			roleStr := stmt.ColumnText(1)
			title := stmt.ColumnText(2)
			desc := stmt.ColumnText(3)
			ac := stmt.ColumnText(4)
			priorityStr := stmt.ColumnText(5)
			stateStr := stmt.ColumnText(6)
			parentIDStr := ""
			if !stmt.ColumnIsNull(7) {
				parentIDStr = stmt.ColumnText(7)
			}
			createdAtStr := stmt.ColumnText(8)
			idemKey := ""
			if !stmt.ColumnIsNull(9) {
				idemKey = stmt.ColumnText(9)
			}
			deleted := stmt.ColumnInt(10)

			id, _ := domain.ParseID(idStr)
			priority, _ := domain.ParsePriority(priorityStr)
			state, _ := domain.ParseState(stateStr)
			createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

			var pid domain.ID
			if parentIDStr != "" {
				pid, _ = domain.ParseID(parentIDStr)
			}

			role, _ := domain.ParseRole(roleStr)

			switch role {
			case domain.RoleTask:
				t, _ = domain.NewTask(domain.NewTaskParams{
					ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
					Priority: priority, ParentID: pid, CreatedAt: createdAt,
					IdempotencyKey: idemKey,
				})
			case domain.RoleEpic:
				t, _ = domain.NewEpic(domain.NewEpicParams{
					ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
					Priority: priority, ParentID: pid, CreatedAt: createdAt,
					IdempotencyKey: idemKey,
				})
			}

			t = t.WithState(state)
			if deleted != 0 {
				t = t.WithDeleted()
			}

			return nil
		},
	})
	if err != nil {
		return domain.Issue{}, err
	}
	if !found {
		return domain.Issue{}, errNotFound
	}
	return t, nil
}

// queryInt executes a query that returns a single integer value.
func queryInt(conn *sqlite.Conn, query string, args ...any) (int, error) {
	val, err := sqlitex.ResultInt(conn.Prep(query))
	if err != nil && len(args) == 0 {
		return 0, err
	}

	// For parameterised queries, use Execute since Prep doesn't bind args.
	if len(args) > 0 {
		var result int
		err := sqlitex.Execute(conn, query, &sqlitex.ExecOptions{
			Args: args,
			ResultFunc: func(stmt *sqlite.Stmt) error {
				result = stmt.ColumnInt(0)
				return nil
			},
		})
		return result, err
	}

	return val, err
}

// nullable converts an domain.ID to a value suitable for SQL binding. Returns
// nil (which binds as NULL) for zero IDs, or the string representation
// otherwise.
func nullable(id domain.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// boolToInt converts a boolean to 0 or 1 for SQLite integer storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isNotFound reports whether err represents a "no rows" condition.
var errNotFound = fmt.Errorf("no rows")

func isNotFound(err error) bool {
	return err == errNotFound
}

func buildIssueWhere(filter driven.IssueFilter) (string, []any) {
	conditions := []string{"1=1"}
	var args []any

	if !filter.IncludeDeleted {
		conditions = append(conditions, "t.deleted = 0")
	}

	if len(filter.Roles) > 0 {
		placeholders := make([]string, len(filter.Roles))
		for i, r := range filter.Roles {
			placeholders[i] = "?"
			args = append(args, r.String())
		}
		conditions = append(conditions, fmt.Sprintf("t.role IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.States) > 0 {
		placeholders := make([]string, len(filter.States))
		for i, s := range filter.States {
			placeholders[i] = "?"
			args = append(args, s.String())
		}
		conditions = append(conditions, fmt.Sprintf("t.state IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.ExcludeClosed && len(filter.States) == 0 {
		conditions = append(conditions, "t.state != 'closed'")
	}

	if len(filter.ParentIDs) > 0 {
		placeholders := make([]string, len(filter.ParentIDs))
		for i, pid := range filter.ParentIDs {
			placeholders[i] = "?"
			args = append(args, pid.String())
		}
		conditions = append(conditions, fmt.Sprintf("t.parent_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if !filter.DescendantsOf.IsZero() {
		// Recursive CTE walks the parent_id chain to find all descendants.
		conditions = append(conditions, `t.issue_id IN (
			WITH RECURSIVE desc(tid) AS (
				SELECT issue_id FROM issues WHERE parent_id = ?
				UNION ALL
				SELECT c.issue_id FROM issues c JOIN desc d ON c.parent_id = d.tid
			)
			SELECT tid FROM desc
		)`)
		args = append(args, filter.DescendantsOf.String())
	}

	if !filter.AncestorsOf.IsZero() {
		// Recursive CTE walks up the parent chain to find all ancestors.
		conditions = append(conditions, `t.issue_id IN (
			WITH RECURSIVE anc(aid) AS (
				SELECT parent_id FROM issues WHERE issue_id = ?
				UNION ALL
				SELECT p.parent_id FROM issues p JOIN anc a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT aid FROM anc WHERE aid IS NOT NULL
		)`)
		args = append(args, filter.AncestorsOf.String())
	}

	for _, ff := range filter.LabelFilters {
		// Virtual labels are backed by columns on the issues table,
		// not the labels table — generate column-based SQL.
		if domain.IsVirtualLabelKey(ff.Key) {
			cond, condArgs := buildVirtualLabelCondition(ff)
			conditions = append(conditions, cond)
			args = append(args, condArgs...)
			continue
		}

		if ff.Negate {
			if ff.Value == "" {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM labels f WHERE f.issue_id = t.issue_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM labels f WHERE f.issue_id = t.issue_id AND f.key = ? AND f.value = ?)`)
				args = append(args, ff.Key, ff.Value)
				continue
			}
		} else {
			if ff.Value == "" {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM labels f WHERE f.issue_id = t.issue_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM labels f WHERE f.issue_id = t.issue_id AND f.key = ? AND f.value = ?)`)
				args = append(args, ff.Key, ff.Value)
				continue
			}
		}
		args = append(args, ff.Key)
	}

	if filter.Ready {
		// Ready means: correct state, no active (non-stale) claim, no
		// unresolved blockers, no deferred ancestors, and (for epics) no
		// children.
		//
		// State: all issues must be open.
		conditions = append(conditions, `t.state = 'open'`)

		// No active (non-stale) claim. Claimed issues remain open but are
		// not available for new claims until the existing claim expires.
		// The stale_threshold is stored in nanoseconds; integer division to
		// seconds truncates sub-second remainders, which is acceptable
		// because claim thresholds are always measured in hours.
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM claims c
			WHERE c.issue_id = t.issue_id
			  AND datetime(c.last_activity, '+' || (c.stale_threshold / 1000000000) || ' seconds') > datetime('now')
		)`)

		// Epics with children are already decomposed — not ready.
		conditions = append(conditions, `(t.role = 'task' OR NOT EXISTS (
			SELECT 1 FROM issues c WHERE c.parent_id = t.issue_id AND c.deleted = 0
		))`)

		// No unresolved blocked_by relationships. A blocker is resolved if its
		// target issue is closed or deleted.
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM relationships r
			JOIN issues bt ON r.target_id = bt.issue_id
			WHERE r.source_id = t.issue_id
			  AND r.rel_type = 'blocked_by'
			  AND bt.deleted = 0
			  AND bt.state != 'closed'
		)`)

		// No ancestor epic is deferred or blocked. Walk the parent chain
		// with a recursive CTE and reject issues that have any such ancestor.
		conditions = append(conditions, `NOT EXISTS (
			WITH RECURSIVE ancestors(aid) AS (
				SELECT t.parent_id
				UNION ALL
				SELECT p.parent_id FROM issues p JOIN ancestors a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT 1 FROM ancestors a
			JOIN issues anc ON anc.issue_id = a.aid
			WHERE anc.state = 'deferred'
			   OR EXISTS (
				SELECT 1 FROM relationships r
				JOIN issues bt ON r.target_id = bt.issue_id
				WHERE r.source_id = anc.issue_id
				  AND r.rel_type = 'blocked_by'
				  AND bt.deleted = 0
				  AND bt.state != 'closed'
			)
		)`)
	}

	if filter.Orphan {
		conditions = append(conditions, "(t.parent_id IS NULL OR t.parent_id = '')")
	}

	if filter.Blocked {
		// Issues that are blocked — either directly (own blocked_by) or
		// inherited (an ancestor epic has unresolved blocked_by).
		conditions = append(conditions, `(
			EXISTS (
				SELECT 1 FROM relationships r
				JOIN issues bt ON r.target_id = bt.issue_id
				WHERE r.source_id = t.issue_id
				  AND r.rel_type = 'blocked_by'
				  AND bt.deleted = 0
				  AND bt.state != 'closed'
			)
			OR EXISTS (
				WITH RECURSIVE ancestors(aid) AS (
					SELECT t.parent_id
					UNION ALL
					SELECT p.parent_id FROM issues p JOIN ancestors a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
				)
				SELECT 1 FROM ancestors a
				JOIN issues anc ON anc.issue_id = a.aid
				WHERE EXISTS (
					SELECT 1 FROM relationships r
					JOIN issues bt ON r.target_id = bt.issue_id
					WHERE r.source_id = anc.issue_id
					  AND r.rel_type = 'blocked_by'
					  AND bt.deleted = 0
					  AND bt.state != 'closed'
				)
			)
		)`)
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

// buildVirtualLabelCondition generates a SQL condition for a virtual label
// filter that targets a column on the issues table instead of the labels
// table. Currently supports the idempotency-key virtual label.
func buildVirtualLabelCondition(ff driven.LabelFilter) (string, []any) {
	switch ff.Key {
	case domain.VirtualKeyIdempotency:
		if ff.Negate {
			if ff.Value == "" {
				return `t.idempotency_key IS NULL`, nil
			}
			return `(t.idempotency_key IS NULL OR t.idempotency_key != ?)`, []any{ff.Value}
		}
		if ff.Value == "" {
			return `t.idempotency_key IS NOT NULL`, nil
		}
		return `t.idempotency_key = ?`, []any{ff.Value}
	default:
		// Unknown virtual label — fall through to a no-match condition.
		return `0`, nil
	}
}

// parseIDList splits a comma-separated string of issue IDs into a slice.
// Used to parse the GROUP_CONCAT output of blocker ID subqueries.
func parseIDList(raw string) []domain.ID {
	parts := strings.Split(raw, ",")
	ids := make([]domain.ID, 0, len(parts))
	for _, p := range parts {
		if id, err := domain.ParseID(strings.TrimSpace(p)); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// issueOrderClause returns the ORDER BY clause for issue list queries.
// All clauses use a family-anchored creation time as the secondary sort:
// COALESCE(parent.created_at, t.created_at) clusters children with their
// parent in chronological position, followed by t.created_at to order within
// a family. Every clause ends with t.issue_id ASC as a tiebreaker, making
// results deterministic when issues share the same primary sort values.
//
// Callers must include the parent join returned by issueParentJoin() in
// their FROM clause for the COALESCE expression to resolve correctly.
func issueOrderClause(orderBy driven.IssueOrderBy, direction driven.SortDirection) string {
	// primaryDir is the SQL keyword for the primary sort axis. Tiebreaker
	// columns (issue_id) always use ASC for deterministic output.
	primaryDir := "ASC"
	if direction == driven.SortDescending {
		primaryDir = "DESC"
	}

	familyAnchor := "COALESCE(parent.created_at, t.created_at)"
	switch orderBy {
	case driven.OrderByPriority:
		return " ORDER BY t.priority " + primaryDir + ", " + familyAnchor + " " + primaryDir + ", t.created_at " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByPriorityCreated:
		return " ORDER BY t.priority " + primaryDir + ", t.created_at " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByCreatedAt:
		return " ORDER BY " + familyAnchor + " " + primaryDir + ", t.created_at " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByUpdatedAt:
		// SortAscending is oldest-first; SortDescending is newest-first.
		// primaryDir handles the direction uniformly, matching how every other
		// OrderBy value behaves — no per-key inversion needed.
		return " ORDER BY " + familyAnchor + " " + primaryDir + ", t.created_at " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByID:
		return " ORDER BY t.issue_id " + primaryDir
	case driven.OrderByRole:
		return " ORDER BY t.role " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByState:
		return " ORDER BY t.state " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByTitle:
		return " ORDER BY t.title COLLATE NOCASE " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByParentID:
		// COALESCE substitutes an empty-string sentinel for parentless issues.
		// Under SortAscending the sentinel sorts before any real ID (parentless
		// first); under SortDescending the ordering is reversed (parentless last).
		return " ORDER BY COALESCE(t.parent_id, '') " + primaryDir + ", t.issue_id ASC"
	case driven.OrderByParentCreated:
		// COALESCE substitutes an empty-string sentinel (not an epoch timestamp)
		// for parentless issues. Under SortAscending the sentinel sorts before
		// any real timestamp (parentless first); under SortDescending the
		// ordering is reversed (parentless last).
		return " ORDER BY COALESCE(parent.created_at, '') " + primaryDir + ", t.issue_id ASC"
	default:
		return " ORDER BY t.priority " + primaryDir + ", " + familyAnchor + " " + primaryDir + ", t.created_at " + primaryDir + ", t.issue_id ASC"
	}
}

// issueParentJoin returns the LEFT JOIN clause that resolves a parent
// issue's created_at for family-anchored sorting. Must be included in
// any query that uses issueOrderClause.
func issueParentJoin() string {
	return " LEFT JOIN issues parent ON parent.issue_id = t.parent_id"
}
