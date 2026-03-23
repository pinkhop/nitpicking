package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"

	_ "modernc.org/sqlite"
)

// Store provides SQLite-backed persistence for the nitpicking domain.
type Store struct {
	db *sql.DB
}

// Open opens or creates a SQLite database at the given path.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, &domain.DatabaseError{Op: "open database", Err: err}
	}

	// Apply schema.
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, &domain.DatabaseError{Op: "apply schema", Err: err}
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// --- Transactor ---

// WithTransaction executes fn within a database transaction.
func (s *Store) WithTransaction(ctx context.Context, fn func(uow port.UnitOfWork) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return &domain.DatabaseError{Op: "begin transaction", Err: err}
	}

	uow := &sqlUnitOfWork{tx: tx}
	if err := fn(uow); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return &domain.DatabaseError{Op: "commit transaction", Err: err}
	}
	return nil
}

// WithReadTransaction executes fn within a read-only transaction.
func (s *Store) WithReadTransaction(ctx context.Context, fn func(uow port.UnitOfWork) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return &domain.DatabaseError{Op: "begin read transaction", Err: err}
	}

	uow := &sqlUnitOfWork{tx: tx}
	if err := fn(uow); err != nil {
		_ = tx.Rollback()
		return err
	}

	_ = tx.Rollback() // Read-only — no commit needed.
	return nil
}

// sqlUnitOfWork wraps a sql.Tx to implement port.UnitOfWork.
type sqlUnitOfWork struct {
	tx *sql.Tx
}

func (u *sqlUnitOfWork) Tickets() port.TicketRepository             { return &ticketRepo{tx: u.tx} }
func (u *sqlUnitOfWork) Notes() port.NoteRepository                 { return &noteRepo{tx: u.tx} }
func (u *sqlUnitOfWork) Claims() port.ClaimRepository               { return &claimRepo{tx: u.tx} }
func (u *sqlUnitOfWork) Relationships() port.RelationshipRepository { return &relRepo{tx: u.tx} }
func (u *sqlUnitOfWork) History() port.HistoryRepository            { return &histRepo{tx: u.tx} }
func (u *sqlUnitOfWork) Database() port.DatabaseRepository          { return &dbRepo{tx: u.tx} }

// --- DatabaseRepository ---

type dbRepo struct{ tx *sql.Tx }

func (r *dbRepo) InitDatabase(ctx context.Context, prefix string) error {
	_, err := r.tx.ExecContext(ctx, `INSERT INTO metadata (key, value) VALUES ('prefix', ?)`, prefix)
	if err != nil {
		return &domain.DatabaseError{Op: "init database", Err: err}
	}
	return nil
}

