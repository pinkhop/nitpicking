package note

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// Note represents a comment attached to a ticket. Notes are immutable after
// creation. IDs are auto-assigned sequential integers, displayed as
// "note-<integer>" (e.g., "note-368"). IDs are global across the database,
// not scoped per ticket.
type Note struct {
	id        int64
	ticketID  ticket.ID
	author    identity.Author
	createdAt time.Time
	body      string
}

// NewNoteParams holds the parameters for creating a new note.
type NewNoteParams struct {
	ID        int64
	TicketID  ticket.ID
	Author    identity.Author
	CreatedAt time.Time
	Body      string
}

// NewNote creates a validated Note. The body must be non-empty.
func NewNote(p NewNoteParams) (Note, error) {
	if p.TicketID.IsZero() {
		return Note{}, domain.NewValidationError("ticket_id", "must not be empty")
	}
	if p.Author.IsZero() {
		return Note{}, domain.NewValidationError("author", "must not be empty")
	}
	if p.Body == "" {
		return Note{}, domain.NewValidationError("body", "must not be empty")
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Note{
		id:        p.ID,
		ticketID:  p.TicketID,
		author:    p.Author,
		createdAt: createdAt,
		body:      p.Body,
	}, nil
}

// ID returns the note's sequential integer ID.
func (n Note) ID() int64 { return n.id }

// DisplayID returns the human-readable note ID (e.g., "note-368").
func (n Note) DisplayID() string { return fmt.Sprintf("note-%d", n.id) }

// TicketID returns the ID of the ticket this note belongs to.
func (n Note) TicketID() ticket.ID { return n.ticketID }

// Author returns the note's author.
func (n Note) Author() identity.Author { return n.author }

// CreatedAt returns the note's creation timestamp.
func (n Note) CreatedAt() time.Time { return n.createdAt }

// Body returns the note's text content.
func (n Note) Body() string { return n.body }
