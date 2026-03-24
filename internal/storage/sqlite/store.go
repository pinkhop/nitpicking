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
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
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
// DDL — the database must already have been created with Create. It runs any
// pending one-shot migrations (e.g. the ticket→issue rename) before returning.
func Open(dbPath string) (*Store, error) {
	pool, err := sqlitex.NewPool(dbPath, sqlitex.PoolOptions{
		PoolSize:    1,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "open database", Err: err}
	}

	conn, err := pool.Take(context.Background())
	if err != nil {
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "take connection for migration", Err: errors.Join(err, closeErr)}
	}
	err = migrateTicketsToIssues(conn)
	if err != nil {
		pool.Put(conn)
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "migrate tickets to issues", Err: errors.Join(err, closeErr)}
	}
	err = migrateFacetsToDimensions(conn)
	if err != nil {
		pool.Put(conn)
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "migrate facets to dimensions", Err: errors.Join(err, closeErr)}
	}
	err = migrateNotesToComments(conn)
	if err != nil {
		pool.Put(conn)
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "migrate notes to comments", Err: errors.Join(err, closeErr)}
	}
	err = migrateActiveToOpen(conn)
	pool.Put(conn)
	if err != nil {
		closeErr := pool.Close()
		return nil, &domain.DatabaseError{Op: "migrate active to open", Err: errors.Join(err, closeErr)}
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
func (s *Store) WithTransaction(ctx context.Context, fn func(uow port.UnitOfWork) error) (err error) {
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
func (s *Store) WithReadTransaction(ctx context.Context, fn func(uow port.UnitOfWork) error) (err error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return &domain.DatabaseError{Op: "take connection", Err: err}
	}
	defer s.pool.Put(conn)

	endFn := sqlitex.Transaction(conn)
	defer endFn(&err)

	return fn(&connUnitOfWork{conn: conn})
}

// connUnitOfWork wraps a *sqlite.Conn to implement port.UnitOfWork.
type connUnitOfWork struct {
	conn *sqlite.Conn
}

func (u *connUnitOfWork) Issues() port.IssueRepository               { return &issueRepo{conn: u.conn} }
func (u *connUnitOfWork) Comments() port.CommentRepository           { return &commentRepo{conn: u.conn} }
func (u *connUnitOfWork) Claims() port.ClaimRepository               { return &claimRepo{conn: u.conn} }
func (u *connUnitOfWork) Relationships() port.RelationshipRepository { return &relRepo{conn: u.conn} }
func (u *connUnitOfWork) History() port.HistoryRepository            { return &histRepo{conn: u.conn} }
func (u *connUnitOfWork) Database() port.DatabaseRepository          { return &dbRepo{conn: u.conn} }

// --- DatabaseRepository ---

type dbRepo struct{ conn *sqlite.Conn }

func (r *dbRepo) InitDatabase(_ context.Context, prefix string) error {
	err := sqlitex.Execute(r.conn, `INSERT INTO metadata (key, value) VALUES ('prefix', ?)`, &sqlitex.ExecOptions{
		Args: []any{prefix},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "init database", Err: err}
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

func (r *dbRepo) GC(_ context.Context, includeClosed bool) error {
	gcQueries := []struct {
		op    string
		query string
	}{
		{"gc dimensions", `DELETE FROM dimensions WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc comments", `DELETE FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc history", `DELETE FROM history WHERE issue_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc relationships", `DELETE FROM relationships WHERE source_id IN (SELECT issue_id FROM issues WHERE deleted = 1) OR target_id IN (SELECT issue_id FROM issues WHERE deleted = 1)`},
		{"gc issues", `DELETE FROM issues WHERE deleted = 1`},
	}

	for _, q := range gcQueries {
		if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
			return &domain.DatabaseError{Op: q.op, Err: err}
		}
	}

	if includeClosed {
		closedQueries := []struct {
			op    string
			query string
		}{
			{"gc closed dimensions", `DELETE FROM dimensions WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed comments", `DELETE FROM comments WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed history", `DELETE FROM history WHERE issue_id IN (SELECT issue_id FROM issues WHERE state = 'closed')`},
			{"gc closed issues", `DELETE FROM issues WHERE state = 'closed'`},
		}

		for _, q := range closedQueries {
			if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
				return &domain.DatabaseError{Op: q.op, Err: err}
			}
		}
	}

	return nil
}

// --- IssueRepository ---

type issueRepo struct{ conn *sqlite.Conn }

func (r *issueRepo) CreateIssue(_ context.Context, t issue.Issue) error {
	parentID := nullable(t.ParentID())
	var idemKey any
	if t.IdempotencyKey() != "" {
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

	// Save dimensions.
	for k, v := range t.Dimensions().All() {
		if err := sqlitex.Execute(r.conn, `INSERT INTO dimensions (issue_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "create dimension", Err: err}
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

func (r *issueRepo) GetIssue(_ context.Context, id issue.ID, includeDeleted bool) (issue.Issue, error) {
	query := `SELECT issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM issues WHERE issue_id = ?`
	if !includeDeleted {
		query += ` AND deleted = 0`
	}

	t, err := scanIssueRow(r.conn, query, id.String())
	if err != nil {
		if isNotFound(err) {
			return issue.Issue{}, domain.ErrNotFound
		}
		return issue.Issue{}, &domain.DatabaseError{Op: "get issue", Err: err}
	}

	// Load dimensions.
	dimensions, err := r.loadDimensions(id.String())
	if err != nil {
		return issue.Issue{}, err
	}
	t = t.WithDimensions(dimensions)

	return t, nil
}

func (r *issueRepo) UpdateIssue(_ context.Context, t issue.Issue) error {
	parentID := nullable(t.ParentID())

	err := sqlitex.Execute(r.conn,
		`UPDATE issues SET title = ?, description = ?, acceptance_criteria = ?, priority = ?, state = ?, parent_id = ?, deleted = ? WHERE issue_id = ?`,
		&sqlitex.ExecOptions{
			Args: []any{
				t.Title(), t.Description(), t.AcceptanceCriteria(), t.Priority().String(),
				t.State().String(), parentID, boolToInt(t.IsDeleted()), t.ID().String(),
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "update issue", Err: err}
	}

	// Replace dimensions.
	_ = sqlitex.Execute(r.conn, `DELETE FROM dimensions WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	})
	for k, v := range t.Dimensions().All() {
		if err := sqlitex.Execute(r.conn, `INSERT INTO dimensions (issue_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "update dimension", Err: err}
		}
	}

	// FTS sync — delete old entry and insert updated one.
	_ = sqlitex.Execute(r.conn, `DELETE FROM issues_fts WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	})
	_ = sqlitex.Execute(r.conn, `INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String(), t.Title(), t.Description(), t.AcceptanceCriteria()},
	})

	return nil
}

func (r *issueRepo) ListIssues(_ context.Context, filter port.IssueFilter, orderBy port.IssueOrderBy, page port.PageRequest) ([]port.IssueListItem, port.PageResult, error) {
	page = page.Normalize()
	where, args := buildIssueWhere(filter)

	// Count total.
	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues t `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count issues", Err: err}
	}

	orderClause := issueOrderClause(orderBy)
	query := `SELECT t.issue_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted, t.parent_id FROM issues t ` + where + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	var items []port.IssueListItem
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := issue.ParseID(stmt.ColumnText(0))
			role, _ := issue.ParseRole(stmt.ColumnText(1))
			state, _ := issue.ParseState(stmt.ColumnText(2))
			priority, _ := issue.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))
			parentID, _ := issue.ParseID(stmt.ColumnText(7))

			items = append(items, port.IssueListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), ParentID: parentID, CreatedAt: createdAt,
				IsDeleted: stmt.ColumnInt(6) != 0,
			})
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list issues", Err: err}
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *issueRepo) SearchIssues(_ context.Context, query string, filter port.IssueFilter, orderBy port.IssueOrderBy, page port.PageRequest) ([]port.IssueListItem, port.PageResult, error) {
	page = page.Normalize()
	where, args := buildIssueWhere(filter)

	ftsWhere := ` AND t.issue_id IN (SELECT issue_id FROM issues_fts WHERE issues_fts MATCH ?)`
	args = append(args, query)

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues t `+where+ftsWhere, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count search results", Err: err}
	}

	orderClause := issueOrderClause(orderBy)
	selectQuery := `SELECT t.issue_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted, t.parent_id FROM issues t ` + where + ftsWhere + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	var items []port.IssueListItem
	err = sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := issue.ParseID(stmt.ColumnText(0))
			role, _ := issue.ParseRole(stmt.ColumnText(1))
			state, _ := issue.ParseState(stmt.ColumnText(2))
			priority, _ := issue.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))
			parentID, _ := issue.ParseID(stmt.ColumnText(7))

			items = append(items, port.IssueListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), ParentID: parentID, CreatedAt: createdAt,
				IsDeleted: stmt.ColumnInt(6) != 0,
			})
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search issues", Err: err}
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *issueRepo) GetChildStatuses(_ context.Context, epicID issue.ID) ([]issue.ChildStatus, error) {
	var children []issue.ChildStatus
	err := sqlitex.Execute(r.conn, `SELECT role, state FROM issues WHERE parent_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
		Args: []any{epicID.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			role, _ := issue.ParseRole(stmt.ColumnText(0))
			state, _ := issue.ParseState(stmt.ColumnText(1))
			children = append(children, issue.ChildStatus{
				Role: role, State: state, IsComplete: state == issue.StateClosed,
			})
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get child statuses", Err: err}
	}
	return children, nil
}

