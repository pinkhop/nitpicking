package agent_test

import (
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/agent"
)

func TestAgentInstructions_ContainsCoreWorkflowSections(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then
	required := []string{
		"np claim ready",
		"np json update",
		"np close",
		"np issue release",
		"claim",
		"np json create",
		"np json comment",
		"exclusive",
		"np issue defer",
		"np issue search",
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
	output := agent.AgentInstructions()

	// Then
	if len(output) < 100 {
		t.Errorf("expected substantial instructions, got %d bytes", len(output))
	}
}

func TestAgentInstructions_IntroductionOpensWithWorkspaceUsesNitpicking(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the very first line introduces nitpicking as the workspace's
	// issue tracker, suitable for a file titled "Issue Tracking".
	wantPrefix := "This workspace uses the nitpicking"
	if !strings.HasPrefix(output, wantPrefix) {
		firstLine, _, _ := strings.Cut(output, "\n")
		t.Errorf("expected output to start with %q, got %q", wantPrefix, firstLine)
	}
}

func TestAgentInstructions_IntroductionDescribesLocalOnlyNature(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the introduction mentions local-only nature following the
	// lead sentence, using "Nitpicking is" phrasing.
	if !strings.Contains(output, "Nitpicking is local-only") {
		t.Error("expected instructions to contain \"Nitpicking is local-only\"")
	}
}

func TestAgentInstructions_DoesNotMentionEmbeddedSQLite(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the implementation-detail phrase about SQLite storage is
	// removed in favour of a parallel-access safety description.
	if strings.Contains(output, "in an embedded SQLite database") {
		t.Error("expected instructions NOT to contain \"in an embedded SQLite database\"")
	}
}

func TestAgentInstructions_MentionsParallelAccessSafety(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — parallel-access safety replaces the SQLite implementation
	// detail.
	if !strings.Contains(output, "parallel") {
		t.Error("expected instructions to mention parallel-access safety")
	}
}

func TestAgentInstructions_IncludesAgentNameInstructions(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — agents are told how to obtain an author name.
	if !strings.Contains(output, "np agent name") {
		t.Error("expected instructions to contain \"np agent name\"")
	}
}

func TestAgentInstructions_CloseCommandIsCorrect(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the close command must use "np close", not "np done".
	if strings.Contains(output, "np done") {
		t.Error("instructions contain 'np done' which is not a valid command; should be 'np close'")
	}
	// The close command must not include --author (np close does not accept it).
	if strings.Contains(output, "np close") {
		// Find the close command line and verify no --author.
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "np close") && strings.Contains(line, "--author") {
				t.Error("np close command includes --author flag, which is not accepted by the CLI")
			}
		}
	}
}

func TestAgentInstructions_UpdateCommandUsesJSONUpdate(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the update command must use "np json update", not "np issue update".
	if !strings.Contains(output, "np json update") {
		t.Error("expected instructions to contain \"np json update\"")
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "np issue update") {
			t.Errorf("instructions contain removed 'np issue update' command: %s", line)
		}
	}
}

func TestAgentInstructions_CommentCommandUsesJSONComment(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the comment command must use "np json comment", not "np comment add".
	if !strings.Contains(output, "np json comment") {
		t.Error("expected instructions to contain \"np json comment\"")
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "np comment add") {
			t.Errorf("instructions contain removed 'np comment add' command: %s", line)
		}
	}
}

func TestAgentInstructions_DocumentsDeferredWorkflow(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the three-step workflow for
	// creating a deferred issue (create claimed, defer, release) instead of
	// a deferred creation-time field.
	if !strings.Contains(output, "np issue defer --claim") {
		t.Error("expected instructions to document the three-step deferred workflow")
	}
	if strings.Contains(output, `"deferred": true`) || strings.Contains(output, `"deferred":true`) {
		t.Error("instructions must not reference the removed deferred creation-time field")
	}
}

func TestAgentInstructions_DoesNotMentionImportJson(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — import json has been removed; the instructions must not
	// reference it as a command.
	if strings.Contains(output, "np import json") {
		t.Error("expected instructions NOT to contain 'np import json' — command has been removed")
	}
}

func TestAgentInstructions_DocumentsWithLabelFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --with-label flag so agents
	// know they can filter claims by label.
	if !strings.Contains(output, "--with-label") {
		t.Error("expected instructions to document --with-label flag")
	}
}

func TestAgentInstructions_DocumentsWithRoleFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --with-role flag so agents
	// know they can filter claims by role.
	if !strings.Contains(output, "--with-role") {
		t.Error("expected instructions to document --with-role flag")
	}
}

func TestAgentInstructions_DocumentsDurationFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --duration flag so agents
	// know they can control claim staleness timing.
	if !strings.Contains(output, "--duration") {
		t.Error("expected instructions to document --duration flag")
	}
}

func TestAgentInstructions_DocumentsStaleAtFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --stale-at flag so agents
	// know they can set an absolute stale time.
	if !strings.Contains(output, "--stale-at") {
		t.Error("expected instructions to document --stale-at flag")
	}
}

func TestAgentInstructions_DoesNotMentionOldFlagNames(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — old flag names that were renamed in the claim refactor must
	// not appear in the instructions.
	oldFlags := []string{
		"--stale-threshold",
		"--steal-if-needed",
		"--label ", // bare --label (not --with-label)
		"--role ",  // bare --role (not --with-role)
	}
	for _, old := range oldFlags {
		if strings.Contains(output, old) {
			t.Errorf("expected instructions NOT to contain old flag %q", old)
		}
	}
}

func TestAgentInstructions_DoesNotMentionStealFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the --steal flag and stealing mechanics have been removed from the
	// state model; they must not appear in the instructions.
	stealTerms := []string{
		"--steal",
		"stealing",
		"steal a claim",
	}
	for _, term := range stealTerms {
		if strings.Contains(output, term) {
			t.Errorf("expected instructions NOT to contain steal-related term %q", term)
		}
	}
}

func TestAgentInstructions_DoesNotTreatClaimedAsPrimaryState(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — "claimed" is a secondary state of open, not a primary lifecycle
	// state. Phrases that imply claimed is a distinct primary state must not
	// appear in the instructions.
	primaryStateTerms := []string{
		"in the claimed state",
		"state: claimed",
		`"claimed"`,
	}
	for _, term := range primaryStateTerms {
		if strings.Contains(output, term) {
			t.Errorf("expected instructions NOT to contain phrase implying claimed is a primary state: %q", term)
		}
	}
}

func TestAgentInstructions_ReleaseDeletesLocalClaim(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must explain that release deletes the local
	// claim record without changing the issue's primary state.
	wantPhrase := "deletes the local claim"
	if !strings.Contains(output, wantPhrase) {
		t.Errorf("expected instructions to explain that release %q", wantPhrase)
	}
}

func TestAgentInstructions_NoEchoPipeJSONExamples(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — all JSON examples must use heredoc syntax, not echo-pipe.
	for i, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "echo") && strings.Contains(line, "| np") {
			t.Errorf("line %d still uses echo-pipe pattern: %s", i+1, line)
		}
	}
}

func TestAgentInstructions_UsesHeredocSyntax(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — heredoc examples must use single-quoted JSONEND delimiter
	// to prevent shell expansion.
	if !strings.Contains(output, "<<'JSONEND'") {
		t.Error("expected instructions to use <<'JSONEND' heredoc syntax")
	}
}

func TestAgentInstructions_StartsWithContentImmediately(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — no leading blank lines or whitespace.
	if output != strings.TrimLeft(output, "\n\r \t") {
		t.Error("expected output to start immediately with content, found leading whitespace")
	}
}
