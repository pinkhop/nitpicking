package relcmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Tests ---

func TestFilterRelationships_BlockedBy_ReturnsOnlyBlockingRels(t *testing.T) {
	t.Parallel()

	// Given: a full set of relationship DTOs.
	rels := []driving.RelationshipDTO{
		{SourceID: "NP-aaaaa", TargetID: "NP-bbbbb", Type: "blocked_by"},
		{SourceID: "NP-bbbbb", TargetID: "NP-aaaaa", Type: "blocks"},
		{SourceID: "NP-aaaaa", TargetID: "NP-ccccc", Type: "cites"},
		{SourceID: "NP-ccccc", TargetID: "NP-aaaaa", Type: "cited_by"},
	}

	// When: filtering for blocked_by and blocks.
	filtered := relcmd.FilterRelationships(rels, "blocked_by", "blocks")

	// Then: only blocking relationships are returned.
	if len(filtered) != 2 {
		t.Fatalf("count: got %d, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.Type != "blocked_by" && r.Type != "blocks" {
			t.Errorf("unexpected relationship type: %s", r.Type)
		}
	}
}

func TestFilterRelationships_Cites_ReturnsOnlyCitationRels(t *testing.T) {
	t.Parallel()

	// Given
	rels := []driving.RelationshipDTO{
		{SourceID: "NP-aaaaa", TargetID: "NP-bbbbb", Type: "blocked_by"},
		{SourceID: "NP-aaaaa", TargetID: "NP-ccccc", Type: "cites"},
		{SourceID: "NP-ccccc", TargetID: "NP-aaaaa", Type: "cited_by"},
	}

	// When
	filtered := relcmd.FilterRelationships(rels, "cites", "cited_by")

	// Then
	if len(filtered) != 2 {
		t.Fatalf("count: got %d, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.Type != "cites" && r.Type != "cited_by" {
			t.Errorf("unexpected relationship type: %s", r.Type)
		}
	}
}