func (r *dbRepo) GetPrefix(ctx context.Context) (string, error) {
	var prefix string
	err := r.tx.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'prefix'`).Scan(&prefix)
	if err != nil {
		return "", &domain.DatabaseError{Op: "get prefix", Err: err}
	}
	return prefix, nil
}

func (r *dbRepo) GC(ctx context.Context, includeClosed bool) error {
	// Delete related data for deleted tickets.
	_, err := r.tx.ExecContext(ctx, `DELETE FROM facets WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`)
	if err != nil {
		return &domain.DatabaseError{Op: "gc facets", Err: err}
	}

	_, err = r.tx.ExecContext(ctx, `DELETE FROM notes WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`)
	if err != nil {
		return &domain.DatabaseError{Op: "gc notes", Err: err}
	}

	_, err = r.tx.ExecContext(ctx, `DELETE FROM history WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`)
	if err != nil {
		return &domain.DatabaseError{Op: "gc history", Err: err}
	}

	_, err = r.tx.ExecContext(ctx, `DELETE FROM relationships WHERE source_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1) OR target_id IN (SELECT ticket_id FROM tickets WHERE deleted = 1)`)
	if err != nil {
		return &domain.DatabaseError{Op: "gc relationships", Err: err}
	}

	_, err = r.tx.ExecContext(ctx, `DELETE FROM tickets WHERE deleted = 1`)
	if err != nil {
		return &domain.DatabaseError{Op: "gc tickets", Err: err}
	}

	if includeClosed {
		_, err = r.tx.ExecContext(ctx, `DELETE FROM facets WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`)
		if err != nil {
			return &domain.DatabaseError{Op: "gc closed facets", Err: err}
		}

		_, err = r.tx.ExecContext(ctx, `DELETE FROM notes WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`)
		if err != nil {
			return &domain.DatabaseError{Op: "gc closed notes", Err: err}
		}

		_, err = r.tx.ExecContext(ctx, `DELETE FROM history WHERE ticket_id IN (SELECT ticket_id FROM tickets WHERE state = 'closed')`)
		if err != nil {
			return &domain.DatabaseError{Op: "gc closed history", Err: err}
		}

		_, err = r.tx.ExecContext(ctx, `DELETE FROM tickets WHERE state = 'closed'`)
		if err != nil {
			return &domain.DatabaseError{Op: "gc closed tickets", Err: err}
		}
	}

	return nil
}

// --- TicketRepository ---

type ticketRepo struct{ tx *sql.Tx }

func (r *ticketRepo) CreateTicket(ctx context.Context, t ticket.Ticket) error {
	var parentID *string
	if !t.ParentID().IsZero() {
		s := t.ParentID().String()
		parentID = &s
	}

	var idemKey *string
	if t.IdempotencyKey() != "" {
		idemKey = new(string)
		*idemKey = t.IdempotencyKey()
	}

	_, err := r.tx.ExecContext(ctx, `INSERT INTO tickets (ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID().String(), t.Role().String(), t.Title(), t.Description(), t.AcceptanceCriteria(),
		t.Priority().String(), t.State().String(), parentID, t.CreatedAt().Format(time.RFC3339Nano),
		idemKey, boolToInt(t.IsDeleted()))
	if err != nil {
		return &domain.DatabaseError{Op: "create ticket", Err: err}
	}

	// Save facets.
	for k, v := range t.Facets().All() {
		_, err := r.tx.ExecContext(ctx, `INSERT INTO facets (ticket_id, key, value) VALUES (?, ?, ?)`, t.ID().String(), k, v)
		if err != nil {
			return &domain.DatabaseError{Op: "create facet", Err: err}
		}
	}

	return nil
}

func (r *ticketRepo) GetTicket(ctx context.Context, id ticket.ID, includeDeleted bool) (ticket.Ticket, error) {
	query := `SELECT ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM tickets WHERE ticket_id = ?`
	if !includeDeleted {
		query += ` AND deleted = 0`
	}

	row := r.tx.QueryRowContext(ctx, query, id.String())
	t, err := scanTicket(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ticket.Ticket{}, domain.ErrNotFound
		}
		return ticket.Ticket{}, &domain.DatabaseError{Op: "get ticket", Err: err}
	}

	// Load facets.
	facets, err := r.loadFacets(ctx, id.String())
	if err != nil {
		return ticket.Ticket{}, err
	}
	t = t.WithFacets(facets)

	return t, nil
}

func (r *ticketRepo) UpdateTicket(ctx context.Context, t ticket.Ticket) error {
	var parentID *string
	if !t.ParentID().IsZero() {
		s := t.ParentID().String()
		parentID = &s
	}

	_, err := r.tx.ExecContext(ctx, `UPDATE tickets SET title = ?, description = ?, acceptance_criteria = ?, priority = ?, state = ?, parent_id = ?, deleted = ? WHERE ticket_id = ?`,
		t.Title(), t.Description(), t.AcceptanceCriteria(), t.Priority().String(),
		t.State().String(), parentID, boolToInt(t.IsDeleted()), t.ID().String())
	if err != nil {
		return &domain.DatabaseError{Op: "update ticket", Err: err}
	}

	// Replace facets.
	_, _ = r.tx.ExecContext(ctx, `DELETE FROM facets WHERE ticket_id = ?`, t.ID().String())
	for k, v := range t.Facets().All() {
		_, err := r.tx.ExecContext(ctx, `INSERT INTO facets (ticket_id, key, value) VALUES (?, ?, ?)`, t.ID().String(), k, v)
		if err != nil {
			return &domain.DatabaseError{Op: "update facet", Err: err}
		}
	}

	return nil
}

