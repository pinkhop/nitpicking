package note

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// Note represents a comment attached to an issue. Notes are immutable after
// creation. IDs are auto-assigned sequential integers, displayed as
// "note-<integer>" (e.g., "note-368"). IDs are global across the database,
// not scoped per issue.
type Note struct {
	id        int64
	issueID   issue.ID
	author    identity.Author
	createdAt time.Time
	body      string
}

// NewNoteParams holds the parameters for creating a new note.
type NewNoteParams struct {
	ID        int64
	IssueID   issue.ID
	Author    identity.Author
	CreatedAt time.Time
	Body      string
}

// NewNote creates a validated Note. The body must be non-empty.
func NewNote(p NewNoteParams) (Note, error) {
	if p.IssueID.IsZero() {
		return Note{}, domain.NewValidationError("issue_id", "must not be empty")
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
		issueID:   p.IssueID,
		author:    p.Author,
		createdAt: createdAt,
		body:      p.Body,
	}, nil
}

// ID returns the note's sequential integer ID.
func (n Note) ID() int64 { return n.id }

// DisplayID returns the human-readable note ID (e.g., "note-368").
func (n Note) DisplayID() string { return fmt.Sprintf("note-%d", n.id) }

// IssueID returns the ID of the issue this note belongs to.
func (n Note) IssueID() issue.ID { return n.issueID }

// Author returns the note's author.
func (n Note) Author() identity.Author { return n.author }

// CreatedAt returns the note's creation timestamp.
func (n Note) CreatedAt() time.Time { return n.createdAt }

// Body returns the note's text content.
func (n Note) Body() string { return n.body }
