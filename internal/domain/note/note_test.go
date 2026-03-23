package note_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func mustTicketID(t *testing.T) ticket.ID {
	t.Helper()
	id, err := ticket.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate ticket ID: %v", err)
	}
	return id
}

func TestNewNote_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustTicketID(t)
	author := mustAuthor(t, "alice")
	now := time.Now()

	// When
	n, err := note.NewNote(note.NewNoteParams{
		ID:        42,
		TicketID:  tid,
		Author:    author,
		CreatedAt: now,
		Body:      "This is a note.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.ID() != 42 {
		t.Errorf("expected ID 42, got %d", n.ID())
	}
	if n.DisplayID() != "note-42" {
		t.Errorf("expected note-42, got %s", n.DisplayID())
	}
	if n.TicketID() != tid {
		t.Errorf("expected ticket ID %s, got %s", tid, n.TicketID())
	}
	if !n.Author().Equal(author) {
		t.Errorf("expected author alice, got %s", n.Author())
	}
	if n.Body() != "This is a note." {
		t.Errorf("expected body, got %q", n.Body())
	}
}

func TestNewNote_EmptyBody_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := note.NewNote(note.NewNoteParams{
		TicketID: mustTicketID(t),
		Author:   mustAuthor(t, "alice"),
		Body:     "",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for empty body")
	}
	if !errors.Is(err, &domain.ValidationError{}) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestNewNote_ZeroTicketID_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := note.NewNote(note.NewNoteParams{
		Author: mustAuthor(t, "alice"),
		Body:   "content",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero ticket ID")
	}
}

func TestNewNote_ZeroAuthor_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := note.NewNote(note.NewNoteParams{
		TicketID: mustTicketID(t),
		Body:     "content",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero author")
	}
}