func (r *issueRepo) GetDescendants(_ context.Context, epicID issue.ID) ([]issue.DescendantInfo, error) {
	return r.getDescendantsRecursive(epicID)
}

func (r *issueRepo) getDescendantsRecursive(parentID issue.ID) ([]issue.DescendantInfo, error) {
	type childInfo struct {
		id      issue.ID
		role    issue.Role
		claimed bool
		author  string
	}
	var childInfos []childInfo

	err := sqlitex.Execute(r.conn,
		`SELECT t.issue_id, t.role, COALESCE(c.author, '') as claim_author FROM issues t LEFT JOIN claims c ON t.issue_id = c.issue_id WHERE t.parent_id = ? AND t.deleted = 0`,
		&sqlitex.ExecOptions{
			Args: []any{parentID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				id, _ := issue.ParseID(stmt.ColumnText(0))
				role, _ := issue.ParseRole(stmt.ColumnText(1))
				claimAuthor := stmt.ColumnText(2)
				childInfos = append(childInfos, childInfo{id: id, role: role, claimed: claimAuthor != "", author: claimAuthor})
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get descendants", Err: err}
	}

	var descendants []issue.DescendantInfo
	for _, ci := range childInfos {
		descendants = append(descendants, issue.DescendantInfo{
			ID: ci.id, IsClaimed: ci.claimed, ClaimedBy: ci.author,
		})
		if ci.role == issue.RoleEpic {
			sub, err := r.getDescendantsRecursive(ci.id)
			if err != nil {
				return nil, err
			}
			descendants = append(descendants, sub...)
		}
	}

	return descendants, nil
}

func (r *issueRepo) HasChildren(_ context.Context, epicID issue.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues WHERE parent_id = ? AND deleted = 0`, epicID.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "has children", Err: err}
	}
	return count > 0, nil
}

func (r *issueRepo) GetAncestorStatuses(_ context.Context, id issue.ID) ([]issue.AncestorStatus, error) {
	var ancestors []issue.AncestorStatus
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
		err = sqlitex.Execute(r.conn, `SELECT state FROM issues WHERE issue_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
			Args: []any{parentID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				stateStr = stmt.ColumnText(0)
				stateFound = true
				return nil
			},
		})
		if err != nil || !stateFound {
			break
		}

		state, _ := issue.ParseState(stateStr)
		ancestors = append(ancestors, issue.AncestorStatus{State: state})
		current = parentID
	}

	return ancestors, nil
}

