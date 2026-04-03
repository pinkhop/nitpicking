package history_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
)

func mustAuthor(t *testing.T, name string) domain.Author {
	t.Helper()
	a, err := domain.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func mustIssueID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate issue ID: %v", err)
	}
	return id
}

func TestNewEntry_CreatesImmutableEntry(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustIssueID(t)
	author := mustAuthor(t, "alice")
	now := time.Now()
	changes := []history.FieldChange{
		{Field: "title", Before: "", After: "Fix bug"},
	}

	// When
	entry := history.NewEntry(history.NewEntryParams{
		ID:        1,
		IssueID:   tid,
		Revision:  0,
		Author:    author,
		Timestamp: now,
		EventType: history.EventCreated,
		Changes:   changes,
	})

	// Then
	if entry.ID() != 1 {
		t.Errorf("expected ID 1, got %d", entry.ID())
	}
	if entry.IssueID() != tid {
		t.Errorf("expected issue ID %s, got %s", tid, entry.IssueID())
	}
	if entry.Revision() != 0 {
		t.Errorf("expected revision 0, got %d", entry.Revision())
	}
	if !entry.Author().Equal(author) {
		t.Errorf("expected author alice, got %s", entry.Author())
	}
	if entry.EventType() != history.EventCreated {
		t.Errorf("expected created event, got %s", entry.EventType())
	}
	if len(entry.Changes()) != 1 {
		t.Fatalf("expected 1 change, got %d", len(entry.Changes()))
	}
	if entry.Changes()[0].Field != "title" {
		t.Errorf("expected title field, got %s", entry.Changes()[0].Field)
	}
}

func TestNewEntry_DefensiveCopyOfChanges(t *testing.T) {
	t.Parallel()

	// Given
	changes := []history.FieldChange{
		{Field: "title", Before: "", After: "Fix bug"},
	}
	entry := history.NewEntry(history.NewEntryParams{
		IssueID:   mustIssueID(t),
		Author:    mustAuthor(t, "bob"),
		EventType: history.EventCreated,
		Changes:   changes,
	})

	// When — mutate original slice
	changes[0].Field = "mutated"

	// Then — entry is unaffected
	if entry.Changes()[0].Field != "title" {
		t.Error("expected entry changes to be independent of input slice")
	}
}

func TestNewEntry_ChangesReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	// Given
	entry := history.NewEntry(history.NewEntryParams{
		IssueID:   mustIssueID(t),
		Author:    mustAuthor(t, "bob"),
		EventType: history.EventUpdated,
		Changes: []history.FieldChange{
			{Field: "priority", Before: "P2", After: "P0"},
		},
	})

	// When — mutate returned slice
	returned := entry.Changes()
	returned[0].Field = "mutated"

	// Then — subsequent call is unaffected
	if entry.Changes()[0].Field != "priority" {
		t.Error("expected Changes() to return defensive copy")
	}
}

func TestParseEventType_InvalidInput_ReturnsError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"unknown event name", "exploded"},
		{"numeric string", "42"},
		{"partial match", "create"},
		{"uppercase variant", "CREATED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := history.ParseEventType(tc.input)

			// Then
			if err == nil {
				t.Errorf("expected error for input %q, got nil", tc.input)
			}
		})
	}
}

func TestEventType_StringRoundTrips(t *testing.T) {
	t.Parallel()

	cases := []history.EventType{
		history.EventCreated,
		history.EventClaimed,
		history.EventReleased,
		history.EventUpdated,
		history.EventStateChanged,
		history.EventDeleted,
		history.EventRelationshipAdded,
		history.EventRelationshipRemoved,
		history.EventCommentAdded,
		history.EventLabelAdded,
		history.EventLabelRemoved,
		history.EventRestored,
		history.EventReopened,
		history.EventUndeferred,
	}

	for _, et := range cases {
		t.Run(et.String(), func(t *testing.T) {
			t.Parallel()

			// When
			parsed, err := history.ParseEventType(et.String())
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed != et {
				t.Errorf("round-trip failed: %v != %v", parsed, et)
			}
		})
	}
}
