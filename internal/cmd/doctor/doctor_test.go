package doctor

import (
	"testing"
)

// --- checkNpGitIgnored tests ---

func TestCheckNpGitIgnored_Ignored_ReturnsNoFindings(t *testing.T) {
	t.Parallel()

	// Given — a stub that reports the path is ignored.
	stub := func(dir, path string) (bool, error) { return true, nil }

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — no findings.
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckNpGitIgnored_NotIgnored_ReturnsWarning(t *testing.T) {
	t.Parallel()

	// Given — a stub that reports the path is NOT ignored.
	stub := func(dir, path string) (bool, error) { return false, nil }

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — one warning finding about gitignore.
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Category != "gitignore" {
		t.Errorf("category: got %q, want %q", findings[0].Category, "gitignore")
	}
	if findings[0].Severity != "warning" {
		t.Errorf("severity: got %q, want %q", findings[0].Severity, "warning")
	}
}

func TestCheckNpGitIgnored_NotGitRepo_ReturnsNoFindings(t *testing.T) {
	t.Parallel()

	// Given — a stub that returns an error (not a git repo).
	stub := func(dir, path string) (bool, error) {
		return false, errNotGitRepo
	}

	// When
	findings := checkNpGitIgnored("/some/dir", stub)

	// Then — no findings (check is skipped when not in a git repo).
	if len(findings) != 0 {
		t.Errorf("expected 0 findings when not in git repo, got %d", len(findings))
	}
}