func (r *issueRepo) GetParentID(_ context.Context, id issue.ID) (issue.ID, error) {
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
		return issue.ID{}, &domain.DatabaseError{Op: "get parent ID", Err: err}
	}
	if !found {
		return issue.ID{}, domain.ErrNotFound
	}
	if isNull || parentID == "" {
		return issue.ID{}, nil
	}
	return issue.ParseID(parentID)
}

func (r *issueRepo) IssueIDExists(_ context.Context, id issue.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM issues WHERE issue_id = ?`, id.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "check issue exists", Err: err}
	}
	return count > 0, nil
}

func (r *issueRepo) GetIssueByIdempotencyKey(_ context.Context, key string) (issue.Issue, error) {
	t, err := scanIssueRow(r.conn,
		`SELECT issue_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM issues WHERE idempotency_key = ?`, key)
	if err != nil {
		if isNotFound(err) {
			return issue.Issue{}, domain.ErrNotFound
		}
		return issue.Issue{}, &domain.DatabaseError{Op: "get by idempotency key", Err: err}
	}
	return t, nil
}

func (r *issueRepo) loadDimensions(issueID string) (issue.DimensionSet, error) {
	fs := issue.NewDimensionSet()
	err := sqlitex.Execute(r.conn, `SELECT key, value FROM dimensions WHERE issue_id = ?`, &sqlitex.ExecOptions{
		Args: []any{issueID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			f, _ := issue.NewDimension(stmt.ColumnText(0), stmt.ColumnText(1))
			fs = fs.Set(f)
			return nil
		},
	})
	if err != nil {
		return issue.NewDimensionSet(), &domain.DatabaseError{Op: "load dimensions", Err: err}
	}
	return fs, nil
}

// --- CommentRepository ---

type commentRepo struct{ conn *sqlite.Conn }

func (r *commentRepo) CreateComment(_ context.Context, n comment.Comment) (int64, error) {
	err := sqlitex.Execute(r.conn, `INSERT INTO comments (issue_id, author, created_at, body) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{n.IssueID().String(), n.Author().String(), n.CreatedAt().Format(time.RFC3339Nano), n.Body()},
	})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "create comment", Err: err}
	}

	commentID := r.conn.LastInsertRowID()

	// FTS sync.
	_ = sqlitex.Execute(r.conn, `INSERT INTO comments_fts (comment_id, body) VALUES (?, ?)`, &sqlitex.ExecOptions{
		Args: []any{commentID, n.Body()},
	})

	return commentID, nil
}

