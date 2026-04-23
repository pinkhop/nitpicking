package domain_test

import (
	"errors"
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

func TestGenerateAgentNameFromSeed_ProducesThreePartName(t *testing.T) {
	t.Parallel()

	// When
	name, err := domain.GenerateAgentNameFromSeed("test-seed")
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

func TestGenerateAgentNameFromSeed_SameSeedProducesSameName(t *testing.T) {
	t.Parallel()

	// When — two calls with the same seed
	first, err1 := domain.GenerateAgentNameFromSeed("stable-seed-42")
	second, err2 := domain.GenerateAgentNameFromSeed("stable-seed-42")

	// Then — both calls succeed and return the same name
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if first != second {
		t.Errorf("expected same name for same seed, got %q and %q", first, second)
	}
}

func TestGenerateAgentNameFromSeed_DifferentSeedsProduceDifferentNames(t *testing.T) {
	t.Parallel()

	// Given — a representative sample of distinct seed strings
	seeds := []string{
		"1234", "5678", "abcd", "efgh", "seed-one",
		"seed-two", "hello", "world", "go-test", "nitpick",
	}

	// When — generate a name for each seed
	names := make(map[string]bool, len(seeds))
	for _, seed := range seeds {
		name, err := domain.GenerateAgentNameFromSeed(seed)
		if err != nil {
			t.Fatalf("unexpected error for seed %q: %v", seed, err)
		}
		names[name] = true
	}

	// Then — at least half of the distinct seeds produce distinct names;
	// some collisions are acceptable, but they must not dominate the sample.
	if len(names) < len(seeds)/2 {
		t.Errorf("expected at least %d distinct names from %d seeds, got %d", len(seeds)/2, len(seeds), len(names))
	}
}

func TestGenerateAgentNameFromSeed_EmptySeedReturnsError(t *testing.T) {
	t.Parallel()

	// When — empty seed string
	name, err := domain.GenerateAgentNameFromSeed("")

	// Then — returns ErrEmptyAgentNameSeed and an empty name
	if !errors.Is(err, domain.ErrEmptyAgentNameSeed) {
		t.Errorf("expected ErrEmptyAgentNameSeed, got %v", err)
	}
	if name != "" {
		t.Errorf("expected empty name on error, got %q", name)
	}
}
