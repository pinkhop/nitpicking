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
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
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

func (u *connUnitOfWork) Tickets() port.TicketRepository             { return &ticketRepo{conn: u.conn} }
func (u *connUnitOfWork) Notes() port.NoteRepository                 { return &noteRepo{conn: u.conn} }
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
		{"gc facets", `DELETE FROM facets WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`},
		{"gc notes", `DELETE FROM notes WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`},
		{"gc history", `DELETE FROM history WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`},
		{"gc relationships", `DELETE FROM relationships WHERE source_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1) OR target_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`},
		{"gc tickets", `DELETE FROM tickets WHERE deleted = 1`},
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
			{"gc closed facets", `DELETE FROM facets WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`},
			{"gc closed notes", `DELETE FROM notes WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`},
			{"gc closed history", `DELETE FROM history WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`},
			{"gc closed tickets", `DELETE FROM tickets WHERE state = 'closed'`},
		}

		for _, q := range closedQueries {
			if err := sqlitex.Execute(r.conn, q.query, nil); err != nil {
				return &domain.DatabaseError{Op: q.op, Err: err}
			}
		}
	}

	return nil
}

// --- TicketRepository ---

type ticketRepo struct{ conn *sqlite.Conn }

func (r *ticketRepo) CreateTicket(_ context.Context, t ticket.Ticket) error {
	parentID := nullable(t.ParentID())
	var idemKey any
	if t.IdempotencyKey() != "" {
		idemKey = t.IdempotencyKey()
	}

	err := sqlitex.Execute(r.conn,
		`INSERT INTO tickets (ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{
				t.ID().String(), t.Role().String(), t.Title(), t.Description(), t.AcceptanceCriteria(),
				t.Priority().String(), t.State().String(), parentID, t.CreatedAt().Format(time.RFC3339Nano),
				idemKey, boolToInt(t.IsDeleted()),
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "create ticket", Err: err}
	}

	// Save facets.
	for k, v := range t.Facets().All() {
		if err := sqlitex.Execute(r.conn, `INSERT INTO facets (ticket_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "create facet", Err: err}
		}
	}

	// FTS sync.
	err = sqlitex.Execute(r.conn, `INSERT INTO tickets_fts (ticket_id, title, description, acceptance_criteria) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String(), t.Title(), t.Description(), t.AcceptanceCriteria()},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "fts insert", Err: err}
	}

	return nil
}

func (r *ticketRepo) GetTicket(_ context.Context, id ticket.ID, includeDeleted bool) (ticket.Ticket, error) {
	query := `SELECT ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM tickets WHERE ticket_id = ?`
	if !includeDeleted {
		query += ` AND deleted = 0`
	}

	t, err := scanTicketRow(r.conn, query, id.String())
	if err != nil {
		if isNotFound(err) {
			return ticket.Ticket{}, domain.ErrNotFound
		}
		return ticket.Ticket{}, &domain.DatabaseError{Op: "get ticket", Err: err}
	}

	// Load facets.
	facets, err := r.loadFacets(id.String())
	if err != nil {
		return ticket.Ticket{}, err
	}
	t = t.WithFacets(facets)

	return t, nil
}

func (r *ticketRepo) UpdateTicket(_ context.Context, t ticket.Ticket) error {
	parentID := nullable(t.ParentID())

	err := sqlitex.Execute(r.conn,
		`UPDATE tickets SET title = ?, description = ?, acceptance_criteria = ?, priority = ?, state = ?, parent_id = ?, deleted = ? WHERE ticket_id = ?`,
		&sqlitex.ExecOptions{
			Args: []any{
				t.Title(), t.Description(), t.AcceptanceCriteria(), t.Priority().String(),
				t.State().String(), parentID, boolToInt(t.IsDeleted()), t.ID().String(),
			},
		})
	if err != nil {
		return &domain.DatabaseError{Op: "update ticket", Err: err}
	}

	// Replace facets.
	_ = sqlitex.Execute(r.conn, `DELETE FROM facets WHERE ticket_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	})
	for k, v := range t.Facets().All() {
		if err := sqlitex.Execute(r.conn, `INSERT INTO facets (ticket_id, key, value) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
			Args: []any{t.ID().String(), k, v},
		}); err != nil {
			return &domain.DatabaseError{Op: "update facet", Err: err}
		}
	}

	// FTS sync — delete old entry and insert updated one.
	_ = sqlitex.Execute(r.conn, `DELETE FROM tickets_fts WHERE ticket_id = ?`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String()},
	})
	_ = sqlitex.Execute(r.conn, `INSERT INTO tickets_fts (ticket_id, title, description, acceptance_criteria) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{t.ID().String(), t.Title(), t.Description(), t.AcceptanceCriteria()},
	})

	return nil
}