func (r *commentRepo) GetComment(_ context.Context, id int64) (comment.Comment, error) {
	var result comment.Comment
	var found bool

	err := sqlitex.Execute(r.conn, `SELECT issue_id, author, created_at, body FROM comments WHERE comment_id = ?`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			issueID, _ := issue.ParseID(stmt.ColumnText(0))
			author, _ := identity.NewAuthor(stmt.ColumnText(1))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(2))

			var err error
			result, err = comment.NewComment(comment.NewCommentParams{
				ID: id, IssueID: issueID, Author: author, CreatedAt: createdAt, Body: stmt.ColumnText(3),
			})
			return err
		},
	})
	if err != nil {
		return comment.Comment{}, &domain.DatabaseError{Op: "get comment", Err: err}
	}
	if !found {
		return comment.Comment{}, domain.ErrNotFound
	}
	return result, nil
}

func (r *commentRepo) ListComments(_ context.Context, issueID issue.ID, filter port.CommentFilter, page port.PageRequest) ([]comment.Comment, port.PageResult, error) {
	page = page.Normalize()

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

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM comments `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count comments", Err: err}
	}

	query := `SELECT comment_id, issue_id, author, created_at, body FROM comments ` + where + ` ORDER BY comment_id LIMIT ?`
	args = append(args, page.PageSize)

	var comments []comment.Comment
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := issue.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := comment.NewComment(comment.NewCommentParams{
				ID: stmt.ColumnInt64(0), IssueID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			comments = append(comments, n)
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list comments", Err: err}
	}

	return comments, port.PageResult{TotalCount: total}, nil
}

func (r *commentRepo) SearchComments(_ context.Context, query string, filter port.CommentFilter, page port.PageRequest) ([]comment.Comment, port.PageResult, error) {
	page = page.Normalize()

	where := `WHERE c.comment_id IN (SELECT comment_id FROM comments_fts WHERE comments_fts MATCH ?)`
	args := []any{query}

	if !filter.IssueID.IsZero() {
		where += ` AND c.issue_id = ?`
		args = append(args, filter.IssueID.String())
	}

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM comments c `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count comment search results", Err: err}
	}

	selectQuery := `SELECT c.comment_id, c.issue_id, c.author, c.created_at, c.body FROM comments c ` + where + ` ORDER BY c.comment_id LIMIT ?`
	args = append(args, page.PageSize)

	var comments []comment.Comment
	err = sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := issue.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := comment.NewComment(comment.NewCommentParams{
				ID: stmt.ColumnInt64(0), IssueID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			comments = append(comments, n)
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search comments", Err: err}
	}

	return comments, port.PageResult{TotalCount: total}, nil
}

// --- ClaimRepository ---

type claimRepo struct{ conn *sqlite.Conn }

