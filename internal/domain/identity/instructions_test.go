package identity_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
)

func TestAgentInstructions_ContainsCoreWorkflowSections(t *testing.T) {
	t.Parallel()

	// When
	output := identity.AgentInstructions()

	// Then
	required := []string{
		"np claim",
		"np update",
		"np close",
		"np release",
		"claim ID",
		"np --help",
		"np next",
		"exclusive",
	}

	for _, keyword := range required {
		if !strings.Contains(output, keyword) {
			t.Errorf("expected instructions to contain %q", keyword)
		}
	}
}

func TestAgentInstructions_IsNonEmpty(t *testing.T) {
	t.Parallel()

	// When
	output := identity.AgentInstructions()

	// Then
	if len(output) < 100 {
		t.Errorf("expected substantial instructions, got %d bytes", len(output))
	}
}
