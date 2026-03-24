package port

import (
	"context"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// PageRequest specifies keyset pagination parameters for list operations.
type PageRequest struct {
	// PageSize is the maximum number of items to return. Default: 20.
	PageSize int

	// AfterSortKey is the sort key of the last item from the previous page.
	// Empty for the first page.
	AfterSortKey string

	// AfterID is the ID of the last item from the previous page, used as a
	// tiebreaker. Empty for the first page.
	AfterID string
}

// DefaultPageSize is the default number of items per page.
const DefaultPageSize = 20

// Normalize applies defaults to a PageRequest.
func (p PageRequest) Normalize() PageRequest {
	if p.PageSize <= 0 {
		p.PageSize = DefaultPageSize
	}
	return p
}

// PageResult holds pagination metadata alongside results.
type PageResult struct {
	// TotalCount is the total number of matching items (not just this page).
	TotalCount int
}

// TicketListItem is a lightweight projection of a ticket for list views.
type TicketListItem struct {
	ID        ticket.ID
	Role      ticket.Role
	State     ticket.State
	Priority  ticket.Priority
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
	IsDeleted bool
}

// TicketFilter defines filtering criteria for ticket list and search.
type TicketFilter struct {
	// Role filters by ticket role (zero value means no filter).
	Role ticket.Role
	// States filters by one or more states (empty means no filter).
	States []ticket.State
	// Ready filters to only ready tickets when true.
	Ready bool
	// ParentID filters to children of a specific epic.
	ParentID ticket.ID
	// DescendantsOf recursively filters to all descendants of a ticket.
	DescendantsOf ticket.ID
	// AncestorsOf filters to the parent chain of a ticket (up to the root).
	AncestorsOf ticket.ID
	// FacetFilters specifies facet-based filters.
	FacetFilters []FacetFilter
	// IncludeDeleted includes soft-deleted tickets when true.
	IncludeDeleted bool
}

// FacetFilter specifies a single facet-based filter criterion.
type FacetFilter struct {
	// Key is the facet key to match.
	Key string
	// Value is the facet value to match. Empty for wildcard ("key:*").
	Value string
	// Negate inverts the filter — exclude tickets matching this facet.
	Negate bool
}

// TicketOrderBy specifies the sort order for ticket listings.
type TicketOrderBy int

const (
	// OrderByPriority sorts by priority (highest urgency first), then
	// creation time as tiebreaker.
	OrderByPriority TicketOrderBy = iota

	// OrderByCreatedAt sorts by creation time (oldest first).
	OrderByCreatedAt

	// OrderByUpdatedAt sorts by last modification time (most recent first).
	OrderByUpdatedAt
)

// NoteFilter defines filtering criteria for note listings.
type NoteFilter struct {
	// Author filters notes by author.
	Author identity.Author
	// CreatedAfter filters to notes created after this timestamp.
	CreatedAfter time.Time
	// AfterNoteID filters to notes with ID greater than this.
	AfterNoteID int64
	// TicketID scopes the search to a specific ticket (zero = global).
	TicketID ticket.ID
}

// HistoryFilter defines filtering criteria for history listings.
type HistoryFilter struct {
	// Author filters entries by author.
	Author identity.Author
	// After filters entries created after this timestamp.
	After time.Time
	// Before filters entries created before this timestamp.
	Before time.Time
}

// TicketRepository defines the persistence interface for tickets.
type TicketRepository interface {
	// CreateTicket persists a new ticket. Returns the created ticket.
	CreateTicket(ctx context.Context, t ticket.Ticket) error

	// GetTicket retrieves a ticket by ID. Returns domain.ErrNotFound if
	// not found or if soft-deleted (unless includeDeleted is true).
	GetTicket(ctx context.Context, id ticket.ID, includeDeleted bool) (ticket.Ticket, error)

	// UpdateTicket persists changes to an existing ticket.
	UpdateTicket(ctx context.Context, t ticket.Ticket) error

	// ListTickets returns a filtered, ordered, paginated list of tickets.
	ListTickets(ctx context.Context, filter TicketFilter, orderBy TicketOrderBy, page PageRequest) ([]TicketListItem, PageResult, error)

	// SearchTickets performs full-text search on title, description, and
	// acceptance criteria.
	SearchTickets(ctx context.Context, query string, filter TicketFilter, orderBy TicketOrderBy, page PageRequest) ([]TicketListItem, PageResult, error)

	// GetChildStatuses returns the completion-relevant status of all direct
	// children of an epic, for deriving epic completion.
	GetChildStatuses(ctx context.Context, epicID ticket.ID) ([]ticket.ChildStatus, error)

	// GetDescendants returns all descendants of an epic (recursively),
	// with claim status, for recursive deletion checks.
	GetDescendants(ctx context.Context, epicID ticket.ID) ([]ticket.DescendantInfo, error)

	// HasChildren reports whether an epic has any children.
	HasChildren(ctx context.Context, epicID ticket.ID) (bool, error)

	// GetAncestorStatuses returns the states of all ancestor epics of a
	// ticket, walking up the parent chain, for readiness propagation.
	GetAncestorStatuses(ctx context.Context, id ticket.ID) ([]ticket.AncestorStatus, error)

	// GetParentID returns the parent ID of a ticket (for cycle detection).
	GetParentID(ctx context.Context, id ticket.ID) (ticket.ID, error)

	// TicketIDExists reports whether a ticket ID already exists (for
	// collision detection during ID generation).
	TicketIDExists(ctx context.Context, id ticket.ID) (bool, error)

	// GetTicketByIdempotencyKey retrieves a ticket by its idempotency key.
	// Returns domain.ErrNotFound if no ticket exists with that key.
	GetTicketByIdempotencyKey(ctx context.Context, key string) (ticket.Ticket, error)
}

// NoteRepository defines the persistence interface for notes.
type NoteRepository interface {
	// CreateNote persists a new note and returns the assigned ID.
	CreateNote(ctx context.Context, n note.Note) (int64, error)

	// GetNote retrieves a note by ID. Returns domain.ErrNotFound if not found.
	GetNote(ctx context.Context, id int64) (note.Note, error)

	// ListNotes returns notes for a ticket with optional filters.
	ListNotes(ctx context.Context, ticketID ticket.ID, filter NoteFilter, page PageRequest) ([]note.Note, PageResult, error)

	// SearchNotes performs full-text search on note bodies.
	SearchNotes(ctx context.Context, query string, filter NoteFilter, page PageRequest) ([]note.Note, PageResult, error)
}

// ClaimRepository defines the persistence interface for claims.
type ClaimRepository interface {
	// CreateClaim persists a new claim.
	CreateClaim(ctx context.Context, c claim.Claim) error

	// GetClaimByTicket retrieves the active claim for a ticket.
	// Returns domain.ErrNotFound if no active claim exists.
	GetClaimByTicket(ctx context.Context, ticketID ticket.ID) (claim.Claim, error)

	// GetClaimByID retrieves a claim by its claim ID.
	// Returns domain.ErrNotFound if not found.
	GetClaimByID(ctx context.Context, claimID string) (claim.Claim, error)

	// InvalidateClaim removes the active claim from a ticket.
	InvalidateClaim(ctx context.Context, claimID string) error

	// UpdateClaimLastActivity updates the last activity timestamp on a claim.
	UpdateClaimLastActivity(ctx context.Context, claimID string, lastActivity time.Time) error

	// UpdateClaimThreshold updates the stale threshold on a claim.
	UpdateClaimThreshold(ctx context.Context, claimID string, threshold time.Duration) error

	// ListStaleClaims returns all claims that are stale as of the given time.
	ListStaleClaims(ctx context.Context, now time.Time) ([]claim.Claim, error)
}

// RelationshipRepository defines the persistence interface for relationships.
type RelationshipRepository interface {
	// CreateRelationship creates a relationship if it does not already exist.
	// Returns true if created, false if it already existed (idempotent).
	CreateRelationship(ctx context.Context, rel ticket.Relationship) (bool, error)

	// DeleteRelationship removes a relationship if it exists.
	// Returns true if deleted, false if it did not exist (idempotent).
	DeleteRelationship(ctx context.Context, sourceID, targetID ticket.ID, relType ticket.RelationType) (bool, error)

	// ListRelationships returns all relationships for a ticket (both
	// directions).
	ListRelationships(ctx context.Context, ticketID ticket.ID) ([]ticket.Relationship, error)

	// GetBlockerStatuses returns the blocker statuses for readiness checks.
	GetBlockerStatuses(ctx context.Context, ticketID ticket.ID) ([]ticket.BlockerStatus, error)
}

// HistoryRepository defines the persistence interface for history entries.
type HistoryRepository interface {
	// AppendHistory adds a history entry for a ticket and returns the
	// assigned entry ID.
	AppendHistory(ctx context.Context, entry history.Entry) (int64, error)

	// ListHistory returns history entries for a ticket with optional filters.
	ListHistory(ctx context.Context, ticketID ticket.ID, filter HistoryFilter, page PageRequest) ([]history.Entry, PageResult, error)

	// CountHistory returns the number of history entries for a ticket
	// (used to compute revision).
	CountHistory(ctx context.Context, ticketID ticket.ID) (int, error)

	// GetLatestHistory returns the most recent history entry for a ticket
	// (used to derive the ticket's current author).
	GetLatestHistory(ctx context.Context, ticketID ticket.ID) (history.Entry, error)
}

// DatabaseRepository defines database-level operations.
type DatabaseRepository interface {
	// InitDatabase creates the database schema and stores the prefix.
	InitDatabase(ctx context.Context, prefix string) error

	// GetPrefix retrieves the stored prefix.
	GetPrefix(ctx context.Context) (string, error)

	// GC physically removes deleted (and optionally closed) ticket data.
	GC(ctx context.Context, includeClosedTickets bool) error
}

// UnitOfWork represents a transactional scope. All repository operations
// within a unit of work are atomic — they either all succeed or all fail.
type UnitOfWork interface {
	// Tickets returns the ticket repository within this transaction.
	Tickets() TicketRepository

	// Notes returns the note repository within this transaction.
	Notes() NoteRepository

	// Claims returns the claim repository within this transaction.
	Claims() ClaimRepository

	// Relationships returns the relationship repository within this transaction.
	Relationships() RelationshipRepository

	// History returns the history repository within this transaction.
	History() HistoryRepository

	// Database returns the database-level repository within this transaction.
	Database() DatabaseRepository
}

// UnitOfWorkFactory creates new units of work.
type UnitOfWorkFactory interface {
	// Begin starts a new unit of work (transaction). The caller must call
	// Commit or Rollback on the returned UnitOfWork.
	Begin(ctx context.Context) (UnitOfWork, error)

	// ReadOnly starts a read-only unit of work.
	ReadOnly(ctx context.Context) (UnitOfWork, error)
}

// Transactor provides a higher-level API for executing work within a
// transaction. It handles commit/rollback automatically.
type Transactor interface {
	// WithTransaction executes fn within a transaction. If fn returns nil,
	// the transaction is committed. If fn returns an error, the transaction
	// is rolled back and the error is returned.
	WithTransaction(ctx context.Context, fn func(uow UnitOfWork) error) error

	// WithReadTransaction executes fn within a read-only transaction.
	WithReadTransaction(ctx context.Context, fn func(uow UnitOfWork) error) error
}
