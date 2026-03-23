package ticket

import "fmt"

// RelationType represents the kind of relationship between two tickets.
type RelationType int

const (
	// RelBlockedBy indicates the source ticket cannot make progress until
	// the target ticket is closed.
	RelBlockedBy RelationType = iota + 1

	// RelBlocks is the inverse of RelBlockedBy — the source blocks the target.
	RelBlocks

	// RelCites indicates the source ticket references the target as relevant
	// context.
	RelCites

	// RelCitedBy is the inverse of RelCites — the source is cited by the target.
	RelCitedBy
)

// relationTypeStrings maps each RelationType to its canonical string.
var relationTypeStrings = map[RelationType]string{
	RelBlockedBy: "blocked_by",
	RelBlocks:    "blocks",
	RelCites:     "cites",
	RelCitedBy:   "cited_by",
}

// String returns the canonical string representation.
func (rt RelationType) String() string {
	if s, ok := relationTypeStrings[rt]; ok {
		return s
	}
	return fmt.Sprintf("RelationType(%d)", int(rt))
}

// ParseRelationType parses a relationship type string into a RelationType.
func ParseRelationType(s string) (RelationType, error) {
	for rt, str := range relationTypeStrings {
		if s == str {
			return rt, nil
		}
	}
	return 0, fmt.Errorf("invalid relationship type %q: must be blocked_by, blocks, cites, or cited_by", s)
}

// Inverse returns the inverse relationship type.
func (rt RelationType) Inverse() RelationType {
	switch rt {
	case RelBlockedBy:
		return RelBlocks
	case RelBlocks:
		return RelBlockedBy
	case RelCites:
		return RelCitedBy
	case RelCitedBy:
		return RelCites
	default:
		return 0
	}
}

// Relationship represents a directional link between two tickets.
type Relationship struct {
	sourceID ID
	targetID ID
	relType  RelationType
}

// NewRelationship creates a validated relationship. It rejects self-relationships.
func NewRelationship(sourceID, targetID ID, relType RelationType) (Relationship, error) {
	if sourceID.IsZero() || targetID.IsZero() {
		return Relationship{}, fmt.Errorf("relationship requires non-zero source and target IDs")
	}
	if sourceID == targetID {
		return Relationship{}, fmt.Errorf("a ticket cannot have a relationship with itself")
	}
	if relType < RelBlockedBy || relType > RelCitedBy {
		return Relationship{}, fmt.Errorf("invalid relationship type")
	}
	return Relationship{sourceID: sourceID, targetID: targetID, relType: relType}, nil
}

// SourceID returns the ID of the ticket initiating the relationship.
func (r Relationship) SourceID() ID { return r.sourceID }

// TargetID returns the ID of the ticket referenced by the relationship.
func (r Relationship) TargetID() ID { return r.targetID }

// Type returns the relationship type.
func (r Relationship) Type() RelationType { return r.relType }