func (r *ticketRepo) ListTickets(_ context.Context, filter port.TicketFilter, orderBy port.TicketOrderBy, page port.PageRequest) ([]port.TicketListItem, port.PageResult, error) {
	page = page.Normalize()
	where, args := buildTicketWhere(filter)

	// Count total.
	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM tickets t `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count tickets", Err: err}
	}

	orderClause := ticketOrderClause(orderBy)
	query := `SELECT t.ticket_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted FROM tickets t ` + where + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	var items []port.TicketListItem
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := ticket.ParseID(stmt.ColumnText(0))
			role, _ := ticket.ParseRole(stmt.ColumnText(1))
			state, _ := ticket.ParseState(stmt.ColumnText(2))
			priority, _ := ticket.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))

			items = append(items, port.TicketListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), CreatedAt: createdAt,
				IsDeleted: stmt.ColumnInt(6) != 0,
			})
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list tickets", Err: err}
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *ticketRepo) SearchTickets(_ context.Context, query string, filter port.TicketFilter, orderBy port.TicketOrderBy, page port.PageRequest) ([]port.TicketListItem, port.PageResult, error) {
	page = page.Normalize()
	where, args := buildTicketWhere(filter)

	ftsWhere := ` AND t.ticket_id IN (SELECT ticket_id FROM tickets_fts WHERE tickets_fts MATCH ?)`
	args = append(args, query)

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM tickets t `+where+ftsWhere, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count search results", Err: err}
	}

	orderClause := ticketOrderClause(orderBy)
	selectQuery := `SELECT t.ticket_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted FROM tickets t ` + where + ftsWhere + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	var items []port.TicketListItem
	err = sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			id, _ := ticket.ParseID(stmt.ColumnText(0))
			role, _ := ticket.ParseRole(stmt.ColumnText(1))
			state, _ := ticket.ParseState(stmt.ColumnText(2))
			priority, _ := ticket.ParsePriority(stmt.ColumnText(3))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(5))

			items = append(items, port.TicketListItem{
				ID: id, Role: role, State: state, Priority: priority,
				Title: stmt.ColumnText(4), CreatedAt: createdAt,
				IsDeleted: stmt.ColumnInt(6) != 0,
			})
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search tickets", Err: err}
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *ticketRepo) GetChildStatuses(_ context.Context, epicID ticket.ID) ([]ticket.ChildStatus, error) {
	var children []ticket.ChildStatus
	err := sqlitex.Execute(r.conn, `SELECT role, state FROM tickets WHERE parent_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
		Args: []any{epicID.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			role, _ := ticket.ParseRole(stmt.ColumnText(0))
			state, _ := ticket.ParseState(stmt.ColumnText(1))
			children = append(children, ticket.ChildStatus{
				Role: role, State: state, IsComplete: state == ticket.StateClosed,
			})
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get child statuses", Err: err}
	}
	return children, nil
}

func (r *ticketRepo) GetDescendants(_ context.Context, epicID ticket.ID) ([]ticket.DescendantInfo, error) {
	return r.getDescendantsRecursive(epicID)
}

func (r *ticketRepo) getDescendantsRecursive(parentID ticket.ID) ([]ticket.DescendantInfo, error) {
	type childInfo struct {
		id      ticket.ID
		role    ticket.Role
		claimed bool
		author  string
	}
	var childInfos []childInfo

	err := sqlitex.Execute(r.conn,
		`SELECT t.ticket_id, t.role, COALESCE(c.author, '') as claim_author FROM tickets t LEFT JOIN claims c ON t.ticket_id = c.ticket_id WHERE t.parent_id = ? AND t.deleted = 0`,
		&sqlitex.ExecOptions{
			Args: []any{parentID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				id, _ := ticket.ParseID(stmt.ColumnText(0))
				role, _ := ticket.ParseRole(stmt.ColumnText(1))
				claimAuthor := stmt.ColumnText(2)
				childInfos = append(childInfos, childInfo{id: id, role: role, claimed: claimAuthor != "", author: claimAuthor})
				return nil
			},
		})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get descendants", Err: err}
	}

	var descendants []ticket.DescendantInfo
	for _, ci := range childInfos {
		descendants = append(descendants, ticket.DescendantInfo{
			ID: ci.id, IsClaimed: ci.claimed, ClaimedBy: ci.author,
		})
		if ci.role == ticket.RoleEpic {
			sub, err := r.getDescendantsRecursive(ci.id)
			if err != nil {
				return nil, err
			}
			descendants = append(descendants, sub...)
		}
	}

	return descendants, nil
}

func (r *ticketRepo) HasChildren(_ context.Context, epicID ticket.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM tickets WHERE parent_id = ? AND deleted = 0`, epicID.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "has children", Err: err}
	}
	return count > 0, nil
}

