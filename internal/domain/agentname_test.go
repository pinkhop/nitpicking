package domain_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestGenerateAgentName_ProducesThreePartName(t *testing.T) {
	t.Parallel()

	// When
	name := domain.GenerateAgentName()

	// Then
	parts := strings.Split(name, "-")
	if len(parts) != 3 {
		t.Errorf("expected 3 parts, got %d in %q", len(parts), name)
	}
	for _, part := range parts {
		if part == "" {
			t.Errorf("expected non-empty parts in %q", name)
		}
	}
}

func TestGenerateAgentName_ProducesVariedNames(t *testing.T) {
	t.Parallel()

	// When — generate several names
	names := make(map[string]bool)
	for range 10 {
		names[domain.GenerateAgentName()] = true
	}

	// Then — at least 2 unique names (extremely high probability)
	if len(names) < 2 {
		t.Error("expected varied names across 10 generations")
	}
}
