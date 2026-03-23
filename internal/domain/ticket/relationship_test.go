package ticket_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func TestNewRelationship_ValidRelationship_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	src := mustID(t)
	tgt := mustID(t)

	// When
	rel, err := ticket.NewRelationship(src, tgt, ticket.RelBlockedBy)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.SourceID() != src {
		t.Errorf("expected source %s, got %s", src, rel.SourceID())
	}
	if rel.TargetID() != tgt {
		t.Errorf("expected target %s, got %s", tgt, rel.TargetID())
	}
	if rel.Type() != ticket.RelBlockedBy {
		t.Errorf("expected blocked_by, got %s", rel.Type())
	}
}

func TestNewRelationship_SelfRelationship_Fails(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	_, err := ticket.NewRelationship(id, id, ticket.RelCites)

	// Then
	if err == nil {
		t.Error("expected error for self-relationship")
	}
}

func TestNewRelationship_ZeroIDs_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := ticket.NewRelationship(ticket.ID{}, mustID(t), ticket.RelCites)

	// Then
	if err == nil {
		t.Error("expected error for zero source ID")
	}
}

func TestRelationType_Inverse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    ticket.RelationType
		expected ticket.RelationType
	}{
		{ticket.RelBlockedBy, ticket.RelBlocks},
		{ticket.RelBlocks, ticket.RelBlockedBy},
		{ticket.RelCites, ticket.RelCitedBy},
		{ticket.RelCitedBy, ticket.RelCites},
	}

	for _, tc := range cases {
		t.Run(tc.input.String(), func(t *testing.T) {
			t.Parallel()

			// When
			inv := tc.input.Inverse()

			// Then
			if inv != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, inv)
			}
		})
	}
}

func TestParseRelationType_ValidTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected ticket.RelationType
	}{
		{"blocked_by", ticket.RelBlockedBy},
		{"blocks", ticket.RelBlocks},
		{"cites", ticket.RelCites},
		{"cited_by", ticket.RelCitedBy},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			rt, err := ticket.ParseRelationType(tc.input)
			// Then
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rt != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, rt)
			}
		})
	}
}

func TestParseRelationType_Invalid_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := ticket.ParseRelationType("depends_on")

	// Then
	if err == nil {
		t.Error("expected error for invalid relationship type")
	}
}