func (r *ticketRepo) GetAncestorStatuses(_ context.Context, id ticket.ID) ([]ticket.AncestorStatus, error) {
	var ancestors []ticket.AncestorStatus
	current := id.String()

	for {
		var parentID string
		var found bool
		err := sqlitex.Execute(r.conn, `SELECT parent_id FROM tickets WHERE ticket_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
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
		err = sqlitex.Execute(r.conn, `SELECT state FROM tickets WHERE ticket_id = ? AND deleted = 0`, &sqlitex.ExecOptions{
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

		state, _ := ticket.ParseState(stateStr)
		ancestors = append(ancestors, ticket.AncestorStatus{State: state})
		current = parentID
	}

	return ancestors, nil
}

func (r *ticketRepo) GetParentID(_ context.Context, id ticket.ID) (ticket.ID, error) {
	var parentID string
	var found bool
	var isNull bool

	err := sqlitex.Execute(r.conn, `SELECT parent_id FROM tickets WHERE ticket_id = ?`, &sqlitex.ExecOptions{
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
		return ticket.ID{}, &domain.DatabaseError{Op: "get parent ID", Err: err}
	}
	if !found {
		return ticket.ID{}, domain.ErrNotFound
	}
	if isNull || parentID == "" {
		return ticket.ID{}, nil
	}
	return ticket.ParseID(parentID)
}

func (r *ticketRepo) TicketIDExists(_ context.Context, id ticket.ID) (bool, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM tickets WHERE ticket_id = ?`, id.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "check ticket exists", Err: err}
	}
	return count > 0, nil
}

func (r *ticketRepo) GetTicketByIdempotencyKey(_ context.Context, key string) (ticket.Ticket, error) {
	t, err := scanTicketRow(r.conn,
		`SELECT ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM tickets WHERE idempotency_key = ?`, key)
	if err != nil {
		if isNotFound(err) {
			return ticket.Ticket{}, domain.ErrNotFound
		}
		return ticket.Ticket{}, &domain.DatabaseError{Op: "get by idempotency key", Err: err}
	}
	return t, nil
}

func (r *ticketRepo) loadFacets(ticketID string) (ticket.FacetSet, error) {
	fs := ticket.NewFacetSet()
	err := sqlitex.Execute(r.conn, `SELECT key, value FROM facets WHERE ticket_id = ?`, &sqlitex.ExecOptions{
		Args: []any{ticketID},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			f, _ := ticket.NewFacet(stmt.ColumnText(0), stmt.ColumnText(1))
			fs = fs.Set(f)
			return nil
		},
	})
	if err != nil {
		return ticket.NewFacetSet(), &domain.DatabaseError{Op: "load facets", Err: err}
	}
	return fs, nil
}

