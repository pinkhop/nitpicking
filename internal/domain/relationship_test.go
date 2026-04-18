package domain_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestNewRelationship_ValidRelationship_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	src := mustID(t)
	tgt := mustID(t)

	// When
	rel, err := domain.NewRelationship(src, tgt, domain.RelBlockedBy)
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
	if rel.Type() != domain.RelBlockedBy {
		t.Errorf("expected blocked_by, got %s", rel.Type())
	}
}

func TestNewRelationship_SelfRelationship_Fails(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	_, err := domain.NewRelationship(id, id, domain.RelRefs)

	// Then
	if err == nil {
		t.Error("expected error for self-relationship")
	}
}

func TestNewRelationship_ZeroIDs_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewRelationship(domain.ID{}, mustID(t), domain.RelRefs)

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
	rel, err := domain.NewRelationship(src, tgt, domain.RelRefs)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.Type() != domain.RelRefs {
		t.Errorf("expected refs, got %s", rel.Type())
	}
}

func TestRelationType_Refs_IsSymmetric(t *testing.T) {
	t.Parallel()

	// When
	isSymmetric := domain.RelRefs.IsSymmetric()

	// Then
	if !isSymmetric {
		t.Error("expected refs to be symmetric")
	}
}

func TestRelationType_BlockedBy_IsNotSymmetric(t *testing.T) {
	t.Parallel()

	// When
	isSymmetric := domain.RelBlockedBy.IsSymmetric()

	// Then
	if isSymmetric {
		t.Error("expected blocked_by to not be symmetric")
	}
}

func TestRelationType_Refs_InverseIsSelf(t *testing.T) {
	t.Parallel()

	// When
	inv := domain.RelRefs.Inverse()

	// Then — symmetric types are their own inverse.
	if inv != domain.RelRefs {
		t.Errorf("expected refs inverse to be refs, got %s", inv)
	}
}

func TestParseRelationType_Refs_Succeeds(t *testing.T) {
	t.Parallel()

	// When
	rt, err := domain.ParseRelationType("refs")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != domain.RelRefs {
		t.Errorf("expected RelRefs, got %v", rt)
	}
}

func TestRelationType_Inverse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    domain.RelationType
		expected domain.RelationType
	}{
		{domain.RelBlockedBy, domain.RelBlocks},
		{domain.RelBlocks, domain.RelBlockedBy},
		{domain.RelRefs, domain.RelRefs},
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
		expected domain.RelationType
	}{
		{"blocked_by", domain.RelBlockedBy},
		{"blocks", domain.RelBlocks},
		{"refs", domain.RelRefs},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When
			rt, err := domain.ParseRelationType(tc.input)
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
	_, err := domain.ParseRelationType("depends_on")

	// Then
	if err == nil {
		t.Error("expected error for invalid relationship type")
	}
}