func (r *ticketRepo) ListTickets(ctx context.Context, filter port.TicketFilter, orderBy port.TicketOrderBy, page port.PageRequest) ([]port.TicketListItem, port.PageResult, error) {
	page = page.Normalize()

	where, args := buildTicketWhere(filter)

	// Count total.
	var total int
	countQuery := `SELECT COUNT(*) FROM tickets t ` + where
	if err := r.tx.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count tickets", Err: err}
	}

	orderClause := ticketOrderClause(orderBy)
	query := `SELECT t.ticket_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted FROM tickets t ` + where + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	rows, err := r.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list tickets", Err: err}
	}
	defer rows.Close()

	var items []port.TicketListItem
	for rows.Next() {
		var idStr, roleStr, stateStr, priorityStr, title, createdAtStr string
		var deleted int
		if err := rows.Scan(&idStr, &roleStr, &stateStr, &priorityStr, &title, &createdAtStr, &deleted); err != nil {
			return nil, port.PageResult{}, &domain.DatabaseError{Op: "scan ticket", Err: err}
		}

		id, _ := ticket.ParseID(idStr)
		role, _ := ticket.ParseRole(roleStr)
		state, _ := ticket.ParseState(stateStr)
		priority, _ := ticket.ParsePriority(priorityStr)
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

		items = append(items, port.TicketListItem{
			ID:        id,
			Role:      role,
			State:     state,
			Priority:  priority,
			Title:     title,
			CreatedAt: createdAt,
			IsDeleted: deleted != 0,
		})
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *ticketRepo) SearchTickets(ctx context.Context, query string, filter port.TicketFilter, orderBy port.TicketOrderBy, page port.PageRequest) ([]port.TicketListItem, port.PageResult, error) {
	page = page.Normalize()

	where, args := buildTicketWhere(filter)

	// Add FTS condition.
	ftsWhere := ` AND t.ticket_id IN (SELECT ticket_id FROM tickets_fts WHERE tickets_fts MATCH ?)`
	args = append(args, query)

	var total int
	countQuery := `SELECT COUNT(*) FROM tickets t ` + where + ftsWhere
	if err := r.tx.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "count search results", Err: err}
	}

	orderClause := ticketOrderClause(orderBy)
	selectQuery := `SELECT t.ticket_id, t.role, t.state, t.priority, t.title, t.created_at, t.deleted FROM tickets t ` + where + ftsWhere + orderClause + ` LIMIT ?`
	args = append(args, page.PageSize)

	rows, err := r.tx.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search tickets", Err: err}
	}
	defer rows.Close()

	var items []port.TicketListItem
	for rows.Next() {
		var idStr, roleStr, stateStr, priorityStr, title, createdAtStr string
		var deleted int
		if err := rows.Scan(&idStr, &roleStr, &stateStr, &priorityStr, &title, &createdAtStr, &deleted); err != nil {
			return nil, port.PageResult{}, &domain.DatabaseError{Op: "scan search result", Err: err}
		}
		id, _ := ticket.ParseID(idStr)
		role, _ := ticket.ParseRole(roleStr)
		state, _ := ticket.ParseState(stateStr)
		priority, _ := ticket.ParsePriority(priorityStr)
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		items = append(items, port.TicketListItem{
			ID: id, Role: role, State: state, Priority: priority,
			Title: title, CreatedAt: createdAt, IsDeleted: deleted != 0,
		})
	}

	return items, port.PageResult{TotalCount: total}, nil
}

func (r *ticketRepo) GetChildStatuses(ctx context.Context, epicID ticket.ID) ([]ticket.ChildStatus, error) {
	rows, err := r.tx.QueryContext(ctx, `SELECT role, state FROM tickets WHERE parent_id = ? AND deleted = 0`, epicID.String())
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get child statuses", Err: err}
	}
	defer rows.Close()

	var children []ticket.ChildStatus
	for rows.Next() {
		var roleStr, stateStr string
		if err := rows.Scan(&roleStr, &stateStr); err != nil {
			return nil, &domain.DatabaseError{Op: "scan child status", Err: err}
		}
		role, _ := ticket.ParseRole(roleStr)
		state, _ := ticket.ParseState(stateStr)
		children = append(children, ticket.ChildStatus{
			Role:       role,
			State:      state,
			IsComplete: state == ticket.StateClosed,
		})
	}
	return children, nil
}

func (r *ticketRepo) GetDescendants(ctx context.Context, epicID ticket.ID) ([]ticket.DescendantInfo, error) {
	return r.getDescendantsRecursive(ctx, epicID)
}

