package gitignore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/fix/gitignore"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// --- helpers ---

// setupNpDir creates a temp directory tree with a .np/ directory and returns
// the path to the temp root and D₀ (the directory that contains .np/).
func setupNpDir(t *testing.T) (root, d0 string) {
	t.Helper()
	root = t.TempDir()
	d0 = root
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	return root, d0
}

// writeFile writes content to a file at path, creating parent directories if
// needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("creating dirs for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// readFile reads the named file and returns its content as a string.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test helper reading files in t.TempDir()
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

// run calls gitignore.Run with the given input and returns the output buffer
// and any error.
func run(t *testing.T, input gitignore.RunInput) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	input.Out = &buf
	err := gitignore.Run(t.Context(), input)
	return buf.String(), err
}

// --- amend existing .gitignore ---

func TestRun_ExistingGitignore_AppendsEntry(t *testing.T) {
	t.Parallel()

	// Given — a repo with .git and an existing .gitignore at D₀ containing one rule.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "*.log\n")

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — no error, entry appended, message mentions the path.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readFile(t, gitignorePath)
	if !strings.Contains(content, ".np/") {
		t.Errorf("expected .np/ in .gitignore, got:\n%s", content)
	}
	if !strings.Contains(output, gitignorePath) {
		t.Errorf("expected output to mention %s, got: %q", gitignorePath, output)
	}
	if !strings.HasPrefix(output, "Added") {
		t.Errorf("expected output to start with 'Added', got: %q", output)
	}
}

func TestRun_ExistingGitignore_NonEmptyNoTrailingNewline_InsertsSeparator(t *testing.T) {
	t.Parallel()

	// Given — a .gitignore without a trailing newline.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "*.log")

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — content has a blank line before .np/.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readFile(t, gitignorePath)
	if !strings.Contains(content, "\n\n.np/\n") {
		t.Errorf("expected blank-line separator before .np/, got:\n%q", content)
	}
}

func TestRun_ExistingGitignore_TrailingBlankLine_NoDoubleSeparator(t *testing.T) {
	t.Parallel()

	// Given — a .gitignore that already ends with a blank line.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "*.log\n\n")

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — no triple newline; just .np/ after the existing blank line.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readFile(t, gitignorePath)
	if strings.Contains(content, "\n\n\n") {
		t.Errorf("expected no triple newline, got:\n%q", content)
	}
	if !strings.HasSuffix(strings.TrimRight(content, "\n"), ".np/") {
		t.Errorf("expected .np/ at end, got:\n%q", content)
	}
}

// --- create new .gitignore at repo root ---

func TestRun_NoGitignore_CreatesAtRepoRoot(t *testing.T) {
	t.Parallel()

	// Given — a repo root with .git/ but no .gitignore; .np/ is inside a subdirectory.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	sub := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(sub, ".np"), 0o755); err != nil {
		t.Fatalf("creating sub/.np: %v", err)
	}

	// When — D₀ is the subdirectory (which contains .np/).
	output, err := run(t, gitignore.RunInput{StartDir: sub})
	// Then — .gitignore created at root, output says "Created".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedPath := filepath.Join(root, ".gitignore")
	content := readFile(t, expectedPath)
	if content != ".np/\n" {
		t.Errorf("new .gitignore content: got %q, want %q", content, ".np/\n")
	}
	if !strings.HasPrefix(output, "Created") {
		t.Errorf("expected output to start with 'Created', got: %q", output)
	}
	if !strings.Contains(output, expectedPath) {
		t.Errorf("expected output to mention %s, got: %q", expectedPath, output)
	}
}

func TestRun_GitAsFile_CreatesGitignoreAtRepoRoot(t *testing.T) {
	t.Parallel()

	// Given — .git is a file (worktree/submodule), not a directory.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git"), "gitdir: ../.git/worktrees/foo")
	sub := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(sub, ".np"), 0o755); err != nil {
		t.Fatalf("creating sub/.np: %v", err)
	}

	// When
	output, err := run(t, gitignore.RunInput{StartDir: sub})
	// Then — .gitignore created at root, .git-as-file treated same as directory.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedPath := filepath.Join(root, ".gitignore")
	content := readFile(t, expectedPath)
	if content != ".np/\n" {
		t.Errorf("new .gitignore content: got %q, want %q", content, ".np/\n")
	}
	if !strings.HasPrefix(output, "Created") {
		t.Errorf("expected 'Created' output, got: %q", output)
	}
}

