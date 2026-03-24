// Package quickstart implements the setup guide detection logic and rendering
// for the "np quickstart" command.
package quickstart

import (
	"os"
	"path/filepath"
	"strings"
)

// IsDatabaseInitialized reports whether an np database exists at the given
// directory by checking for the presence of a .np/ subdirectory.
func IsDatabaseInitialized(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".np"))
	return err == nil && info.IsDir()
}

// IsGitIgnored reports whether .np/ is listed in the .gitignore file at the
// given directory. Checks for patterns that would exclude the .np directory:
// ".np/", ".np", or "/.np/".
func IsGitIgnored(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return false
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == ".np/" || trimmed == ".np" || trimmed == "/.np/" || trimmed == "/.np" {
			return true
		}
	}
	return false
}

// HasAgentInstructions reports whether at least one agent instruction file
// (CLAUDE.md or AGENTS.md) exists at the given directory and contains a
// reference to np.
func HasAgentInstructions(dir string) bool {
	candidates := []string{"CLAUDE.md", "AGENTS.md", filepath.Join(".github", "copilot-instructions.md")}
	for _, name := range candidates {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "np ") || strings.Contains(content, "`np`") {
			return true
		}
	}
	return false
}

// HasAuthorConfigured reports whether the NP_AUTHOR environment variable
// has a non-empty value.
func HasAuthorConfigured(envValue string) bool {
	return envValue != ""
}
