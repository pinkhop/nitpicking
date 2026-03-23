package history

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// EventType identifies the kind of mutation recorded by a history entry.
type EventType int

const (
	// EventCreated records ticket creation.
	EventCreated EventType = iota + 1

	// EventClaimed records a claim being taken on a ticket.
	EventClaimed

	// EventReleased records a claim being released (ticket returned to
	// its default unclaimed state).
	EventReleased

	// EventUpdated records one or more field changes to a ticket.
	EventUpdated

	// EventStateChanged records a state transition (close, defer, wait).
	EventStateChanged

	// EventDeleted records soft deletion of a ticket.
	EventDeleted

	// EventRelationshipAdded records a new relationship from this ticket.
	EventRelationshipAdded

	// EventRelationshipRemoved records removal of a relationship from this ticket.
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
// a ticket. Every mutation produces exactly one entry. The revision is the
// zero-based index within the ticket's history (0 = creation).
type Entry struct {
	id        int64
	ticketID  ticket.ID
	revision  int
	author    identity.Author
	timestamp time.Time
	eventType EventType
	changes   []FieldChange
}

// NewEntryParams holds the parameters for creating a history entry.
type NewEntryParams struct {
	ID        int64
	TicketID  ticket.ID
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
		ticketID:  p.TicketID,
		revision:  p.Revision,
		author:    p.Author,
		timestamp: ts,
		eventType: p.EventType,
		changes:   changes,
	}
}

// ID returns the entry's unique identifier.
func (e Entry) ID() int64 { return e.id }

// TicketID returns the ID of the ticket this entry belongs to.
func (e Entry) TicketID() ticket.ID { return e.ticketID }

// Revision returns the zero-based revision index within the ticket's history.
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