// --- no-op (already ignored) ---

func TestRun_AlreadyIgnored_AllFourForms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		content string
	}{
		{name: ".np", content: ".np\n"},
		{name: ".np/", content: ".np/\n"},
		{name: "/.np", content: "/.np\n"},
		{name: "/.np/", content: "/.np/\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given — .gitignore already contains the matching form.
			_, d0 := setupNpDir(t)
			writeFile(t, filepath.Join(d0, ".git", "config"), "")
			gitignorePath := filepath.Join(d0, ".gitignore")
			writeFile(t, gitignorePath, tc.content)
			before := readFile(t, gitignorePath)

			// When
			output, err := run(t, gitignore.RunInput{StartDir: d0})
			// Then — no error, file unchanged, output mentions "already ignored".
			if err != nil {
				t.Fatalf("[%s] unexpected error: %v", tc.name, err)
			}
			after := readFile(t, gitignorePath)
			if after != before {
				t.Errorf("[%s] .gitignore was modified; before: %q, after: %q", tc.name, before, after)
			}
			if !strings.Contains(output, "already ignored") {
				t.Errorf("[%s] expected 'already ignored' in output, got: %q", tc.name, output)
			}
		})
	}
}

func TestRun_AlreadyIgnored_CommentedLineIgnored(t *testing.T) {
	t.Parallel()

	// Given — .gitignore has a commented-out .np/ line (should not count as ignored).
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "# .np/\n")

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — commented line is not treated as ignored; entry is added.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := readFile(t, gitignorePath)
	if !strings.Contains(content, "\n.np/\n") {
		t.Errorf("expected .np/ to be added after comment, got:\n%s", content)
	}
}

// --- nearest .gitignore wins ---

func TestRun_NearestGitignoreWins(t *testing.T) {
	t.Parallel()

	// Given — a directory tree with .gitignore at both an intermediate dir and
	// the repo root; D₀ is below the intermediate dir.
	//
	//   root/
	//     .git/
	//     .gitignore          ← repo-root gitignore (should NOT be amended)
	//     sub/
	//       .gitignore        ← nearest gitignore (SHOULD be amended)
	//       project/          ← D₀ (contains .np/)
	//         .np/
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	writeFile(t, filepath.Join(root, ".gitignore"), "# root\n")
	sub := filepath.Join(root, "sub")
	writeFile(t, filepath.Join(sub, ".gitignore"), "# sub\n")
	d0 := filepath.Join(sub, "project")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — only sub/.gitignore is amended; root/.gitignore is unchanged.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	subContent := readFile(t, filepath.Join(sub, ".gitignore"))
	if !strings.Contains(subContent, ".np/") {
		t.Errorf("expected .np/ in sub/.gitignore, got:\n%s", subContent)
	}
	rootContent := readFile(t, filepath.Join(root, ".gitignore"))
	if strings.Contains(rootContent, ".np/") {
		t.Errorf("root/.gitignore should not be modified, got:\n%s", rootContent)
	}
}

// --- not in git ---