func (r *ticketRepo) getDescendantsRecursive(ctx context.Context, parentID ticket.ID) ([]ticket.DescendantInfo, error) {
	rows, err := r.tx.QueryContext(ctx,
		`SELECT t.ticket_id, t.role, COALESCE(c.author, '') as claim_author FROM tickets t LEFT JOIN claims c ON t.ticket_id = c.ticket_id WHERE t.parent_id = ? AND t.deleted = 0`,
		parentID.String())
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get descendants", Err: err}
	}
	defer rows.Close()

	var descendants []ticket.DescendantInfo
	type childInfo struct {
		id      ticket.ID
		role    ticket.Role
		claimed bool
		author  string
	}
	var childInfos []childInfo

	for rows.Next() {
		var idStr, roleStr, claimAuthor string
		if err := rows.Scan(&idStr, &roleStr, &claimAuthor); err != nil {
			return nil, &domain.DatabaseError{Op: "scan descendant", Err: err}
		}
		id, _ := ticket.ParseID(idStr)
		role, _ := ticket.ParseRole(roleStr)
		childInfos = append(childInfos, childInfo{id: id, role: role, claimed: claimAuthor != "", author: claimAuthor})
	}

	for _, ci := range childInfos {
		descendants = append(descendants, ticket.DescendantInfo{
			ID: ci.id, IsClaimed: ci.claimed, ClaimedBy: ci.author,
		})
		if ci.role == ticket.RoleEpic {
			sub, err := r.getDescendantsRecursive(ctx, ci.id)
			if err != nil {
				return nil, err
			}
			descendants = append(descendants, sub...)
		}
	}

	return descendants, nil
}

func (r *ticketRepo) HasChildren(ctx context.Context, epicID ticket.ID) (bool, error) {
	var count int
	err := r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets WHERE parent_id = ? AND deleted = 0`, epicID.String()).Scan(&count)
	if err != nil {
		return false, &domain.DatabaseError{Op: "has children", Err: err}
	}
	return count > 0, nil
}

func (r *ticketRepo) GetAncestorStatuses(ctx context.Context, id ticket.ID) ([]ticket.AncestorStatus, error) {
	var ancestors []ticket.AncestorStatus
	current := id.String()

	for {
		var parentID sql.NullString
		err := r.tx.QueryRowContext(ctx, `SELECT parent_id FROM tickets WHERE ticket_id = ? AND deleted = 0`, current).Scan(&parentID)
		if err != nil || !parentID.Valid || parentID.String == "" {
			break
		}

		var stateStr string
		err = r.tx.QueryRowContext(ctx, `SELECT state FROM tickets WHERE ticket_id = ? AND deleted = 0`, parentID.String).Scan(&stateStr)
		if err != nil {
			break
		}

		state, _ := ticket.ParseState(stateStr)
		ancestors = append(ancestors, ticket.AncestorStatus{State: state})
		current = parentID.String
	}

	return ancestors, nil
}

func (r *ticketRepo) GetParentID(ctx context.Context, id ticket.ID) (ticket.ID, error) {
	var parentID sql.NullString
	err := r.tx.QueryRowContext(ctx, `SELECT parent_id FROM tickets WHERE ticket_id = ?`, id.String()).Scan(&parentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return ticket.ID{}, domain.ErrNotFound
		}
		return ticket.ID{}, &domain.DatabaseError{Op: "get parent ID", Err: err}
	}
	if !parentID.Valid || parentID.String == "" {
		return ticket.ID{}, nil
	}
	return ticket.ParseID(parentID.String)
}

func (r *ticketRepo) TicketIDExists(ctx context.Context, id ticket.ID) (bool, error) {
	var count int
	err := r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets WHERE ticket_id = ?`, id.String()).Scan(&count)
	if err != nil {
		return false, &domain.DatabaseError{Op: "check ticket exists", Err: err}
	}
	return count > 0, nil
}

func (r *ticketRepo) GetTicketByIdempotencyKey(ctx context.Context, key string) (ticket.Ticket, error) {
	row := r.tx.QueryRowContext(ctx,
		`SELECT ticket_id, role, title, description, acceptance_criteria, priority, state, parent_id, created_at, idempotency_key, deleted FROM tickets WHERE idempotency_key = ?`, key)
	t, err := scanTicket(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return ticket.Ticket{}, domain.ErrNotFound
		}
		return ticket.Ticket{}, &domain.DatabaseError{Op: "get by idempotency key", Err: err}
	}
	return t, nil
}

