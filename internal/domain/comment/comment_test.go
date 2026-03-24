package comment_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func mustIssueID(t *testing.T) issue.ID {
	t.Helper()
	id, err := issue.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate issue ID: %v", err)
	}
	return id
}

func TestNewComment_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustIssueID(t)
	author := mustAuthor(t, "alice")
	now := time.Now()

	// When
	c, err := comment.NewComment(comment.NewCommentParams{
		ID:        42,
		IssueID:   tid,
		Author:    author,
		CreatedAt: now,
		Body:      "This is a comment.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID() != 42 {
		t.Errorf("expected ID 42, got %d", c.ID())
	}
	if c.DisplayID() != "comment-42" {
		t.Errorf("expected comment-42, got %s", c.DisplayID())
	}
	if c.IssueID() != tid {
		t.Errorf("expected issue ID %s, got %s", tid, c.IssueID())
	}
	if !c.Author().Equal(author) {
		t.Errorf("expected author alice, got %s", c.Author())
	}
	if c.Body() != "This is a comment." {
		t.Errorf("expected body, got %q", c.Body())
	}
}

func TestNewComment_EmptyBody_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := comment.NewComment(comment.NewCommentParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Body:    "",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !errors.Is(err, &domain.ValidationError{}) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestNewComment_ZeroIssueID_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := comment.NewComment(comment.NewCommentParams{
		Author: mustAuthor(t, "alice"),
		Body:   "content",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero issue ID")
	}
}

func TestNewComment_ZeroAuthor_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := comment.NewComment(comment.NewCommentParams{
		IssueID: mustIssueID(t),
		Body:    "content",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero author")
	}
}