// --- NoteRepository ---

type noteRepo struct{ conn *sqlite.Conn }

func (r *noteRepo) CreateNote(_ context.Context, n note.Note) (int64, error) {
	err := sqlitex.Execute(r.conn, `INSERT INTO notes (ticket_id, author, created_at, body) VALUES (?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{n.TicketID().String(), n.Author().String(), n.CreatedAt().Format(time.RFC3339Nano), n.Body()},
	})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "create note", Err: err}
	}

	noteID := r.conn.LastInsertRowID()

	// FTS sync.
	_ = sqlitex.Execute(r.conn, `INSERT INTO notes_fts (note_id, body) VALUES (?, ?)`, &sqlitex.ExecOptions{
		Args: []any{noteID, n.Body()},
	})

	return noteID, nil
}

func (r *noteRepo) GetNote(_ context.Context, id int64) (note.Note, error) {
	var result note.Note
	var found bool

	err := sqlitex.Execute(r.conn, `SELECT ticket_id, author, created_at, body FROM notes WHERE note_id = ?`, &sqlitex.ExecOptions{
		Args: []any{id},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			ticketID, _ := ticket.ParseID(stmt.ColumnText(0))
			author, _ := identity.NewAuthor(stmt.ColumnText(1))
			createdAt, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(2))

			var err error
			result, err = note.NewNote(note.NewNoteParams{
				ID: id, TicketID: ticketID, Author: author, CreatedAt: createdAt, Body: stmt.ColumnText(3),
			})
			return err
		},
	})
	if err != nil {
		return note.Note{}, &domain.DatabaseError{Op: "get note", Err: err}
	}
	if !found {
		return note.Note{}, domain.ErrNotFound
	}
	return result, nil
}

func (r *noteRepo) ListNotes(_ context.Context, ticketID ticket.ID, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
	page = page.Normalize()

	where := `WHERE ticket_id = ?`
	args := []any{ticketID.String()}

	if !filter.Author.IsZero() {
		where += ` AND author = ?`
		args = append(args, filter.Author.String())
	}
	if !filter.CreatedAfter.IsZero() {
		where += ` AND created_at > ?`
		args = append(args, filter.CreatedAfter.Format(time.RFC3339Nano))
	}
	if filter.AfterNoteID > 0 {
		where += ` AND note_id > ?`
		args = append(args, filter.AfterNoteID)
	}

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM notes `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count notes", Err: err}
	}

	query := `SELECT note_id, ticket_id, author, created_at, body FROM notes ` + where + ` ORDER BY note_id LIMIT ?`
	args = append(args, page.PageSize)

	var notes []note.Note
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := ticket.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := note.NewNote(note.NewNoteParams{
				ID: stmt.ColumnInt64(0), TicketID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			notes = append(notes, n)
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list notes", Err: err}
	}

	return notes, port.PageResult{TotalCount: total}, nil
}

func (r *noteRepo) SearchNotes(_ context.Context, query string, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
	page = page.Normalize()

	where := `WHERE n.note_id IN (SELECT note_id FROM notes_fts WHERE notes_fts MATCH ?)`
	args := []any{query}

	if !filter.TicketID.IsZero() {
		where += ` AND n.ticket_id = ?`
		args = append(args, filter.TicketID.String())
	}

	total, err := queryInt(r.conn, `SELECT COUNT(*) FROM notes n `+where, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count note search results", Err: err}
	}

	selectQuery := `SELECT n.note_id, n.ticket_id, n.author, n.created_at, n.body FROM notes n ` + where + ` ORDER BY n.note_id LIMIT ?`
	args = append(args, page.PageSize)

	var notes []note.Note
	err = sqlitex.Execute(r.conn, selectQuery, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := ticket.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(2))
			created, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(3))
			n, _ := note.NewNote(note.NewNoteParams{
				ID: stmt.ColumnInt64(0), TicketID: tid, Author: author, CreatedAt: created, Body: stmt.ColumnText(4),
			})
			notes = append(notes, n)
			return nil
		},
	})
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search notes", Err: err}
	}

	return notes, port.PageResult{TotalCount: total}, nil
}

