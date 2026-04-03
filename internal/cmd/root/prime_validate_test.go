package root

import (
	"regexp"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/agent"
)

// TestAgentPrimeCommands_MatchCommandTree validates that every np command line
// in the agent prime output refers to a real command path with valid flags. This
// catches drift between the agent instructions and the CLI — e.g., nonexistent
// commands like "np done" or phantom flags like "--author" on "np close".
func TestAgentPrimeCommands_MatchCommandTree(t *testing.T) {
	t.Parallel()

	// Given — the agent prime instructions and the full command tree.
	instructions := agent.AgentInstructions()
	rootCmd := NewRootCmd(newTestFactory())

	// Extract np command invocations from fenced code blocks and inline
	// backtick-delimited code spans.
	invocations := extractNPInvocations(t, instructions)
	if len(invocations) == 0 {
		t.Fatal("no np invocations found in agent prime output")
	}

	// Collect global flags from the root command so they can be recognized
	// on any subcommand (urfave/cli propagates these at runtime).
	globalFlags := make(map[string]bool)
	for _, f := range rootCmd.Flags {
		for _, n := range f.Names() {
			globalFlags[n] = true
		}
	}

	// When / Then — validate each invocation against the command tree.
	for _, inv := range invocations {
		t.Run(inv.raw, func(t *testing.T) {
			t.Parallel()

			cmd := resolveCommand(t, rootCmd, inv.path)
			if cmd == nil {
				return // already reported
			}

			for _, flag := range inv.flags {
				if !commandHasFlag(cmd, flag) && !globalFlags[flag] {
					t.Errorf("command %q does not have flag --%s", strings.Join(inv.path, " "), flag)
				}
			}
		})
	}
}

// npInvocation represents a parsed np command line from the instructions.
type npInvocation struct {
	raw   string   // original text for subtest naming
	path  []string // command path segments, e.g. ["json", "update"]
	flags []string // long flag names (without --), e.g. ["claim"]
}

// codeBlockRe matches fenced code blocks (```...```).
var codeBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

// inlineCodeRe matches inline backtick code spans (`...`).
var inlineCodeRe = regexp.MustCompile("`([^`]+)`")

// flagRe matches --flag-name patterns, capturing the flag name.
var flagRe = regexp.MustCompile(`--([a-z][-a-z0-9]*)`)

// extractNPInvocations parses all np command invocations from the instructions
// text. It looks in both fenced code blocks and inline code spans.
func extractNPInvocations(t *testing.T, text string) []npInvocation {
	t.Helper()

	var invocations []npInvocation
	seen := make(map[string]bool)

	// Helper to process a single line that might contain an np command.
	processLine := func(line string) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "np ") {
			return
		}

		// Strip trailing comments (# ...).
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}

		// Deduplicate.
		if seen[line] {
			return
		}
		seen[line] = true

		inv := parseNPCommand(line)
		invocations = append(invocations, inv)
	}

	// Process fenced code blocks and remove them from text so inline code
	// span extraction below does not accidentally match across code fences.
	remaining := codeBlockRe.ReplaceAllStringFunc(text, func(block string) string {
		content := codeBlockRe.FindStringSubmatch(block)[1]
		for _, line := range strings.Split(content, "\n") {
			processLine(line)
		}
		return "" // remove from text for inline processing
	})

	// Process inline code spans from text outside fenced code blocks.
	for _, match := range inlineCodeRe.FindAllStringSubmatch(remaining, -1) {
		processLine(match[1])
	}

	return invocations
}

// parseNPCommand parses a single np command line into path segments and flags.
// Example: "np json update --claim <CID>"
// → path: ["json", "update"], flags: ["claim"]
func parseNPCommand(line string) npInvocation {
	// Remove the "np " prefix.
	line = strings.TrimPrefix(line, "np ")

	inv := npInvocation{raw: "np " + line}

	// Split into tokens.
	tokens := strings.Fields(line)

	// Walk tokens to separate command path from arguments and flags.
	for _, tok := range tokens {
		if strings.HasPrefix(tok, "--") {
			// It's a flag.
			matches := flagRe.FindStringSubmatch(tok)
			if len(matches) == 2 {
				inv.flags = append(inv.flags, matches[1])
			}
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) == 2 {
			// Short flag like -r, -t — skip (we validate long names only).
			continue
		}
		if strings.HasPrefix(tok, "<") || strings.HasPrefix(tok, "\"") || strings.HasPrefix(tok, "'") {
			// Placeholder or quoted value — stop collecting path segments.
			continue
		}

		// If we haven't seen a flag yet, this is part of the command path.
		if len(inv.flags) == 0 {
			inv.path = append(inv.path, tok)
		}
	}

	return inv
}

// resolveCommand walks the command tree to find the command at the given path.
// Reports a test error and returns nil if the path cannot be resolved. When a
// segment cannot be found as a subcommand and the current command has no
// subcommands at all, the remaining segments are treated as positional
// arguments and resolution stops — this handles commands like "np claim ready"
// where "ready" is a positional argument, not a subcommand.
func resolveCommand(t *testing.T, root *cli.Command, path []string) *cli.Command {
	t.Helper()

	current := root
	for i, segment := range path {
		found := false
		for _, sub := range current.Commands {
			if sub.Name == segment {
				current = sub
				found = true
				break
			}
		}
		if !found {
			// If the current command has no subcommands, remaining path
			// segments are positional arguments — stop resolution here.
			if len(current.Commands) == 0 {
				break
			}
			t.Errorf("command path %q: no subcommand %q at position %d",
				strings.Join(path, " "), segment, i)
			return nil
		}
	}
	return current
}

// commandHasFlag checks whether a command has a flag with the given long name
// in its own Flags list. Global (root) flags are checked separately by the
// caller since urfave/cli's parent chain is not accessible in static analysis.
func commandHasFlag(cmd *cli.Command, name string) bool {
	for _, f := range cmd.Flags {
		for _, n := range f.Names() {
			if n == name {
				return true
			}
		}
	}
	return false
}