func (r *ticketRepo) loadFacets(ctx context.Context, ticketID string) (ticket.FacetSet, error) {
	rows, err := r.tx.QueryContext(ctx, `SELECT key, value FROM facets WHERE ticket_id = ?`, ticketID)
	if err != nil {
		return ticket.NewFacetSet(), &domain.DatabaseError{Op: "load facets", Err: err}
	}
	defer rows.Close()

	fs := ticket.NewFacetSet()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return ticket.NewFacetSet(), &domain.DatabaseError{Op: "scan facet", Err: err}
		}
		f, _ := ticket.NewFacet(k, v)
		fs = fs.Set(f)
	}
	return fs, nil
}

// --- NoteRepository ---

type noteRepo struct{ tx *sql.Tx }

func (r *noteRepo) CreateNote(ctx context.Context, n note.Note) (int64, error) {
	result, err := r.tx.ExecContext(ctx, `INSERT INTO notes (ticket_id, author, created_at, body) VALUES (?, ?, ?, ?)`,
		n.TicketID().String(), n.Author().String(), n.CreatedAt().Format(time.RFC3339Nano), n.Body())
	if err != nil {
		return 0, &domain.DatabaseError{Op: "create note", Err: err}
	}
	return result.LastInsertId()
}

func (r *noteRepo) GetNote(ctx context.Context, id int64) (note.Note, error) {
	var ticketIDStr, authorStr, createdAtStr, body string
	err := r.tx.QueryRowContext(ctx, `SELECT ticket_id, author, created_at, body FROM notes WHERE note_id = ?`, id).
		Scan(&ticketIDStr, &authorStr, &createdAtStr, &body)
	if err != nil {
		if err == sql.ErrNoRows {
			return note.Note{}, domain.ErrNotFound
		}
		return note.Note{}, &domain.DatabaseError{Op: "get note", Err: err}
	}

	ticketID, _ := ticket.ParseID(ticketIDStr)
	author, _ := identity.NewAuthor(authorStr)
	createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

	return note.NewNote(note.NewNoteParams{
		ID: id, TicketID: ticketID, Author: author, CreatedAt: createdAt, Body: body,
	})
}

func (r *noteRepo) ListNotes(ctx context.Context, ticketID ticket.ID, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
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

	var total int
	r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes `+where, args...).Scan(&total)

	query := `SELECT note_id, ticket_id, author, created_at, body FROM notes ` + where + ` ORDER BY note_id LIMIT ?`
	args = append(args, page.PageSize)

	rows, err := r.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list notes", Err: err}
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		var noteID int64
		var tidStr, authorStr, createdStr, body string
		if err := rows.Scan(&noteID, &tidStr, &authorStr, &createdStr, &body); err != nil {
			return nil, port.PageResult{}, &domain.DatabaseError{Op: "scan note", Err: err}
		}
		tid, _ := ticket.ParseID(tidStr)
		author, _ := identity.NewAuthor(authorStr)
		created, _ := time.Parse(time.RFC3339Nano, createdStr)
		n, _ := note.NewNote(note.NewNoteParams{ID: noteID, TicketID: tid, Author: author, CreatedAt: created, Body: body})
		notes = append(notes, n)
	}

	return notes, port.PageResult{TotalCount: total}, nil
}

func (r *noteRepo) SearchNotes(ctx context.Context, query string, filter port.NoteFilter, page port.PageRequest) ([]note.Note, port.PageResult, error) {
	page = page.Normalize()

	where := `WHERE n.note_id IN (SELECT note_id FROM notes_fts WHERE notes_fts MATCH ?)`
	args := []any{query}

	if !filter.TicketID.IsZero() {
		where += ` AND n.ticket_id = ?`
		args = append(args, filter.TicketID.String())
	}

	var total int
	r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes n `+where, args...).Scan(&total)

	selectQuery := `SELECT n.note_id, n.ticket_id, n.author, n.created_at, n.body FROM notes n ` + where + ` ORDER BY n.note_id LIMIT ?`
	args = append(args, page.PageSize)

	rows, err := r.tx.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "search notes", Err: err}
	}
	defer rows.Close()

	var notes []note.Note
	for rows.Next() {
		var noteID int64
		var tidStr, authorStr, createdStr, body string
		if err := rows.Scan(&noteID, &tidStr, &authorStr, &createdStr, &body); err != nil {
			return nil, port.PageResult{}, &domain.DatabaseError{Op: "scan note", Err: err}
		}
		tid, _ := ticket.ParseID(tidStr)
		author, _ := identity.NewAuthor(authorStr)
		created, _ := time.Parse(time.RFC3339Nano, createdStr)
		n, _ := note.NewNote(note.NewNoteParams{ID: noteID, TicketID: tid, Author: author, CreatedAt: created, Body: body})
		notes = append(notes, n)
	}

	return notes, port.PageResult{TotalCount: total}, nil
}

