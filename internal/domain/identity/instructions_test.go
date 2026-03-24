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
		"np claim ready",
		"np update",
		"np done",
		"np issue release",
		"claim",
		"np create",
		"np comment",
		"exclusive",
		"np issue defer",
		"np search",
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
