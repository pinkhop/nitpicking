//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

func TestE2E_Help_HelpCommandAlignedWithCategorizedCommands(t *testing.T) {
	// Given — any directory (help doesn't need a database).
	dir := t.TempDir()

	// When — run np --help.
	stdout, stderr, code := runNP(t, dir, "--help")

	// Then — the "help" command line should be indented at 5 spaces
	// (matching the categorized commands), not 3 spaces.
	if code != 0 {
		t.Fatalf("--help failed (exit %d): %s", code, stderr)
	}

	lines := strings.Split(stdout, "\n")
	var helpLine string
	for _, line := range lines {
		if strings.Contains(line, "help, h") && strings.Contains(line, "Shows a list") {
			helpLine = line
			break
		}
	}
	if helpLine == "" {
		t.Fatalf("could not find help command line in output:\n%s", stdout)
	}

	// The help line must start with exactly 5 spaces (matching categorized commands).
	if !strings.HasPrefix(helpLine, "     help") {
		t.Errorf("help command should be indented at 5 spaces, got: %q", helpLine)
	}
}