// TestRun_NotInGit_DoesNotTouchGitignoreAbove is a regression test for a bug
// where findTarget returned the first .gitignore it saw before proving the
// walk was inside a git repository. The fix walks past any .gitignore found
// outside a git repo and returns ErrNotInGit instead of silently modifying it.
func TestRun_NotInGit_DoesNotTouchGitignoreAbove(t *testing.T) {
	t.Parallel()

	// Given — a .gitignore exists above the workspace, but no .git marker
	// exists anywhere in the temp tree (so the walk terminates at the temp
	// root rather than escaping to a real git repository).
	root := t.TempDir()
	strayGitignore := filepath.Join(root, ".gitignore")
	writeFile(t, strayGitignore, "# stray\n")
	before := readFile(t, strayGitignore)

	d0 := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})

	// Then — error wraps ErrNotInGit; the stray .gitignore is untouched.
	if err == nil {
		t.Fatal("expected error when not in a git repository")
	}
	if !errors.Is(err, gitignore.ErrNotInGit) {
		t.Errorf("expected errors.Is(err, gitignore.ErrNotInGit), got: %v", err)
	}
	after := readFile(t, strayGitignore)
	if after != before {
		t.Errorf("stray .gitignore was modified outside a git repo:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestRun_NotInGit_ReturnsErrNotInGit(t *testing.T) {
	t.Parallel()

	// Given — a directory with .np/ but no .git marker anywhere in the temp
	// tree. We seed the StartDir as a deeply nested path so the walk terminates
	// at the temp root rather than escaping to a real git repository.
	root := t.TempDir()
	d0 := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	_, err := run(t, gitignore.RunInput{StartDir: d0})

	// Then — error wraps ErrNotInGit so callers can branch without string
	// matching.
	if err == nil {
		t.Fatal("expected error when not in a git repository")
	}
	if !errors.Is(err, gitignore.ErrNotInGit) {
		t.Errorf("expected errors.Is(err, gitignore.ErrNotInGit), got: %v", err)
	}
}

// --- dry-run ---

func TestRun_DryRun_WouldAmend_NoChanges(t *testing.T) {
	t.Parallel()

	// Given — existing .gitignore without .np/.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "*.log\n")
	before := readFile(t, gitignorePath)

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, DryRun: true})
	// Then — no error, file unchanged, output says "Would add".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := readFile(t, gitignorePath)
	if after != before {
		t.Error("dry-run modified the .gitignore file")
	}
	if !strings.HasPrefix(output, "Would add") {
		t.Errorf("expected 'Would add' output, got: %q", output)
	}
	if !strings.Contains(output, "Re-run without --dry-run") {
		t.Errorf("expected re-run hint in output, got: %q", output)
	}
}

func TestRun_DryRun_WouldCreate_NoChanges(t *testing.T) {
	t.Parallel()

	// Given — repo with .git but no .gitignore.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	d0 := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, DryRun: true})
	// Then — no error, .gitignore not created, output says "Would create".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".gitignore")); statErr == nil {
		t.Error("dry-run should not create .gitignore")
	}
	if !strings.HasPrefix(output, "Would create") {
		t.Errorf("expected 'Would create' output, got: %q", output)
	}
	if !strings.Contains(output, "Re-run without --dry-run") {
		t.Errorf("expected re-run hint in output, got: %q", output)
	}
}

// --- JSON output ---

func TestRun_JSON_Added(t *testing.T) {
	t.Parallel()

	// Given — existing .gitignore without .np/.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	writeFile(t, filepath.Join(d0, ".gitignore"), "*.log\n")

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, JSON: true})
	// Then — valid JSON with added=true, created=false.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if result["added"] != true {
		t.Errorf("added: got %v, want true", result["added"])
	}
	if result["created"] != false {
		t.Errorf("created: got %v, want false", result["created"])
	}
	if result["path"] == "" {
		t.Error("path must be non-empty")
	}
}

func TestRun_JSON_Created(t *testing.T) {
	t.Parallel()

	// Given — repo with .git but no .gitignore.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	d0 := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, JSON: true})
	// Then — valid JSON with added=true, created=true.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if result["added"] != true {
		t.Errorf("added: got %v, want true", result["added"])
	}
	if result["created"] != true {
		t.Errorf("created: got %v, want true", result["created"])
	}
}

func TestRun_JSON_AlreadyIgnored(t *testing.T) {
	t.Parallel()

	// Given — .gitignore already has .np/.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	writeFile(t, filepath.Join(d0, ".gitignore"), ".np/\n")

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, JSON: true})
	// Then — valid JSON with added=false, reason="already_ignored".
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if result["added"] != false {
		t.Errorf("added: got %v, want false", result["added"])
	}
	if result["reason"] != "already_ignored" {
		t.Errorf("reason: got %v, want already_ignored", result["reason"])
	}
}

func TestRun_JSON_DryRunWouldAdd(t *testing.T) {
	t.Parallel()

	// Given — existing .gitignore without .np/.
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	writeFile(t, filepath.Join(d0, ".gitignore"), "*.log\n")

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, JSON: true, DryRun: true})
	// Then — valid JSON with would_add=true, would_create=false.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if result["would_add"] != true {
		t.Errorf("would_add: got %v, want true", result["would_add"])
	}
	if result["would_create"] != false {
		t.Errorf("would_create: got %v, want false", result["would_create"])
	}
}

func TestRun_JSON_DryRunWouldCreate(t *testing.T) {
	t.Parallel()

	// Given — repo with .git but no .gitignore.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	d0 := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(d0, ".np"), 0o755); err != nil {
		t.Fatalf("creating d0/.np: %v", err)
	}

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0, JSON: true, DryRun: true})
	// Then — valid JSON with would_add=true, would_create=true.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, output)
	}
	if result["would_add"] != true {
		t.Errorf("would_add: got %v, want true", result["would_add"])
	}
	if result["would_create"] != true {
		t.Errorf("would_create: got %v, want true", result["would_create"])
	}
}