// --- ClaimRepository ---

type claimRepo struct{ conn *sqlite.Conn }

func (r *claimRepo) CreateClaim(_ context.Context, c claim.Claim) error {
	err := sqlitex.Execute(r.conn, `INSERT OR REPLACE INTO claims (claim_id, ticket_id, author, stale_threshold, last_activity) VALUES (?, ?, ?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{c.ID(), c.TicketID().String(), c.Author().String(), int64(c.StaleThreshold()), c.LastActivity().Format(time.RFC3339Nano)},
	})
	if err != nil {
		return &domain.DatabaseError{Op: "create claim", Err: err}
	}
	return nil
}

func (r *claimRepo) GetClaimByTicket(_ context.Context, ticketID ticket.ID) (claim.Claim, error) {
	return r.scanClaim(`SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims WHERE ticket_id = ?`, ticketID.String())
}

func (r *claimRepo) GetClaimByID(_ context.Context, claimID string) (claim.Claim, error) {
	return r.scanClaim(`SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims WHERE claim_id = ?`, claimID)
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
	err := sqlitex.Execute(r.conn, `SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := ticket.ParseID(stmt.ColumnText(1))
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
			tid, _ := ticket.ParseID(stmt.ColumnText(1))
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

func (r *relRepo) CreateRelationship(_ context.Context, rel ticket.Relationship) (bool, error) {
	err := sqlitex.Execute(r.conn, `INSERT OR IGNORE INTO relationships (source_id, target_id, rel_type) VALUES (?, ?, ?)`, &sqlitex.ExecOptions{
		Args: []any{rel.SourceID().String(), rel.TargetID().String(), rel.Type().String()},
	})
	if err != nil {
		return false, &domain.DatabaseError{Op: "create relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) DeleteRelationship(_ context.Context, sourceID, targetID ticket.ID, relType ticket.RelationType) (bool, error) {
	err := sqlitex.Execute(r.conn, `DELETE FROM relationships WHERE source_id = ? AND target_id = ? AND rel_type = ?`, &sqlitex.ExecOptions{
		Args: []any{sourceID.String(), targetID.String(), relType.String()},
	})
	if err != nil {
		return false, &domain.DatabaseError{Op: "delete relationship", Err: err}
	}
	return r.conn.Changes() > 0, nil
}

func (r *relRepo) ListRelationships(_ context.Context, ticketID ticket.ID) ([]ticket.Relationship, error) {
	var rels []ticket.Relationship
	err := sqlitex.Execute(r.conn, `SELECT source_id, target_id, rel_type FROM relationships WHERE source_id = ? OR target_id = ?`, &sqlitex.ExecOptions{
		Args: []any{ticketID.String(), ticketID.String()},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			src, _ := ticket.ParseID(stmt.ColumnText(0))
			tgt, _ := ticket.ParseID(stmt.ColumnText(1))
			rt, _ := ticket.ParseRelationType(stmt.ColumnText(2))
			rel, _ := ticket.NewRelationship(src, tgt, rt)
			rels = append(rels, rel)
			return nil
		},
	})
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list relationships", Err: err}
	}
	return rels, nil
}

func (r *relRepo) GetBlockerStatuses(_ context.Context, ticketID ticket.ID) ([]ticket.BlockerStatus, error) {
	var statuses []ticket.BlockerStatus
	err := sqlitex.Execute(r.conn,
		`SELECT t.state, t.deleted FROM relationships r JOIN tickets t ON r.target_id = t.ticket_id WHERE r.source_id = ? AND r.rel_type = 'blocked_by'`,
		&sqlitex.ExecOptions{
			Args: []any{ticketID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				state, _ := ticket.ParseState(stmt.ColumnText(0))
				statuses = append(statuses, ticket.BlockerStatus{
					IsClosed:  state == ticket.StateClosed,
					IsDeleted: stmt.ColumnInt(1) != 0,
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
		`INSERT INTO history (ticket_id, revision, author, timestamp, event_type, changes) VALUES (?, ?, ?, ?, ?, ?)`,
		&sqlitex.ExecOptions{
			Args: []any{
				entry.TicketID().String(), entry.Revision(), entry.Author().String(),
				entry.Timestamp().Format(time.RFC3339Nano), entry.EventType().String(), string(changesJSON),
			},
		})
	if err != nil {
		return 0, &domain.DatabaseError{Op: "append history", Err: err}
	}
	return r.conn.LastInsertRowID(), nil
}

func (r *histRepo) ListHistory(_ context.Context, ticketID ticket.ID, filter port.HistoryFilter, page port.PageRequest) ([]history.Entry, port.PageResult, error) {
	page = page.Normalize()

	where := `WHERE ticket_id = ?`
	args := []any{ticketID.String()}

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

	query := `SELECT entry_id, ticket_id, revision, author, timestamp, event_type, changes FROM history ` + where + ` ORDER BY revision LIMIT ?`
	args = append(args, page.PageSize)

	var entries []history.Entry
	err = sqlitex.Execute(r.conn, query, &sqlitex.ExecOptions{
		Args: args,
		ResultFunc: func(stmt *sqlite.Stmt) error {
			tid, _ := ticket.ParseID(stmt.ColumnText(1))
			author, _ := identity.NewAuthor(stmt.ColumnText(3))
			ts, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
			eventType, _ := history.ParseEventType(stmt.ColumnText(5))

			var changes []history.FieldChange
			_ = json.Unmarshal([]byte(stmt.ColumnText(6)), &changes)

			entries = append(entries, history.NewEntry(history.NewEntryParams{
				ID: stmt.ColumnInt64(0), TicketID: tid, Revision: stmt.ColumnInt(2), Author: author,
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

func (r *histRepo) CountHistory(_ context.Context, ticketID ticket.ID) (int, error) {
	count, err := queryInt(r.conn, `SELECT COUNT(*) FROM history WHERE ticket_id = ?`, ticketID.String())
	if err != nil {
		return 0, &domain.DatabaseError{Op: "count history", Err: err}
	}
	return count, nil
}

func (r *histRepo) GetLatestHistory(_ context.Context, ticketID ticket.ID) (history.Entry, error) {
	var result history.Entry
	var found bool

	err := sqlitex.Execute(r.conn,
		`SELECT entry_id, ticket_id, revision, author, timestamp, event_type, changes FROM history WHERE ticket_id = ? ORDER BY revision DESC LIMIT 1`,
		&sqlitex.ExecOptions{
			Args: []any{ticketID.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				tid, _ := ticket.ParseID(stmt.ColumnText(1))
				author, _ := identity.NewAuthor(stmt.ColumnText(3))
				ts, _ := time.Parse(time.RFC3339Nano, stmt.ColumnText(4))
				eventType, _ := history.ParseEventType(stmt.ColumnText(5))

				var changes []history.FieldChange
				_ = json.Unmarshal([]byte(stmt.ColumnText(6)), &changes)

				result = history.NewEntry(history.NewEntryParams{
					ID: stmt.ColumnInt64(0), TicketID: tid, Revision: stmt.ColumnInt(2), Author: author,
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

// scanTicketRow executes a query expected to return a single ticket row and
// scans it into a domain Ticket.
func scanTicketRow(conn *sqlite.Conn, query string, args ...any) (ticket.Ticket, error) {
	var t ticket.Ticket
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

			id, _ := ticket.ParseID(idStr)
			priority, _ := ticket.ParsePriority(priorityStr)
			state, _ := ticket.ParseState(stateStr)
			createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

			var pid ticket.ID
			if parentIDStr != "" {
				pid, _ = ticket.ParseID(parentIDStr)
			}

			role, _ := ticket.ParseRole(roleStr)

			switch role {
			case ticket.RoleTask:
				t, _ = ticket.NewTask(ticket.NewTaskParams{
					ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
					Priority: priority, ParentID: pid, CreatedAt: createdAt,
					IdempotencyKey: idemKey,
				})
			case ticket.RoleEpic:
				t, _ = ticket.NewEpic(ticket.NewEpicParams{
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
		return ticket.Ticket{}, err
	}
	if !found {
		return ticket.Ticket{}, errNotFound
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

// nullable converts a ticket.ID to a value suitable for SQL binding. Returns
// nil (which binds as NULL) for zero IDs, or the string representation
// otherwise.
func nullable(id ticket.ID) any {
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

func buildTicketWhere(filter port.TicketFilter) (string, []any) {
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

	if !filter.ParentID.IsZero() {
		conditions = append(conditions, "t.parent_id = ?")
		args = append(args, filter.ParentID.String())
	}

	if !filter.DescendantsOf.IsZero() {
		// Recursive CTE walks the parent_id chain to find all descendants.
		conditions = append(conditions, `t.ticket_id IN (
			WITH RECURSIVE desc(tid) AS (
				SELECT ticket_id FROM tickets WHERE parent_id = ?
				UNION ALL
				SELECT c.ticket_id FROM tickets c JOIN desc d ON c.parent_id = d.tid
			)
			SELECT tid FROM desc
		)`)
		args = append(args, filter.DescendantsOf.String())
	}

	if !filter.AncestorsOf.IsZero() {
		// Recursive CTE walks up the parent chain to find all ancestors.
		conditions = append(conditions, `t.ticket_id IN (
			WITH RECURSIVE anc(aid) AS (
				SELECT parent_id FROM tickets WHERE ticket_id = ?
				UNION ALL
				SELECT p.parent_id FROM tickets p JOIN anc a ON p.ticket_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT aid FROM anc WHERE aid IS NOT NULL
		)`)
		args = append(args, filter.AncestorsOf.String())
	}

	for _, ff := range filter.FacetFilters {
		if ff.Negate {
			if ff.Value == "" {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM facets f WHERE f.ticket_id = t.ticket_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `NOT EXISTS (SELECT 1 FROM facets f WHERE f.ticket_id = t.ticket_id AND f.key = ? AND f.value = ?)`)
				args = append(args, ff.Key, ff.Value)
				continue
			}
		} else {
			if ff.Value == "" {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM facets f WHERE f.ticket_id = t.ticket_id AND f.key = ?)`)
			} else {
				conditions = append(conditions, `EXISTS (SELECT 1 FROM facets f WHERE f.ticket_id = t.ticket_id AND f.key = ? AND f.value = ?)`)
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
		// State: tasks must be open, epics must be active.
		conditions = append(conditions, `((t.role = 'task' AND t.state = 'open') OR (t.role = 'epic' AND t.state = 'active'))`)

		// Epics with children are already decomposed — not ready.
		conditions = append(conditions, `(t.role = 'task' OR NOT EXISTS (
			SELECT 1 FROM tickets c WHERE c.parent_id = t.ticket_id AND c.deleted = 0
		))`)

		// No unresolved blocked_by relationships. A blocker is resolved if its
		// target ticket is closed or deleted.
		conditions = append(conditions, `NOT EXISTS (
			SELECT 1 FROM relationships r
			JOIN tickets bt ON r.target_id = bt.ticket_id
			WHERE r.source_id = t.ticket_id
			  AND r.rel_type = 'blocked_by'
			  AND bt.state != 'closed'
			  AND bt.deleted = 0
		)`)

		// No ancestor epic is deferred or waiting. Walk the parent chain with
		// a recursive CTE and reject tickets that have any such ancestor.
		conditions = append(conditions, `NOT EXISTS (
			WITH RECURSIVE ancestors(aid) AS (
				SELECT t.parent_id
				UNION ALL
				SELECT p.parent_id FROM tickets p JOIN ancestors a ON p.ticket_id = a.aid WHERE p.parent_id IS NOT NULL
			)
			SELECT 1 FROM ancestors a
			JOIN tickets anc ON anc.ticket_id = a.aid
			WHERE anc.state IN ('deferred', 'waiting')
		)`)
	}

	return "WHERE " + strings.Join(conditions, " AND "), args
}

func ticketOrderClause(orderBy port.TicketOrderBy) string {
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