// --- ClaimRepository ---

type claimRepo struct{ tx *sql.Tx }

func (r *claimRepo) CreateClaim(ctx context.Context, c claim.Claim) error {
	_, err := r.tx.ExecContext(ctx, `INSERT OR REPLACE INTO claims (claim_id, ticket_id, author, stale_threshold, last_activity) VALUES (?, ?, ?, ?, ?)`,
		c.ID(), c.TicketID().String(), c.Author().String(), int64(c.StaleThreshold()), c.LastActivity().Format(time.RFC3339Nano))
	if err != nil {
		return &domain.DatabaseError{Op: "create claim", Err: err}
	}
	return nil
}

func (r *claimRepo) GetClaimByTicket(ctx context.Context, ticketID ticket.ID) (claim.Claim, error) {
	return r.scanClaim(ctx, `SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims WHERE ticket_id = ?`, ticketID.String())
}

func (r *claimRepo) GetClaimByID(ctx context.Context, claimID string) (claim.Claim, error) {
	return r.scanClaim(ctx, `SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims WHERE claim_id = ?`, claimID)
}

func (r *claimRepo) InvalidateClaim(ctx context.Context, claimID string) error {
	result, err := r.tx.ExecContext(ctx, `DELETE FROM claims WHERE claim_id = ?`, claimID)
	if err != nil {
		return &domain.DatabaseError{Op: "invalidate claim", Err: err}
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *claimRepo) UpdateClaimLastActivity(ctx context.Context, claimID string, lastActivity time.Time) error {
	_, err := r.tx.ExecContext(ctx, `UPDATE claims SET last_activity = ? WHERE claim_id = ?`, lastActivity.Format(time.RFC3339Nano), claimID)
	if err != nil {
		return &domain.DatabaseError{Op: "update claim activity", Err: err}
	}
	return nil
}

func (r *claimRepo) UpdateClaimThreshold(ctx context.Context, claimID string, threshold time.Duration) error {
	_, err := r.tx.ExecContext(ctx, `UPDATE claims SET stale_threshold = ? WHERE claim_id = ?`, int64(threshold), claimID)
	if err != nil {
		return &domain.DatabaseError{Op: "update claim threshold", Err: err}
	}
	return nil
}

func (r *claimRepo) ListStaleClaims(ctx context.Context, now time.Time) ([]claim.Claim, error) {
	rows, err := r.tx.QueryContext(ctx, `SELECT claim_id, ticket_id, author, stale_threshold, last_activity FROM claims`)
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list stale claims", Err: err}
	}
	defer rows.Close()

	var stale []claim.Claim
	for rows.Next() {
		var claimID, tidStr, authorStr, lastActStr string
		var threshold int64
		if err := rows.Scan(&claimID, &tidStr, &authorStr, &threshold, &lastActStr); err != nil {
			return nil, &domain.DatabaseError{Op: "scan claim", Err: err}
		}
		tid, _ := ticket.ParseID(tidStr)
		author, _ := identity.NewAuthor(authorStr)
		lastAct, _ := time.Parse(time.RFC3339Nano, lastActStr)
		c := claim.ReconstructClaim(claimID, tid, author, time.Duration(threshold), lastAct)
		if c.IsStale(now) {
			stale = append(stale, c)
		}
	}
	return stale, nil
}

func (r *claimRepo) scanClaim(ctx context.Context, query string, args ...any) (claim.Claim, error) {
	var claimID, tidStr, authorStr, lastActStr string
	var threshold int64
	err := r.tx.QueryRowContext(ctx, query, args...).Scan(&claimID, &tidStr, &authorStr, &threshold, &lastActStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return claim.Claim{}, domain.ErrNotFound
		}
		return claim.Claim{}, &domain.DatabaseError{Op: "get claim", Err: err}
	}
	tid, _ := ticket.ParseID(tidStr)
	author, _ := identity.NewAuthor(authorStr)
	lastAct, _ := time.Parse(time.RFC3339Nano, lastActStr)
	return claim.ReconstructClaim(claimID, tid, author, time.Duration(threshold), lastAct), nil
}

