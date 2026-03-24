//go:build e2e

package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// npBinary returns the path to the built np binary. The binary must be
// built before running E2E tests (make build).
func npBinary(t *testing.T) string {
	t.Helper()

	// Look for the binary relative to the project root.
	candidates := []string{
		"../../dist/np",
		filepath.Join(os.Getenv("GOPATH"), "bin", "np"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	t.Fatal("np binary not found — run 'make build' first")
	return ""
}

// runNP executes the np binary in the given directory with the given args.
// It returns stdout, stderr, and the exit code.
func runNP(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	binary := npBinary(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running np: %v", err)
	}

	return outBuf.String(), errBuf.String(), exitCode
}

func TestE2E_InitAndCreate(t *testing.T) {
	// Given
	dir := t.TempDir()

	// When — init
	_, stderr, code := runNP(t, dir, "init", "TEST")

	// Then
	if code != 0 {
		t.Fatalf("init failed with exit code %d, stderr: %s", code, stderr)
	}

	// Verify .np directory exists.
	if _, err := os.Stat(filepath.Join(dir, ".np")); err != nil {
		t.Fatalf("expected .np directory: %v", err)
	}

	// When — create an issue
	stdout, _, code := runNP(t, dir, "create", "--role", "task", "--title", "E2E task", "--author", "e2e-agent", "--json")

	// Then
	if code != 0 {
		t.Fatalf("create failed with exit code %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["title"] != "E2E task" {
		t.Errorf("expected title 'E2E task', got %v", result["title"])
	}
}

func TestE2E_ListJSON(t *testing.T) {
	// Given
	dir := t.TempDir()
	runNP(t, dir, "init", "TEST")
	runNP(t, dir, "create", "--role", "task", "--title", "Task A", "--author", "alice")
	runNP(t, dir, "create", "--role", "task", "--title", "Task B", "--author", "bob")

	// When
	stdout, _, code := runNP(t, dir, "list", "--json")

	// Then
	if code != 0 {
		t.Fatalf("list failed with exit code %d", code)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	items, ok := result["items"].([]any)
	if !ok || len(items) != 2 {
		t.Errorf("expected 2 items, got %v", len(items))
	}
}

func TestE2E_ExitCodes(t *testing.T) {
	// Given
	dir := t.TempDir()
	runNP(t, dir, "init", "TEST")

	// When — show non-existent issue
	_, _, code := runNP(t, dir, "show", "TEST-zzzzz", "--json")

	// Then — exit code 2 (not found)
	if code != 2 {
		t.Errorf("expected exit code 2 (not found), got %d", code)
	}
}

func TestE2E_ShowAuthor_PresentAfterCreate(t *testing.T) {
	// Given — an issue created with --author but without --claim.
	dir := t.TempDir()
	runNP(t, dir, "init", "TEST")
	createOut, _, code := runNP(t, dir, "create", "--role", "task", "--title", "Author round-trip", "--author", "sweet-anchor-jump", "--json")
	if code != 0 {
		t.Fatalf("precondition: create failed with exit code %d", code)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(createOut), &created); err != nil {
		t.Fatalf("precondition: invalid create JSON: %v", err)
	}
	issueID, ok := created["id"].(string)
	if !ok || issueID == "" {
		t.Fatalf("precondition: missing id in create output")
	}

	// When — show the issue as JSON.
	showOut, _, code := runNP(t, dir, "show", issueID, "--json")

	// Then — author field is present and correct.
	if code != 0 {
		t.Fatalf("show failed with exit code %d", code)
	}
	var shown map[string]any
	if err := json.Unmarshal([]byte(showOut), &shown); err != nil {
		t.Fatalf("invalid show JSON: %v\nstdout: %s", err, showOut)
	}
	authorVal, ok := shown["author"].(string)
	if !ok {
		t.Fatalf("expected author field in show JSON, got: %s", showOut)
	}
	if authorVal != "sweet-anchor-jump" {
		t.Errorf("expected author %q, got %q", "sweet-anchor-jump", authorVal)
	}
}

func TestE2E_AgentName(t *testing.T) {
	// Given
	dir := t.TempDir()

	// When — "agent name" doesn't need a database.
	stdout, _, code := runNP(t, dir, "agent", "name")

	// Then
	if code != 0 {
		t.Fatalf("agent name failed with exit code %d", code)
	}
	parts := strings.Split(strings.TrimSpace(stdout), "-")
	if len(parts) != 3 {
		t.Errorf("expected 3-part name, got %q", stdout)
	}
}
