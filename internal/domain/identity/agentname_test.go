package identity_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
)

func TestGenerateAgentName_ProducesThreePartName(t *testing.T) {
	t.Parallel()

	// When
	name, err := identity.GenerateAgentName()
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
		name, err := identity.GenerateAgentName()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		names[name] = true
	}

	// Then — at least 2 unique names (extremely high probability)
	if len(names) < 2 {
		t.Error("expected varied names across 10 generations")
	}
}
