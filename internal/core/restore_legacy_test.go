package core

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// --- translateLegacyRelType ---

func TestTranslateLegacyRelType_Cites_TranslatesToRefs(t *testing.T) {
	t.Parallel()

	// Given — a v0.2.0 "cites" relationship record.
	input := domain.BackupRelationshipRecord{
		TargetID: "NP-abc12",
		RelType:  "cites",
	}

	// When
	got, keep := translateLegacyRelType(input)

	// Then — the record is kept and its type is translated to "refs".
	if !keep {
		t.Fatal("cites row should be kept (translated to refs), got keep=false")
	}
	if got.RelType != "refs" {
		t.Errorf("RelType: got %q, want %q", got.RelType, "refs")
	}
	if got.TargetID != input.TargetID {
		t.Errorf("TargetID: got %q, want %q", got.TargetID, input.TargetID)
	}
}

func TestTranslateLegacyRelType_CitedBy_IsDropped(t *testing.T) {
	t.Parallel()

	// Given — a v0.2.0 "cited_by" relationship record.
	input := domain.BackupRelationshipRecord{
		TargetID: "NP-abc12",
		RelType:  "cited_by",
	}

	// When
	_, keep := translateLegacyRelType(input)

	// Then — the record is dropped because cited_by is the redundant inverse
	// of cites/refs, which is symmetric-by-inverse.
	if keep {
		t.Fatal("cited_by row should be dropped, got keep=true")
	}
}

func TestTranslateLegacyRelType_ModernTypes_PassThrough(t *testing.T) {
	t.Parallel()

	cases := []struct {
		relType string
	}{
		{"blocked_by"},
		{"blocks"},
		{"refs"},
	}

	for _, tc := range cases {
		t.Run(tc.relType, func(t *testing.T) {
			t.Parallel()

			// Given — a modern v0.3.0 relationship record.
			input := domain.BackupRelationshipRecord{
				TargetID: "NP-abc12",
				RelType:  tc.relType,
			}

			// When
			got, keep := translateLegacyRelType(input)

			// Then — the record is kept unchanged.
			if !keep {
				t.Fatalf("%s row should be kept, got keep=false", tc.relType)
			}
			if got.RelType != tc.relType {
				t.Errorf("RelType: got %q, want %q", got.RelType, tc.relType)
			}
			if got.TargetID != input.TargetID {
				t.Errorf("TargetID: got %q, want %q", got.TargetID, input.TargetID)
			}
		})
	}
}
