package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func mustCommentAuthor(t *testing.T, name string) domain.Author {
	t.Helper()
	a, err := domain.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func TestNewComment_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustID(t)
	author := mustCommentAuthor(t, "alice")
	now := time.Now()

	// When
	c, err := domain.NewComment(domain.NewCommentParams{
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
	_, err := domain.NewComment(domain.NewCommentParams{
		IssueID: mustID(t),
		Author:  mustCommentAuthor(t, "alice"),
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
	_, err := domain.NewComment(domain.NewCommentParams{
		Author: mustCommentAuthor(t, "alice"),
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
	_, err := domain.NewComment(domain.NewCommentParams{
		IssueID: mustID(t),
		Body:    "content",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero author")
	}
}
