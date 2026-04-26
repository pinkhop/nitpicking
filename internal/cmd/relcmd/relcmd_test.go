package relcmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Tests ---

func TestFilterRelationships_BlockedBy_ReturnsOnlyBlockingRels(t *testing.T) {
	t.Parallel()

	// Given: a mixed set of relationship DTOs.
	rels := []driving.RelationshipDTO{
		{SourceID: "FOO-aaaaa", TargetID: "FOO-bbbbb", Type: "blocked_by"},
		{SourceID: "FOO-bbbbb", TargetID: "FOO-aaaaa", Type: "blocks"},
		{SourceID: "FOO-aaaaa", TargetID: "FOO-ccccc", Type: "refs"},
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

func TestFilterRelationships_Refs_ReturnsOnlyRefsRels(t *testing.T) {
	t.Parallel()

	// Given: a mixed set of relationship DTOs.
	rels := []driving.RelationshipDTO{
		{SourceID: "FOO-aaaaa", TargetID: "FOO-bbbbb", Type: "blocked_by"},
		{SourceID: "FOO-aaaaa", TargetID: "FOO-ccccc", Type: "refs"},
	}

	// When: filtering for refs.
	filtered := relcmd.FilterRelationships(rels, "refs")

	// Then: only the refs relationship is returned.
	if len(filtered) != 1 {
		t.Fatalf("count: got %d, want 1", len(filtered))
	}
	if filtered[0].Type != "refs" {
		t.Errorf("unexpected relationship type: %s", filtered[0].Type)
	}
}