// --- RelationshipRepository ---

type relRepo struct{ tx *sql.Tx }

func (r *relRepo) CreateRelationship(ctx context.Context, rel ticket.Relationship) (bool, error) {
	result, err := r.tx.ExecContext(ctx, `INSERT OR IGNORE INTO relationships (source_id, target_id, rel_type) VALUES (?, ?, ?)`,
		rel.SourceID().String(), rel.TargetID().String(), rel.Type().String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "create relationship", Err: err}
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func (r *relRepo) DeleteRelationship(ctx context.Context, sourceID, targetID ticket.ID, relType ticket.RelationType) (bool, error) {
	result, err := r.tx.ExecContext(ctx, `DELETE FROM relationships WHERE source_id = ? AND target_id = ? AND rel_type = ?`,
		sourceID.String(), targetID.String(), relType.String())
	if err != nil {
		return false, &domain.DatabaseError{Op: "delete relationship", Err: err}
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

func (r *relRepo) ListRelationships(ctx context.Context, ticketID ticket.ID) ([]ticket.Relationship, error) {
	rows, err := r.tx.QueryContext(ctx, `SELECT source_id, target_id, rel_type FROM relationships WHERE source_id = ? OR target_id = ?`,
		ticketID.String(), ticketID.String())
	if err != nil {
		return nil, &domain.DatabaseError{Op: "list relationships", Err: err}
	}
	defer rows.Close()

	var rels []ticket.Relationship
	for rows.Next() {
		var srcStr, tgtStr, typeStr string
		if err := rows.Scan(&srcStr, &tgtStr, &typeStr); err != nil {
			return nil, &domain.DatabaseError{Op: "scan relationship", Err: err}
		}
		src, _ := ticket.ParseID(srcStr)
		tgt, _ := ticket.ParseID(tgtStr)
		rt, _ := ticket.ParseRelationType(typeStr)
		rel, _ := ticket.NewRelationship(src, tgt, rt)
		rels = append(rels, rel)
	}
	return rels, nil
}

func (r *relRepo) GetBlockerStatuses(ctx context.Context, ticketID ticket.ID) ([]ticket.BlockerStatus, error) {
	rows, err := r.tx.QueryContext(ctx,
		`SELECT t.state, t.deleted FROM relationships r JOIN tickets t ON r.target_id = t.ticket_id WHERE r.source_id = ? AND r.rel_type = 'blocked_by'`,
		ticketID.String())
	if err != nil {
		return nil, &domain.DatabaseError{Op: "get blocker statuses", Err: err}
	}
	defer rows.Close()

	var statuses []ticket.BlockerStatus
	for rows.Next() {
		var stateStr string
		var deleted int
		if err := rows.Scan(&stateStr, &deleted); err != nil {
			return nil, &domain.DatabaseError{Op: "scan blocker", Err: err}
		}
		state, _ := ticket.ParseState(stateStr)
		statuses = append(statuses, ticket.BlockerStatus{
			IsClosed:  state == ticket.StateClosed,
			IsDeleted: deleted != 0,
		})
	}
	return statuses, nil
}

// --- HistoryRepository ---

type histRepo struct{ tx *sql.Tx }

func (r *histRepo) AppendHistory(ctx context.Context, entry history.Entry) (int64, error) {
	changesJSON, _ := json.Marshal(entry.Changes())

	result, err := r.tx.ExecContext(ctx,
		`INSERT INTO history (ticket_id, revision, author, timestamp, event_type, changes) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.TicketID().String(), entry.Revision(), entry.Author().String(),
		entry.Timestamp().Format(time.RFC3339Nano), entry.EventType().String(), string(changesJSON))
	if err != nil {
		return 0, &domain.DatabaseError{Op: "append history", Err: err}
	}
	return result.LastInsertId()
}

func (r *histRepo) ListHistory(ctx context.Context, ticketID ticket.ID, filter port.HistoryFilter, page port.PageRequest) ([]history.Entry, port.PageResult, error) {
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

	var total int
	r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM history `+where, args...).Scan(&total)

	query := `SELECT entry_id, ticket_id, revision, author, timestamp, event_type, changes FROM history ` + where + ` ORDER BY revision LIMIT ?`
	args = append(args, page.PageSize)

	rows, err := r.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, port.PageResult{}, &domain.DatabaseError{Op: "list history", Err: err}
	}
	defer rows.Close()

	var entries []history.Entry
	for rows.Next() {
		var entryID int64
		var tidStr, authorStr, tsStr, eventStr, changesStr string
		var revision int
		if err := rows.Scan(&entryID, &tidStr, &revision, &authorStr, &tsStr, &eventStr, &changesStr); err != nil {
			return nil, port.PageResult{}, &domain.DatabaseError{Op: "scan history", Err: err}
		}
		tid, _ := ticket.ParseID(tidStr)
		author, _ := identity.NewAuthor(authorStr)
		ts, _ := time.Parse(time.RFC3339Nano, tsStr)
		eventType, _ := history.ParseEventType(eventStr)

		var changes []history.FieldChange
		_ = json.Unmarshal([]byte(changesStr), &changes)

		entries = append(entries, history.NewEntry(history.NewEntryParams{
			ID: entryID, TicketID: tid, Revision: revision, Author: author,
			Timestamp: ts, EventType: eventType, Changes: changes,
		}))
	}

	return entries, port.PageResult{TotalCount: total}, nil
}

func (r *histRepo) CountHistory(ctx context.Context, ticketID ticket.ID) (int, error) {
	var count int
	err := r.tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM history WHERE ticket_id = ?`, ticketID.String()).Scan(&count)
	if err != nil {
		return 0, &domain.DatabaseError{Op: "count history", Err: err}
	}
	return count, nil
}

func (r *histRepo) GetLatestHistory(ctx context.Context, ticketID ticket.ID) (history.Entry, error) {
	var entryID int64
	var tidStr, authorStr, tsStr, eventStr, changesStr string
	var revision int

	err := r.tx.QueryRowContext(ctx,
		`SELECT entry_id, ticket_id, revision, author, timestamp, event_type, changes FROM history WHERE ticket_id = ? ORDER BY revision DESC LIMIT 1`,
		ticketID.String()).Scan(&entryID, &tidStr, &revision, &authorStr, &tsStr, &eventStr, &changesStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return history.Entry{}, domain.ErrNotFound
		}
		return history.Entry{}, &domain.DatabaseError{Op: "get latest history", Err: err}
	}

	tid, _ := ticket.ParseID(tidStr)
	author, _ := identity.NewAuthor(authorStr)
	ts, _ := time.Parse(time.RFC3339Nano, tsStr)
	eventType, _ := history.ParseEventType(eventStr)

	var changes []history.FieldChange
	_ = json.Unmarshal([]byte(changesStr), &changes)

	return history.NewEntry(history.NewEntryParams{
		ID: entryID, TicketID: tid, Revision: revision, Author: author,
		Timestamp: ts, EventType: eventType, Changes: changes,
	}), nil
}

// --- Helper functions ---

func scanTicket(row *sql.Row) (ticket.Ticket, error) {
	var idStr, roleStr, title, desc, ac, priorityStr, stateStr, createdAtStr string
	var parentID sql.NullString
	var idemKey sql.NullString
	var deleted int

	err := row.Scan(&idStr, &roleStr, &title, &desc, &ac, &priorityStr, &stateStr, &parentID, &createdAtStr, &idemKey, &deleted)
	if err != nil {
		return ticket.Ticket{}, err
	}

	id, _ := ticket.ParseID(idStr)
	priority, _ := ticket.ParsePriority(priorityStr)
	state, _ := ticket.ParseState(stateStr)
	createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)

	var pid ticket.ID
	if parentID.Valid && parentID.String != "" {
		pid, _ = ticket.ParseID(parentID.String)
	}

	role, _ := ticket.ParseRole(roleStr)

	var t ticket.Ticket
	switch role {
	case ticket.RoleTask:
		t, _ = ticket.NewTask(ticket.NewTaskParams{
			ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
			Priority: priority, ParentID: pid, CreatedAt: createdAt,
			IdempotencyKey: idemKey.String,
		})
	case ticket.RoleEpic:
		t, _ = ticket.NewEpic(ticket.NewEpicParams{
			ID: id, Title: title, Description: desc, AcceptanceCriteria: ac,
			Priority: priority, ParentID: pid, CreatedAt: createdAt,
			IdempotencyKey: idemKey.String,
		})
	}

	t = t.WithState(state)
	if deleted != 0 {
		t = t.WithDeleted()
	}

	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
