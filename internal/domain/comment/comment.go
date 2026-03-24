package comment

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// Comment represents a comment attached to an issue. Comments are immutable
// after creation. IDs are auto-assigned sequential integers, displayed as
// "comment-<integer>" (e.g., "comment-368"). IDs are global across the
// database, not scoped per issue.
type Comment struct {
	id        int64
	issueID   issue.ID
	author    identity.Author
	createdAt time.Time
	body      string
}

// NewCommentParams holds the parameters for creating a new comment.
type NewCommentParams struct {
	ID        int64
	IssueID   issue.ID
	Author    identity.Author
	CreatedAt time.Time
	Body      string
}

// NewComment creates a validated Comment. The body must be non-empty.
func NewComment(p NewCommentParams) (Comment, error) {
	if p.IssueID.IsZero() {
		return Comment{}, domain.NewValidationError("issue_id", "must not be empty")
	}
	if p.Author.IsZero() {
		return Comment{}, domain.NewValidationError("author", "must not be empty")
	}
	if p.Body == "" {
		return Comment{}, domain.NewValidationError("body", "must not be empty")
	}

	createdAt := p.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	return Comment{
		id:        p.ID,
		issueID:   p.IssueID,
		author:    p.Author,
		createdAt: createdAt,
		body:      p.Body,
	}, nil
}

// ID returns the comment's sequential integer ID.
func (c Comment) ID() int64 { return c.id }

// DisplayID returns the human-readable comment ID (e.g., "comment-368").
func (c Comment) DisplayID() string { return fmt.Sprintf("comment-%d", c.id) }

// IssueID returns the ID of the issue this comment belongs to.
func (c Comment) IssueID() issue.ID { return c.issueID }

// Author returns the comment's author.
func (c Comment) Author() identity.Author { return c.author }

// CreatedAt returns the comment's creation timestamp.
func (c Comment) CreatedAt() time.Time { return c.createdAt }

// Body returns the comment's text content.
func (c Comment) Body() string { return c.body }
