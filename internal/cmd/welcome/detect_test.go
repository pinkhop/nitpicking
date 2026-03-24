package welcome_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/welcome"
)

func TestIsDatabaseInitialized_WithNpDir_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o755); err != nil {
		t.Fatalf("precondition: mkdir .np: %v", err)
	}

	// When
	result := welcome.IsDatabaseInitialized(dir)

	// Then
	if !result {
		t.Error("expected true when .np/ exists")
	}
}

func TestIsDatabaseInitialized_WithoutNpDir_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()

	// When
	result := welcome.IsDatabaseInitialized(dir)

	// Then
	if result {
		t.Error("expected false when .np/ does not exist")
	}
}

func TestIsGitIgnored_WithNpPattern_ReturnsTrue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{"trailing slash", "node_modules/\n.np/\n"},
		{"no trailing slash", ".np\n"},
		{"absolute path", "/.np/\n"},
		{"with surrounding lines", "dist/\n.np/\ncoverage/\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.content), 0o644); err != nil {
				t.Fatalf("precondition: write .gitignore: %v", err)
			}

			// When
			result := welcome.IsGitIgnored(dir)

			// Then
			if !result {
				t.Errorf("expected true for .gitignore content %q", tc.content)
			}
		})
	}
}

func TestIsGitIgnored_WithoutPattern_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\ndist/\n"), 0o644); err != nil {
		t.Fatalf("precondition: write .gitignore: %v", err)
	}

	// When
	result := welcome.IsGitIgnored(dir)

	// Then
	if result {
		t.Error("expected false when .np/ is not in .gitignore")
	}
}

func TestIsGitIgnored_NoGitignore_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()

	// When
	result := welcome.IsGitIgnored(dir)

	// Then
	if result {
		t.Error("expected false when .gitignore does not exist")
	}
}

func TestHasAgentInstructions_WithNpInClaudeMd_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	content := "# Instructions\n\nUse `np` to track work.\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("precondition: write CLAUDE.md: %v", err)
	}

	// When
	result := welcome.HasAgentInstructions(dir)

	// Then
	if !result {
		t.Error("expected true when CLAUDE.md mentions np")
	}
}

func TestHasAgentInstructions_WithNpInAgentsMd_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	content := "np is the issue tracker.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("precondition: write AGENTS.md: %v", err)
	}

	// When
	result := welcome.HasAgentInstructions(dir)

	// Then
	if !result {
		t.Error("expected true when AGENTS.md mentions np")
	}
}

func TestHasAgentInstructions_NoMention_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	content := "# Instructions\n\nSome unrelated content.\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("precondition: write CLAUDE.md: %v", err)
	}

	// When
	result := welcome.HasAgentInstructions(dir)

	// Then
	if result {
		t.Error("expected false when no file mentions np")
	}
}

func TestHasAgentInstructions_NoFiles_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()

	// When
	result := welcome.HasAgentInstructions(dir)

	// Then
	if result {
		t.Error("expected false when no instruction files exist")
	}
}

func TestHasAuthorConfigured_NonEmpty_ReturnsTrue(t *testing.T) {
	t.Parallel()

	// When
	result := welcome.HasAuthorConfigured("alice")

	// Then
	if !result {
		t.Error("expected true for non-empty env value")
	}
}

func TestHasAuthorConfigured_Empty_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// When
	result := welcome.HasAuthorConfigured("")

	// Then
	if result {
		t.Error("expected false for empty env value")
	}
}