func (r *claimRepo) CreateClaim(_ context.Context, c claim.Claim) error {
	err := sqlitex.Execute(r.conn, `INSERT OR REPLACE INTO claims (claim_id, issue_id, author, stale_threshold, last_activity) VALUES (?, ?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{c.ID(), c.IssueID().String(), c.Author().String(), int64(c.StaleThreshold()), c.LastActivity().Format(time.RFC3339Nano)},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "create claim", Err: err}
	}
	return nil
}

func (r *claimRepo) GetClaimByIssue(_ context.Context, issueID issue.ID) (claim.Claim, error) {
	return r.scanClaim(`SELECT claim_id, issue_id, author, stale_threshold, last_activity FROM claims WHERE issue_id = ?`, issueID.String())
}

func (r *claimRepo) GetClaimByID(_ context.Context, claimID string) (claim.Claim, error) {
	return r.scanClaim(`SELECT claim_id, issue_id, author, stale_threshold, last_activity FROM claims WHERE claim_id = ?`, claimID)
}

func (r *claimRepo) InvalidateClaim(_ context.Context, claimID string) error {
	err := sqlitex.Execute(r.conn, `DELETE FROM claims WHERE claim_id = ?`, &sqlitex.ExecOptions{
		Args: []any{claimID},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "invalidate claim", Err: err}
	}
	if r.conn.Changes() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *claimRepo) UpdateClaimLastActivity(_ context.Context, claimID string, lastActivity time.Time) error {
	err := sqlitex.Execute(r.conn, `UPDATE claims SET last_activity = ? WHERE claim_id = ?`, &sqlitex.ExecOptions{
		Args: []any{lastActivity.Format(time.RFC3339Nano), claimID},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "update claim activity", Err: err}
	}
	return nil
}

func (r *claimRepo) UpdateClaimThreshold(_ context.Context, claimID string, threshold time.Duration) error {
	err := sqlitex.Execute(r.conn, `UPDATE claims SET stale_threshold = ? WHERE claim_id = ?`, &sqlitex.ExecOptions{
		Args: []any{int64(threshold), claimID},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "update claim threshold", Err: err}
	}
	return nil
}

func (r *claimRepo) ListStaleClaims(_ context.Context, now time.Time) ([]claim.Claim, error) {
	var stale []claim.Claim
	err := sqlitex.Execute(r.conn, `SELECT claim_id, issue_id, author, stale_threshold, last_activity FROM claims`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := issue.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			lastAct, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			c := claim.ReconstructClaim(stmt.ColumnText(0), tid, author, time.Duration(stmt.ColumnInt64(3)), lastAct)
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

func (r *claimRepo) scanClaim(query string, args ...any) (claim.Claim, error) {
	var result claim.Claim
	var found bool

	err := sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			tid, _ := issue.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			lastAct, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			result = claim.ReconstructClaim(stmt.ColumnText(0), tid, author, time.Duration(stmt.ColumnInt64(3)), lastAct)
			return nil
		},
	})
	if err != nil {
		return claim.Claim{}, &domain.DatabaseError{Op: "get claim", Err: err}
	}
	if !found {
		return claim.Claim{}, domain.ErrNotFound
	}
	return result, nil
}

// --- RelationshipRepository ---

type relRepo struct{ conn *sqlite.Conn }

func (r *relRepo) CreateRelationship(_ context.Context, rel issue.Relationship) (bool, error) {
	err := sqlitex.Execute(r.conn, `INSERT OR IGNORE INTO relationships (source_id, target_id, rel_type) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{rel.SourceID().String(), rel.TargetID().String(), rel.Type().String()},
	})
	if err != nil {
		return false, &domain.DatabaseError{Op: "create relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) DeleteRelationship(_ context.Context, sourceID, targetID issue.ID, relType issue.RelationType) (bool, error) {
	err := sqlitex.Execute(r.conn, `DELETE FROM relationships WHERE source_id = ? AND target_id = ? AND rel_type = ?`, &sqlitex.ExecOptions{
		Args: []any{sourceID.String(), targetID.String(), relType.String()},
	})
	if err != nil {
		return false, &domain.DatabaseError{Op: "delete relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) ListRelationships(_ context.Context, issueID issue.ID) ([]issue.Relationship, error) {
	var rels []issue.Relationship
	err := sqlitex.Execute(r.conn, `SELECT source_id, target_id, rel_type FROM relationships WHERE source_id = ? OR target_id = ?`, &sqlitex.ExecOptions{
		Args: []any{issueID.String(), issueID.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			src, _ := issue.ParseID(stmt.ColumnText(0))
			tgt, _ := issue.ParseID(stmt.ColumnText(1))
			rt, _ := issue.ParseRelationType(stmt.ColumnText(2))
			rel, _ := issue.NewRelationship(src, tgt, rt)
			rels = append(rels, rel)
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list relationships", Err: err}
	}
	return rels, nil
}

func (r *relRepo) GetBlockerStatuses(_ context.Context, issueID issue.ID) ([]issue.BlockerStatus, error) {
	var statuses []issue.BlockerStatus
	err := sqlitex.Execute(r.conn,
		`SELECT t.state, t.deleted, t.role, t.issue_id FROM relationships r JOIN issues t ON r.target_id = t.issue_id WHERE r.source_id = ? AND r.rel_type = 'blocked_by'`,
		&sqlitex.ExecOptions{
			Args: []any{issueID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				state, _ := issue.ParseState(stmt.ColumnText(0))
				deleted := stmt.ColumnInt(1) != 0
				role, _ := issue.ParseRole(stmt.ColumnText(2))
				targetID := stmt.ColumnText(3)

				isComplete := false
				if role == issue.RoleEpic {
					isComplete = r.isEpicComplete(targetID)
				}

				statuses = append(statuses, issue.BlockerStatus{
					IsClosed:   state == issue.StateClosed,
					IsDeleted:  deleted,
					IsComplete: isComplete,
				})
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get blocker statuses", Err: err}
	}
	return statuses, nil
}

// isEpicComplete recursively derives whether an epic is complete by checking
// that it has children and all of them are closed (tasks) or complete
// (sub-epics).
func (r *relRepo) isEpicComplete(epicID string) bool {
	type child struct {
		role  issue.Role
		state issue.State
		id    string
	}

	var children []child
	_ = sqlitex.Execute(r.conn,
		`SELECT role, state, issue_id FROM issues WHERE parent_id = ? AND deleted = 0`,
		&sqlitex.ExecOptions{
			Args: []any{epicID},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				role, _ := issue.ParseRole(stmt.ColumnText(0))
				state, _ := issue.ParseState(stmt.ColumnText(1))
				children = append(children, child{role: role, state: state, id: stmt.ColumnText(2)})
				return nil
			},
		})

	if len(children) == 0 {
		return false
	}

	for _, c := range children {
		switch c.role {
		case issue.RoleTask:
			if c.state != issue.StateClosed {
				return false
			}
		case issue.RoleEpic:
			if !r.isEpicComplete(c.id) {
				return false
			}
		}
	}
	return true
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

func (r *histRepo) ListHistory(_ context.Context, issueID issue.ID, filter port.HistoryFilter, page port.PageRequest) ([]history.Entry, port.PageResult, error) {
	page = page.Normalize()

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

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM history `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count history", Err: err}
	}

	query := `SELECT entry_id, issue_id, revision, author, timestamp, event_type, changes FROM history ` + where + ` ORDER BY revision LIMIT ?`
	args = append(args, page.PageSize)

	var entries []history.Entry
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := issue.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(3))
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
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list history", Err: err}
	}

	return entries, port.PageResult{TotalCount: total}, nil
}

func (r *histRepo) CountHistory(_ context.Context, issueID issue.ID) (int, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM history WHERE issue_id = ?`, issueID.String())
	if err != nil {
		return 0, &domain.DatabaseError{Op: "count history", Err: err}
	}
	return count, nil
}

func (r *histRepo) GetLatestHistory(_ context.Context, issueID issue.ID) (history.Entry, error) {
	var result history.Entry
	var found bool

	err := sqlitex.Execute(r.conn,
		`SELECT entry_id, issue_id, revision, author, timestamp, event_type, changes FROM history WHERE issue_id = ? ORDER BY revision DESC LIMIT 1`,
		&sqlitex.ExecOptions{
			Args: []any{issueID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				tid, _ := issue.ParseID(stmt.ColumnText(1))
				author, _ := identity.NewAuthor(stmt.ColumnText(3))
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
func scanIssueRow(conn *sqlite.Conn, query string, args ...any) (issue.Issue, error) {
	var t issue.Issue
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

			id, _ := issue.ParseID(idStr)
			priority, _ := issue.ParsePriority(priorityStr)
			state, _ := issue.ParseState(stateStr)
			createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

			var pid issue.ID
			if parentIDStr != "" {
				pid, _ = issue.ParseID(parentIDStr)
			}

			role, _ := issue.ParseRole(roleStr)

			switch role {
			case issue.RoleTask:
				t, _ = issue.NewTask(issue.NewTaskParams{
					ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
					Priority: priority, ParentID: pid, CreatedAt: createdAt,
					IdempotencyKey: idemKey,
				})
			case issue.RoleEpic:
				t, _ = issue.NewEpic(issue.NewEpicParams{
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
		return issue.Issue{}, err
	}
	if !found {
		return issue.Issue{}, errNotFound
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

// nullable converts an issue.ID to a value suitable for SQL binding. Returns
// nil (which binds as NULL) for zero IDs, or the string representation
// otherwise.
func nullable(id issue.ID) any {
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

func buildIssueWhere(filter port.IssueFilter) (string, []any) {
	conditions := []string{"1=1"}
	var args []any

	if !filter.IncludeDeleted {
		conditions = append(conditions, "t.deleted = 0")
	}

	if filter.Role != 0 {
		conditions = append(conditions, "t.role = ?")
		args = append(args, filter.Role.String())
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

	if !filter.ParentID.IsZero() {
		conditions = append(conditions, "t.parent_id = ?")
		args = append(args, filter.ParentID.String())
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

	for _, ff := range filter.DimensionFilters {
		if ff.Negate {
			if ff.Value == "" {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM dimensions f WHERE f.issue_id = t.issue_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM dimensions f WHERE f.issue_id = t.issue_id AND f.key = ? AND f.value = ?)`)
				args = append(args, ff.Key, ff.Value)
				continue
			}
		} else {
			if ff.Value == "" {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM dimensions f WHERE f.issue_id = t.issue_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM dimensions f WHERE f.issue_id = t.issue_id AND f.key = ? AND f.value = ?)`)
				args = append(args, ff.Key, ff.Value)
				continue
			}
		}
		args = append(args, ff.Key)
	}

	if filter.Ready {
		// Ready means: correct state, no unresolved blockers, no deferred/waiting
		// ancestors, and (for epics) no children.
		//
		// State: all issues must be open.
		conditions = append(conditions, `t.state = 'open'`)

		// Epics with children are already decomposed — not ready.
		conditions = append(conditions, `(t.role = 'task' OR NOT EXISTS (
			SELECT 1 FROM issues c WHERE c.parent_id = t.issue_id AND c.deleted = 0
		))`)

		// No unresolved blocked_by relationships. A blocker is resolved if its
		// target issue is closed, deleted, or a complete epic (all non-deleted
		// children closed; checked one level deep — sufficient because the
		// readiness filter runs on list queries where deep nesting is rare,
		// and the per-issue GetBlockerStatuses path handles full recursion).
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM relationships r
			JOIN issues bt ON r.target_id = bt.issue_id
			WHERE r.source_id = t.issue_id
			  AND r.rel_type = 'blocked_by'
			  AND bt.deleted = 0
			  AND bt.state != 'closed'
			  AND NOT (
			    bt.role = 'epic'
			    AND EXISTS (SELECT 1 FROM issues ec WHERE ec.parent_id = bt.issue_id AND ec.deleted = 0)
			    AND NOT EXISTS (
			      SELECT 1 FROM issues ec
			      WHERE ec.parent_id = bt.issue_id
			        AND ec.deleted = 0
			        AND ec.state != 'closed'
			    )
			  )
		)`)

		// No ancestor epic is deferred or waiting. Walk the parent chain with
		// a recursive CTE and reject issues that have any such ancestor.
		conditions = append(conditions, `NOT EXISTS (
			WITH RECURSIVE ancestors(aid) AS (
				SELECT t.parent_id
				UNION ALL
				SELECT p.parent_id FROM issues p JOIN ancestors a ON p.issue_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT 1 FROM ancestors a
			JOIN issues anc ON anc.issue_id = a.aid
			WHERE anc.state IN ('deferred', 'waiting')
		)`)
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

func issueOrderClause(orderBy port.IssueOrderBy) string {
	switch orderBy {
	case port.OrderByPriority:
		return " ORDER BY t.priority, t.created_at"
	case port.OrderByCreatedAt:
		return " ORDER BY t.created_at"
	case port.OrderByUpdatedAt:
		return " ORDER BY t.created_at DESC"
	default:
		return " ORDER BY t.priority, t.created_at"
	}
}
