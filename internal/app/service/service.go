package service

import (
	"context"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// Service defines the driving port — the use-case boundary that CLI and
// other adapters invoke. Each method corresponds to a command from §8 of
// the specification.
type Service interface {
	// --- Global Operations ---

	// Init creates a new database with the given prefix.
	Init(ctx context.Context, prefix string) error

	// AgentName generates a random agent name.
	AgentName(ctx context.Context) (string, error)

	// AgentInstructions returns Markdown instructions for agents.
	AgentInstructions(ctx context.Context) (string, error)

	// GetPrefix returns the database's configured ticket ID prefix.
	GetPrefix(ctx context.Context) (string, error)

	// --- Ticket Operations ---

	// CreateTicket creates a new ticket.
	CreateTicket(ctx context.Context, input CreateTicketInput) (CreateTicketOutput, error)

	// ClaimByID claims a specific ticket.
	ClaimByID(ctx context.Context, input ClaimInput) (ClaimOutput, error)

	// ClaimNextReady claims the highest-priority ready ticket.
	ClaimNextReady(ctx context.Context, input ClaimNextReadyInput) (ClaimOutput, error)

	// OneShotUpdate performs an atomic claim→update→release.
	OneShotUpdate(ctx context.Context, input OneShotUpdateInput) error

	// UpdateTicket updates a claimed ticket's fields.
	UpdateTicket(ctx context.Context, input UpdateTicketInput) error

	// ExtendStaleThreshold extends the stale threshold on an active claim.
	ExtendStaleThreshold(ctx context.Context, ticketID ticket.ID, claimID string, threshold time.Duration) error

	// TransitionState changes the state of a claimed ticket.
	TransitionState(ctx context.Context, input TransitionInput) error

	// DeleteTicket soft-deletes a claimed ticket.
	DeleteTicket(ctx context.Context, input DeleteInput) error

	// ShowTicket returns the full detail view of a ticket.
	ShowTicket(ctx context.Context, id ticket.ID) (ShowTicketOutput, error)

	// ListTickets returns a filtered, ordered, paginated list of tickets.
	ListTickets(ctx context.Context, input ListTicketsInput) (ListTicketsOutput, error)

	// SearchTickets performs full-text search on tickets.
	SearchTickets(ctx context.Context, input SearchTicketsInput) (ListTicketsOutput, error)

	// --- Relationship Operations ---

	// AddRelationship adds a relationship between two tickets.
	AddRelationship(ctx context.Context, sourceID ticket.ID, rel RelationshipInput, author identity.Author) error

	// RemoveRelationship removes a relationship between two tickets.
	RemoveRelationship(ctx context.Context, sourceID ticket.ID, rel RelationshipInput, author identity.Author) error

	// --- Note Operations ---

	// AddNote adds a note to a ticket.
	AddNote(ctx context.Context, input AddNoteInput) (AddNoteOutput, error)

	// ShowNote retrieves a single note by ID.
	ShowNote(ctx context.Context, noteID int64) (note.Note, error)

	// ListNotes lists notes for a ticket.
	ListNotes(ctx context.Context, input ListNotesInput) (ListNotesOutput, error)

	// SearchNotes searches notes by text.
	SearchNotes(ctx context.Context, input SearchNotesInput) (ListNotesOutput, error)

	// --- History Operations ---

	// ShowHistory lists history entries for a ticket.
	ShowHistory(ctx context.Context, input ListHistoryInput) (ListHistoryOutput, error)

	// --- Graph ---

	// GetGraphData returns all non-deleted tickets and their relationships
	// in a single read-only transaction, for rendering as a graph.
	GetGraphData(ctx context.Context) (GraphDataOutput, error)

	// --- Diagnostics ---

	// Doctor runs diagnostics and returns findings.
	Doctor(ctx context.Context) (DoctorOutput, error)

	// GC performs garbage collection.
	GC(ctx context.Context, input GCInput) (GCOutput, error)
}
