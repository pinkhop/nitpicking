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

func TestAgentInstructions_DocumentsLabelFlagOnClaim(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --label flag on np claim ready
	// so agents know they can filter claims by label.
	if !strings.Contains(output, "claim ready --label") {
		t.Error("expected instructions to document --label flag on np claim ready")
	}
}

func TestAgentInstructions_DocumentsRoleFlagOnClaim(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --role flag on np claim ready
	// so agents know they can filter claims by role.
	if !strings.Contains(output, "claim ready --role") {
		t.Error("expected instructions to document --role flag on np claim ready")
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

	// Then — old flag names that were removed in previous refactors must not
	// appear anywhere in the instructions.
	oldFlags := []string{
		"--stale-threshold",
		"--steal-if-needed",
		"--with-role",
		"--with-label",
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

func TestAgentInstructions_DocumentsColumnsFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --columns flag so agents
	// know they can select and reorder columns in tabular output.
	if !strings.Contains(output, "--columns") {
		t.Error("expected instructions to document --columns flag")
	}
}

func TestAgentInstructions_DocumentsDefaultColumns(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must list the default column set so agents
	// understand what columns appear without explicit selection.
	defaultCols := []string{"ID", "PRIORITY", "ROLE", "STATE", "TITLE"}
	for _, col := range defaultCols {
		if !strings.Contains(output, col) {
			t.Errorf("expected instructions to mention default column %q", col)
		}
	}
}

func TestAgentInstructions_DocumentsValidColumnNames(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must list all valid column names so agents
	// know what values --columns accepts.
	validCols := []string{"ID", "CREATED", "PARENT_ID", "PARENT_CREATED", "PRIORITY", "ROLE", "STATE", "TITLE"}
	for _, col := range validCols {
		if !strings.Contains(output, col) {
			t.Errorf("expected instructions to list valid column name %q", col)
		}
	}
}

func TestAgentInstructions_DocumentsOrderAscDescSuffix(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the :asc and :desc suffixes
	// for the --order flag so agents know how to control sort direction.
	if !strings.Contains(output, ":asc") {
		t.Error("expected instructions to document :asc suffix for --order flag")
	}
	if !strings.Contains(output, ":desc") {
		t.Error("expected instructions to document :desc suffix for --order flag")
	}
}

func TestAgentInstructions_DocumentsOrderFlag(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must document the --order flag so agents
	// know they can control sort order.
	if !strings.Contains(output, "--order") {
		t.Error("expected instructions to document --order flag")
	}
}

// TestAgentInstructions_RecommendsSeededAgentName verifies that the
// "Choosing an Author Name" section instructs agents to invoke
// np agent name --seed=$PPID rather than plain np agent name, and that
// it briefly explains why the seed provides stable identity across restarts.
func TestAgentInstructions_RecommendsSeededAgentName(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the exact seeded invocation form must appear so that agents
	// copy a concrete, correct command rather than needing to discover the
	// flag themselves.
	if !strings.Contains(output, "np agent name --seed=$PPID") {
		t.Error("expected instructions to contain \"np agent name --seed=$PPID\"")
	}
}

// TestAgentInstructions_ExplainsWhySeedingIsUseful verifies that the
// prime output explains the benefit of --seed=$PPID — stable identity
// across restarts — so agents understand the purpose, not just the form.
// The explanation must be tied to the seed (not a generic "pick a stable
// name" sentence) and must mention restart-stability so agents grasp why
// the flag is worth using.
func TestAgentInstructions_ExplainsWhySeedingIsUseful(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — locate the "Choosing an Author Name" section and verify its
	// body connects $PPID to restart-stability. Checking the raw output
	// for individual keywords is not enough because "stable" and similar
	// words occur elsewhere; the explanation must live in the author-name
	// section and reference $PPID by name.
	const sectionHeader = "## Choosing an Author Name"
	sectionStart := strings.Index(output, sectionHeader)
	if sectionStart < 0 {
		t.Fatalf("could not find %q section", sectionHeader)
	}
	sectionEnd := strings.Index(output[sectionStart+len(sectionHeader):], "\n## ")
	if sectionEnd < 0 {
		t.Fatalf("could not find end of %q section", sectionHeader)
	}
	section := output[sectionStart : sectionStart+len(sectionHeader)+sectionEnd]

	if !strings.Contains(section, "$PPID") {
		t.Errorf("expected author-name section to reference $PPID explicitly; section:\n%s", section)
	}
	if !strings.Contains(section, "restart") {
		t.Errorf("expected author-name section to explain restart-stability benefit; section:\n%s", section)
	}
}

// TestAgentInstructions_ForbidsAutoInit verifies that the instructions contain
// an explicit directive telling agents not to run np init or create a
// .np/ directory when no nitpicking database is found, unless the user has
// explicitly asked for initialization.
func TestAgentInstructions_ForbidsAutoInit(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the instructions must name the forbidden command so agents
	// cannot plausibly misread the directive.
	if !strings.Contains(output, "np init") {
		t.Error("expected instructions to mention np init in a prohibition context")
	}
}

// TestAgentInstructions_InitializationSectionExists verifies that the prime
// output contains a dedicated section explaining when initialization is and
// is not appropriate.
func TestAgentInstructions_InitializationSectionExists(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the section must exist so agents can locate the directive quickly.
	if !strings.Contains(output, "## Initialization") {
		t.Error("expected instructions to contain \"## Initialization\" section")
	}
}

// TestAgentInstructions_MissingDotNpMeansAsk verifies that the initialization
// section explicitly instructs agents that a missing .np/ directory is a
// signal to ask the user rather than to auto-initialize.
func TestAgentInstructions_MissingDotNpMeansAsk(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the initialization section must say that a missing .np/ is a
	// signal to ask, not to auto-initialize.
	section, ok := extractSection(t, output, "## Initialization")
	if !ok {
		t.Fatal("could not find \"## Initialization\" section")
	}
	if !strings.Contains(section, "ask") {
		t.Errorf("expected Initialization section to instruct agents to ask the user; section:\n%s", section)
	}
}

// TestAgentInstructions_ExplicitUserRequestRequired verifies that the
// initialization section makes clear that initialization may only happen when
// the user has explicitly requested it.
func TestAgentInstructions_ExplicitUserRequestRequired(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the initialization section must require an explicit user request.
	section, ok := extractSection(t, output, "## Initialization")
	if !ok {
		t.Fatal("could not find \"## Initialization\" section")
	}
	if !strings.Contains(section, "explicit") {
		t.Errorf("expected Initialization section to require an explicit user request; section:\n%s", section)
	}
}

// extractSection returns the body of the Markdown section whose header
// starts with sectionHeader (e.g., "## Claim ID Durability"). The returned
// body spans from the header through the last line before the next "## "
// header, or to end-of-string if the section is the last one. Callers use
// this helper to constrain Contains assertions to a single section, so that
// a keyword appearing elsewhere in the instructions does not produce a
// false positive. Returns an empty string and ok=false when the header
// cannot be found.
func extractSection(t *testing.T, output, sectionHeader string) (string, bool) {
	t.Helper()
	sectionStart := strings.Index(output, sectionHeader)
	if sectionStart < 0 {
		return "", false
	}
	rest := output[sectionStart+len(sectionHeader):]
	sectionEnd := strings.Index(rest, "\n## ")
	if sectionEnd < 0 {
		return rest, true
	}
	return rest[:sectionEnd], true
}

// TestAgentInstructions_ClaimIDDurabilityDirectiveExists verifies that the
// prime output contains a clearly-marked section dedicated to claim ID
// durability — alerting agents that a claim ID is critical state that must
// survive conversation compaction.
func TestAgentInstructions_ClaimIDDurabilityDirectiveExists(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — the section header must exist so agents using a table-of-contents
	// reader can locate the directive quickly.
	if !strings.Contains(output, "## Claim ID Durability") {
		t.Error("expected instructions to contain \"## Claim ID Durability\" section")
	}
}

// TestAgentInstructions_ClaimIDDurabilityDirectiveExplainsCompactionRisk
// verifies that the claim ID durability section explicitly warns that
// conversation compaction can erase the claim ID from working memory,
// which is the failure mode the section exists to prevent.
func TestAgentInstructions_ClaimIDDurabilityDirectiveExplainsCompactionRisk(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — locate the section and confirm it references compaction.
	section, ok := extractSection(t, output, "## Claim ID Durability")
	if !ok {
		t.Fatal("could not find \"## Claim ID Durability\" section")
	}
	if !strings.Contains(section, "compaction") {
		t.Errorf("expected Claim ID Durability section to mention compaction; section:\n%s", section)
	}
}

// TestAgentInstructions_ClaimIDDurabilityForbidsRecordingInComments verifies
// that the directive warns agents NOT to record the claim ID in a comment or
// any other shared durable location. The claim ID is a bearer credential; any
// reader could act on the claim, which defeats the claim system's purpose.
func TestAgentInstructions_ClaimIDDurabilityForbidsRecordingInComments(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — locate the section and confirm it forbids writing the claim ID to
	// a comment (or any similarly shared location).
	section, ok := extractSection(t, output, "## Claim ID Durability")
	if !ok {
		t.Fatal("could not find \"## Claim ID Durability\" section")
	}
	if !strings.Contains(section, "bearer credential") {
		t.Errorf("expected Claim ID Durability section to describe the claim ID as a bearer credential; section:\n%s", section)
	}
	if !strings.Contains(section, "Never record") {
		t.Errorf("expected Claim ID Durability section to explicitly forbid recording the claim ID in shared locations; section:\n%s", section)
	}
}

// TestAgentInstructions_ClaimIDDurabilityStatesNoRecovery verifies that the
// section tells agents there is no recovery if the claim ID is lost, so they
// do not waste time searching for one or invent fragile recovery procedures.
func TestAgentInstructions_ClaimIDDurabilityStatesNoRecovery(t *testing.T) {
	t.Parallel()

	// When
	output := agent.AgentInstructions()

	// Then — locate the section and confirm it states there is no recovery.
	section, ok := extractSection(t, output, "## Claim ID Durability")
	if !ok {
		t.Fatal("could not find \"## Claim ID Durability\" section")
	}
	if !strings.Contains(section, "no recovery") {
		t.Errorf("expected Claim ID Durability section to state there is no recovery if the claim ID is lost; section:\n%s", section)
	}
}
