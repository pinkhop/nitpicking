package history

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// EventType identifies the kind of mutation recorded by a history entry.
type EventType int

const (
	// EventCreated records issue creation.
	EventCreated EventType = iota + 1

	// EventClaimed records a claim being taken on an issue.
	EventClaimed

	// EventReleased records a claim being released (issue returned to
	// its default unclaimed state).
	EventReleased

	// EventUpdated records one or more field changes to an issue.
	EventUpdated

	// EventStateChanged records a state transition (close, defer, wait).
	EventStateChanged

	// EventDeleted records soft deletion of an issue.
	EventDeleted

	// EventRelationshipAdded records a new relationship from this issue.
	EventRelationshipAdded

	// EventRelationshipRemoved records removal of a relationship from this issue.
	EventRelationshipRemoved
)

// eventTypeStrings maps each EventType to its canonical string.
var eventTypeStrings = map[EventType]string{
	EventCreated:             "created",
	EventClaimed:             "claimed",
	EventReleased:            "released",
	EventUpdated:             "updated",
	EventStateChanged:        "state_changed",
	EventDeleted:             "deleted",
	EventRelationshipAdded:   "relationship_added",
	EventRelationshipRemoved: "relationship_removed",
}

// String returns the canonical string representation.
func (e EventType) String() string {
	if s, ok := eventTypeStrings[e]; ok {
		return s
	}
	return fmt.Sprintf("EventType(%d)", int(e))
}

// ParseEventType parses an event type string into an EventType.
func ParseEventType(s string) (EventType, error) {
	for et, str := range eventTypeStrings {
		if s == str {
			return et, nil
		}
	}
	return 0, fmt.Errorf("invalid event type %q", s)
}

// FieldChange records the before and after values for a single field mutation.
type FieldChange struct {
	Field  string
	Before string
	After  string
}

// Entry represents an immutable record of a single mutation transaction on
// an issue. Every mutation produces exactly one entry. The revision is the
// zero-based index within the issue's history (0 = creation).
type Entry struct {
	id        int64
	issueID   issue.ID
	revision  int
	author    identity.Author
	timestamp time.Time
	eventType EventType
	changes   []FieldChange
}

// NewEntryParams holds the parameters for creating a history entry.
type NewEntryParams struct {
	ID        int64
	IssueID   issue.ID
	Revision  int
	Author    identity.Author
	Timestamp time.Time
	EventType EventType
	Changes   []FieldChange
}

// NewEntry creates an immutable history entry.
func NewEntry(p NewEntryParams) Entry {
	ts := p.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// Defensive copy of changes slice.
	changes := make([]FieldChange, len(p.Changes))
	copy(changes, p.Changes)

	return Entry{
		id:        p.ID,
		issueID:   p.IssueID,
		revision:  p.Revision,
		author:    p.Author,
		timestamp: ts,
		eventType: p.EventType,
		changes:   changes,
	}
}

// ID returns the entry's unique identifier.
func (e Entry) ID() int64 { return e.id }

// IssueID returns the ID of the issue this entry belongs to.
func (e Entry) IssueID() issue.ID { return e.issueID }

// Revision returns the zero-based revision index within the issue's history.
func (e Entry) Revision() int { return e.revision }

// Author returns the author of the mutation.
func (e Entry) Author() identity.Author { return e.author }

// Timestamp returns when the mutation occurred.
func (e Entry) Timestamp() time.Time { return e.timestamp }

// EventType returns the kind of mutation.
func (e Entry) EventType() EventType { return e.eventType }

// Changes returns a copy of the field changes recorded by this entry.
func (e Entry) Changes() []FieldChange {
	out := make([]FieldChange, len(e.changes))
	copy(out, e.changes)
	return out
}
