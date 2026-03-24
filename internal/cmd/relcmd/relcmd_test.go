package relcmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

// --- Tests ---

func TestFilterRelationships_BlockedBy_ReturnsOnlyBlockingRels(t *testing.T) {
	t.Parallel()

	// Given: a full set of relationships.
	rels := []issue.Relationship{
		mustRel(t, "NP-aaaaa", "NP-bbbbb", issue.RelBlockedBy),
		mustRel(t, "NP-bbbbb", "NP-aaaaa", issue.RelBlocks),
		mustRel(t, "NP-aaaaa", "NP-ccccc", issue.RelCites),
		mustRel(t, "NP-ccccc", "NP-aaaaa", issue.RelCitedBy),
	}

	// When: filtering for blocked_by and blocks.
	filtered := relcmd.FilterRelationships(rels, issue.RelBlockedBy, issue.RelBlocks)

	// Then: only blocking relationships are returned.
	if len(filtered) != 2 {
		t.Fatalf("count: got %d, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.Type() != issue.RelBlockedBy && r.Type() != issue.RelBlocks {
			t.Errorf("unexpected relationship type: %s", r.Type())
		}
	}
}

func TestFilterRelationships_Cites_ReturnsOnlyCitationRels(t *testing.T) {
	t.Parallel()

	// Given
	rels := []issue.Relationship{
		mustRel(t, "NP-aaaaa", "NP-bbbbb", issue.RelBlockedBy),
		mustRel(t, "NP-aaaaa", "NP-ccccc", issue.RelCites),
		mustRel(t, "NP-ccccc", "NP-aaaaa", issue.RelCitedBy),
	}

	// When
	filtered := relcmd.FilterRelationships(rels, issue.RelCites, issue.RelCitedBy)

	// Then
	if len(filtered) != 2 {
		t.Fatalf("count: got %d, want 2", len(filtered))
	}
	for _, r := range filtered {
		if r.Type() != issue.RelCites && r.Type() != issue.RelCitedBy {
			t.Errorf("unexpected relationship type: %s", r.Type())
		}
	}
}

func mustRel(t *testing.T, source, target string, relType issue.RelationType) issue.Relationship {
	t.Helper()
	srcID, err := issue.ParseID(source)
	if err != nil {
		t.Fatalf("precondition: invalid source ID: %v", err)
	}
	tgtID, err := issue.ParseID(target)
	if err != nil {
		t.Fatalf("precondition: invalid target ID: %v", err)
	}
	rel, err := issue.NewRelationship(srcID, tgtID, relType)
	if err != nil {
		t.Fatalf("precondition: invalid relationship: %v", err)
	}
	return rel
}