// --- idempotency ---

// TestRun_Idempotent_AlreadyAppended verifies that running the fix on a
// .gitignore that already contains the ".np/" form (as a prior run would have
// left it) is a no-op.
func TestRun_Idempotent_AlreadyAppended(t *testing.T) {
	t.Parallel()

	// Given — a .gitignore that already ends with a blank-line-separated ".np/"
	// entry, exactly as appendEntry would produce after amending "*.log\n".
	_, d0 := setupNpDir(t)
	writeFile(t, filepath.Join(d0, ".git", "config"), "")
	gitignorePath := filepath.Join(d0, ".gitignore")
	writeFile(t, gitignorePath, "*.log\n\n.np/\n")
	before := readFile(t, gitignorePath)

	// When
	output, err := run(t, gitignore.RunInput{StartDir: d0})
	// Then — no error, file unchanged, output reports already-ignored.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := readFile(t, gitignorePath)
	if after != before {
		t.Errorf(".gitignore was modified:\nbefore: %q\nafter:  %q", before, after)
	}
	if !strings.Contains(output, "already ignored") {
		t.Errorf("expected 'already ignored' in output, got: %q", output)
	}
}

// --- NewCmd flag wiring ---

// TestNewCmd_FlagsParsed verifies that --dry-run and --json are correctly wired
// into the RunInput that reaches the business logic.
func TestNewCmd_FlagsParsed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantDryRun bool
		wantJSON   bool
	}{
		{name: "no flags", args: []string{"git-ignore"}, wantDryRun: false, wantJSON: false},
		{name: "--dry-run", args: []string{"git-ignore", "--dry-run"}, wantDryRun: true, wantJSON: false},
		{name: "--json", args: []string{"git-ignore", "--json"}, wantDryRun: false, wantJSON: true},
		{name: "both flags", args: []string{"git-ignore", "--dry-run", "--json"}, wantDryRun: true, wantJSON: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given — a factory with a working DatabasePath stub and a runFn that
			// captures the RunInput for inspection.
			root := t.TempDir()
			npPath := filepath.Join(root, ".np", "np.db")
			if err := os.MkdirAll(filepath.Dir(npPath), 0o755); err != nil {
				t.Fatalf("creating .np dir: %v", err)
			}
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams:    ios,
				DatabasePath: func() (string, error) { return npPath, nil },
			}

			var capturedInput gitignore.RunInput
			runFn := func(_ context.Context, input gitignore.RunInput) error {
				capturedInput = input
				return nil
			}

			// When
			cmd := gitignore.NewCmd(f, runFn)
			if err := cmd.Run(t.Context(), tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Then — RunInput fields match the flags.
			if capturedInput.DryRun != tc.wantDryRun {
				t.Errorf("DryRun: got %v, want %v", capturedInput.DryRun, tc.wantDryRun)
			}
			if capturedInput.JSON != tc.wantJSON {
				t.Errorf("JSON: got %v, want %v", capturedInput.JSON, tc.wantJSON)
			}
		})
	}
}

// TestNewCmd_StartDirDerivedFromDatabase verifies that the StartDir field in
// RunInput is the directory containing .np/, not .np/ itself.
func TestNewCmd_StartDirDerivedFromDatabase(t *testing.T) {
	t.Parallel()

	// Given — a DatabasePath that returns /some/project/.np/np.db.
	root := t.TempDir()
	npPath := filepath.Join(root, ".np", "np.db")
	if err := os.MkdirAll(filepath.Dir(npPath), 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams:    ios,
		DatabasePath: func() (string, error) { return npPath, nil },
	}

	var capturedInput gitignore.RunInput
	runFn := func(_ context.Context, input gitignore.RunInput) error {
		capturedInput = input
		return nil
	}

	// When
	cmd := gitignore.NewCmd(f, runFn)
	if err := cmd.Run(t.Context(), []string{"git-ignore"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Then — StartDir is the directory that contains .np/, not .np/ itself.
	expectedStartDir := root
	if capturedInput.StartDir != expectedStartDir {
		t.Errorf("StartDir: got %q, want %q", capturedInput.StartDir, expectedStartDir)
	}
}
