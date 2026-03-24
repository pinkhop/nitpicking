package issue_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestNewRelationship_ValidRelationship_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	src := mustID(t)
	tgt := mustID(t)

	// When
	rel, err := issue.NewRelationship(src, tgt, issue.RelBlockedBy)
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
	if rel.Type() != issue.RelBlockedBy {
		t.Errorf("expected blocked_by, got %s", rel.Type())
	}
}

func TestNewRelationship_SelfRelationship_Fails(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	_, err := issue.NewRelationship(id, id, issue.RelCites)

	// Then
	if err == nil {
		t.Error("expected error for self-relationship")
	}
}

func TestNewRelationship_ZeroIDs_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := issue.NewRelationship(issue.ID{}, mustID(t), issue.RelCites)

	// Then
	if err == nil {
		t.Error("expected error for zero source ID")
	}
}

func TestNewRelationship_Refs_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	src := mustID(t)
	tgt := mustID(t)

	// When
	rel, err := issue.NewRelationship(src, tgt, issue.RelRefs)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.Type() != issue.RelRefs {
		t.Errorf("expected refs, got %s", rel.Type())
	}
}

func TestRelationType_Refs_IsSymmetric(t *testing.T) {
	t.Parallel()

	// When
	isSymmetric := issue.RelRefs.IsSymmetric()

	// Then
	if !isSymmetric {
		t.Error("expected refs to be symmetric")
	}
}

func TestRelationType_BlockedBy_IsNotSymmetric(t *testing.T) {
	t.Parallel()

	// When
	isSymmetric := issue.RelBlockedBy.IsSymmetric()

	// Then
	if isSymmetric {
		t.Error("expected blocked_by to not be symmetric")
	}
}

func TestRelationType_Refs_InverseIsSelf(t *testing.T) {
	t.Parallel()

	// When
	inv := issue.RelRefs.Inverse()

	// Then — symmetric types are their own inverse.
	if inv != issue.RelRefs {
		t.Errorf("expected refs inverse to be refs, got %s", inv)
	}
}

func TestParseRelationType_Refs_Succeeds(t *testing.T) {
	t.Parallel()

	// When
	rt, err := issue.ParseRelationType("refs")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != issue.RelRefs {
		t.Errorf("expected RelRefs, got %v", rt)
	}
}

func TestRelationType_Inverse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    issue.RelationType
		expected issue.RelationType
	}{
		{issue.RelBlockedBy, issue.RelBlocks},
		{issue.RelBlocks, issue.RelBlockedBy},
		{issue.RelCites, issue.RelCitedBy},
		{issue.RelCitedBy, issue.RelCites},
		{issue.RelRefs, issue.RelRefs},
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
		expected issue.RelationType
	}{
		{"blocked_by", issue.RelBlockedBy},
		{"blocks", issue.RelBlocks},
		{"cites", issue.RelCites},
		{"cited_by", issue.RelCitedBy},
		{"refs", issue.RelRefs},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			rt, err := issue.ParseRelationType(tc.input)
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
	_, err := issue.ParseRelationType("depends_on")

	// Then
	if err == nil {
		t.Error("expected error for invalid relationship type")
	}
}
