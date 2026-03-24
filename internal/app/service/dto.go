package service

import (
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// --- Ticket DTOs ---

// CreateTicketInput holds the parameters for creating a ticket.
type CreateTicketInput struct {
	Role               ticket.Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           ticket.Priority
	ParentID           ticket.ID
	Facets             []ticket.Facet
	Relationships      []RelationshipInput
	Author             identity.Author
	Claim              bool
	IdempotencyKey     string
}

// RelationshipInput describes a relationship to add during ticket creation
// or update.
type RelationshipInput struct {
	Type     ticket.RelationType
	TargetID ticket.ID
}

// CreateTicketOutput holds the result of creating a ticket.
type CreateTicketOutput struct {
	Ticket  ticket.Ticket
	ClaimID string // Non-empty if the ticket was created as claimed.
}

// ClaimInput holds the parameters for claiming a ticket.
type ClaimInput struct {
	TicketID       ticket.ID
	Author         identity.Author
	AllowSteal     bool
	StaleThreshold time.Duration
}

// ClaimOutput holds the result of claiming a ticket.
type ClaimOutput struct {
	ClaimID  string
	TicketID ticket.ID
	Stolen   bool
}

// ClaimNextReadyInput holds the parameters for claiming the next ready ticket.
type ClaimNextReadyInput struct {
	Author         identity.Author
	Role           ticket.Role
	FacetFilters   []port.FacetFilter
	StealIfNeeded  bool
	StaleThreshold time.Duration
}

// UpdateTicketInput holds the parameters for updating a claimed ticket.
type UpdateTicketInput struct {
	TicketID           ticket.ID
	ClaimID            string
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *ticket.Priority
	ParentID           *ticket.ID
	FacetSet           []ticket.Facet
	FacetRemove        []string
	RelationshipAdd    []RelationshipInput
	RelationshipRemove []RelationshipInput
	NoteBody           string
}

// OneShotUpdateInput holds the parameters for an atomic claim→update→release.
type OneShotUpdateInput struct {
	TicketID           ticket.ID
	Author             identity.Author
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *ticket.Priority
	ParentID           *ticket.ID
	FacetSet           []ticket.Facet
	FacetRemove        []string
}

// TransitionInput holds the parameters for a state transition.
type TransitionInput struct {
	TicketID ticket.ID
	ClaimID  string
	Action   TransitionAction
}

// TransitionAction identifies the kind of state transition.
type TransitionAction int

const (
	// ActionRelease returns the ticket to its default unclaimed state.
	ActionRelease TransitionAction = iota + 1

	// ActionClose marks a task as complete. Terminal.
	ActionClose

	// ActionDefer shelves the ticket.
	ActionDefer

	// ActionWait marks the ticket as externally blocked.
	ActionWait
)

// DeleteInput holds the parameters for soft-deleting a ticket.
type DeleteInput struct {
	TicketID ticket.ID
	ClaimID  string
}

// ShowTicketOutput holds the full detail view of a ticket.
type ShowTicketOutput struct {
	Ticket        ticket.Ticket
	Revision      int
	Author        identity.Author
	Relationships []ticket.Relationship
	IsReady       bool
	IsComplete    bool // Only meaningful for epics.
	NoteCount     int
	ClaimID       string
	ClaimAuthor   string
	ClaimStaleAt  time.Time
}

// ListTicketsInput holds the parameters for listing tickets.
type ListTicketsInput struct {
	Filter  port.TicketFilter
	OrderBy port.TicketOrderBy
	Page    port.PageRequest
}

// ListTicketsOutput holds the result of listing tickets.
type ListTicketsOutput struct {
	Items      []port.TicketListItem
	TotalCount int
}

// SearchTicketsInput holds the parameters for searching tickets.
type SearchTicketsInput struct {
	Query        string
	Filter       port.TicketFilter
	OrderBy      port.TicketOrderBy
	Page         port.PageRequest
	IncludeNotes bool
}

// --- Note DTOs ---

// AddNoteInput holds the parameters for adding a note.
type AddNoteInput struct {
	TicketID ticket.ID
	Author   identity.Author
	Body     string
}

// AddNoteOutput holds the result of adding a note.
type AddNoteOutput struct {
	Note note.Note
}

// ListNotesInput holds the parameters for listing notes.
type ListNotesInput struct {
	TicketID ticket.ID
	Filter   port.NoteFilter
	Page     port.PageRequest
}

// ListNotesOutput holds the result of listing notes.
type ListNotesOutput struct {
	Notes      []note.Note
	TotalCount int
}

// SearchNotesInput holds the parameters for searching notes.
type SearchNotesInput struct {
	Query    string
	TicketID ticket.ID // Zero for global search.
	Filter   port.NoteFilter
	Page     port.PageRequest
}

// --- History DTOs ---

// ListHistoryInput holds the parameters for listing history.
type ListHistoryInput struct {
	TicketID ticket.ID
	Filter   port.HistoryFilter
	Page     port.PageRequest
}

// ListHistoryOutput holds the result of listing history.
type ListHistoryOutput struct {
	Entries    []history.Entry
	TotalCount int
}

// --- Diagnostics DTOs ---

// DoctorFinding represents a single diagnostic finding.
type DoctorFinding struct {
	// Category identifies the kind of finding.
	Category string
	// Severity is "warning" or "error".
	Severity string
	// Message describes the finding.
	Message string
	// TicketIDs lists affected tickets.
	TicketIDs []string
}

// DoctorOutput holds the results of the doctor diagnostic.
type DoctorOutput struct {
	Findings []DoctorFinding
}

// GraphDataOutput holds the data needed to render a ticket graph.
type GraphDataOutput struct {
	// Nodes contains all non-deleted tickets as lightweight projections.
	Nodes []port.TicketListItem
	// Relationships contains all relationships for the included tickets.
	Relationships []ticket.Relationship
}

// GCInput holds the parameters for garbage collection.
type GCInput struct {
	IncludeClosed bool
}

// GCOutput holds the result of garbage collection.
type GCOutput struct {
	DeletedTicketsRemoved int
	ClosedTicketsRemoved  int
}
