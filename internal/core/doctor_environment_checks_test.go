package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- runAgentInstructions ---

// TestRunAgentInstructions_NoInstructionFiles_ReturnsNoFilesWarning verifies
// that the check emits a finding when neither CLAUDE.md nor AGENTS.md exists.
func TestRunAgentInstructions_NoInstructionFiles_ReturnsNoFilesWarning(t *testing.T) {
	t.Parallel()

	// Given — a temp dir with no CLAUDE.md or AGENTS.md.
	dir := t.TempDir()
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — warning with "no instruction files exist" message.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil (pass)")
	}
	if result.NotApplicable {
		t.Fatal("expected a finding, got not-applicable")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
	want := "no instruction files exist"
	if !strings.Contains(result.Summary, want) {
		t.Errorf("summary %q does not contain %q", result.Summary, want)
	}
}

// TestRunAgentInstructions_CLAUDEMDWithoutNpRef_ReturnsMentionWarning verifies
// that the check emits a finding when CLAUDE.md exists but does not reference np.
func TestRunAgentInstructions_CLAUDEMDWithoutNpRef_ReturnsMentionWarning(t *testing.T) {
	t.Parallel()

	// Given — CLAUDE.md with no np reference.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("some content without reference\n"), 0o600); err != nil {
		t.Fatalf("precondition: create CLAUDE.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — warning with "instruction files exist, but none mention np" message.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil (pass)")
	}
	want := "instruction files exist, but none mention np"
	if !strings.Contains(result.Summary, want) {
		t.Errorf("summary %q does not contain %q", result.Summary, want)
	}
}

// TestRunAgentInstructions_AGENTSMDWithoutNpRef_ReturnsMentionWarning verifies
// that the check emits a finding when only AGENTS.md exists but has no np reference.
func TestRunAgentInstructions_AGENTSMDWithoutNpRef_ReturnsMentionWarning(t *testing.T) {
	t.Parallel()

	// Given — AGENTS.md with no np reference.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("some other content\n"), 0o600); err != nil {
		t.Fatalf("precondition: create AGENTS.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — warning about files existing but not mentioning np.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil (pass)")
	}
	want := "instruction files exist, but none mention np"
	if !strings.Contains(result.Summary, want) {
		t.Errorf("summary %q does not contain %q", result.Summary, want)
	}
}

// TestRunAgentInstructions_CLAUDEMDWithNpSpace_Passes verifies that the check
// passes when CLAUDE.md contains "np " (np followed by a space).
func TestRunAgentInstructions_CLAUDEMDWithNpSpace_Passes(t *testing.T) {
	t.Parallel()

	// Given — CLAUDE.md containing "np " with a trailing space.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Use np to track issues.\n"), 0o600); err != nil {
		t.Fatalf("precondition: create CLAUDE.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — passes (nil result).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunAgentInstructions_CLAUDEMDWithNpBacktick_Passes verifies that the check
// passes when CLAUDE.md contains backtick-wrapped `np`.
func TestRunAgentInstructions_CLAUDEMDWithNpBacktick_Passes(t *testing.T) {
	t.Parallel()

	// Given — CLAUDE.md containing "`np`".
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Run `np` to list issues.\n"), 0o600); err != nil {
		t.Fatalf("precondition: create CLAUDE.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — passes (nil result).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunAgentInstructions_AGENTSMDWithNpSpace_Passes verifies that AGENTS.md
// with "np " also satisfies the check.
func TestRunAgentInstructions_AGENTSMDWithNpSpace_Passes(t *testing.T) {
	t.Parallel()

	// Given — only AGENTS.md, containing "np ".
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Run np agent prime for setup.\n"), 0o600); err != nil {
		t.Fatalf("precondition: create AGENTS.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — passes (nil result).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// TestRunAgentInstructions_OneMentionsNp_Passes verifies that when two files
// exist but only one mentions np, the check still passes.
func TestRunAgentInstructions_OneMentionsNp_Passes(t *testing.T) {
	t.Parallel()

	// Given — CLAUDE.md without np reference, AGENTS.md with "np ".
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("no mention here\n"), 0o600); err != nil {
		t.Fatalf("precondition: create CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("np is great\n"), 0o600); err != nil {
		t.Fatalf("precondition: create AGENTS.md: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runAgentInstructions(t.Context(), svc, driving.DoctorInput{WorkDir: dir})
	// Then — passes because at least one file mentions np.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got finding: %q", result.Summary)
	}
}

// --- runGitIgnore ---

// TestRunGitIgnore_NotInGitRepo_IsNotApplicable verifies that the check is
// silently skipped (not applicable) when the workspace is not inside a git repo.
func TestRunGitIgnore_NotInGitRepo_IsNotApplicable(t *testing.T) {
	t.Parallel()

	// Given — a temp dir with .np/ but no .git anywhere in the ancestor chain.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db file: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — not applicable; silently omitted from all output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected not-applicable result, got nil (pass)")
	}
	if !result.NotApplicable {
		t.Errorf("expected NotApplicable=true, got summary: %q", result.Summary)
	}
}

// TestRunGitIgnore_NpInRootGitignore_Passes verifies that the check passes
// when the git root .gitignore contains ".np/".
func TestRunGitIgnore_NpInRootGitignore_Passes(t *testing.T) {
	t.Parallel()

	// Given — git root dir with .git/, .gitignore (".np/"), and .np/ subdirectory.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".np/\n"), 0o600); err != nil {
		t.Fatalf("precondition: create .gitignore: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — passes (nil result).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got: NotApplicable=%v, Summary=%q", result.NotApplicable, result.Summary)
	}
}

// TestRunGitIgnore_NpInSubdirGitignore_Passes verifies that the check passes
// when a .gitignore in a sub-directory (between .np/ parent and git root) has
// the matching entry.
func TestRunGitIgnore_NpInSubdirGitignore_Passes(t *testing.T) {
	t.Parallel()

	// Given — structure: root/.git/, root/sub/.gitignore (".np/"), root/sub/.np/
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatalf("precondition: create sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".gitignore"), []byte(".np/\n"), 0o600); err != nil {
		t.Fatalf("precondition: create sub/.gitignore: %v", err)
	}
	if err := os.Mkdir(filepath.Join(sub, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create sub/.np: %v", err)
	}
	dbPath := filepath.Join(sub, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: sub, DBPath: dbPath})
	// Then — passes because the sub-directory .gitignore has the entry.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got: NotApplicable=%v, Summary=%q", result.NotApplicable, result.Summary)
	}
}

// TestRunGitIgnore_NoMatchingGitignore_ReturnsWarning verifies that the check
// emits a warning when no .gitignore between .np/ parent and git root has a
// matching entry.
func TestRunGitIgnore_NoMatchingGitignore_ReturnsWarning(t *testing.T) {
	t.Parallel()

	// Given — git root with .git/ but .gitignore has no .np/ entry.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o600); err != nil {
		t.Fatalf("precondition: create .gitignore: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — warning; no matching gitignore entry.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil (pass)")
	}
	if result.NotApplicable {
		t.Error("expected a warning finding, got not-applicable")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary in finding")
	}
}

// TestRunGitIgnore_NpWithoutSlash_Passes verifies that ".np" (no trailing slash)
// is recognized as a valid ignore entry.
func TestRunGitIgnore_NpWithoutSlash_Passes(t *testing.T) {
	t.Parallel()

	// Given — .gitignore contains ".np" (no trailing slash).
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".np\n"), 0o600); err != nil {
		t.Fatalf("precondition: create .gitignore: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — passes.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil (pass), got: NotApplicable=%v, Summary=%q", result.NotApplicable, result.Summary)
	}
}

// TestRunGitIgnore_LeadingSlashForms_Passes verifies that "/.np" and "/.np/"
// are both recognized as valid ignore entries.
func TestRunGitIgnore_LeadingSlashForms_Passes(t *testing.T) {
	t.Parallel()

	for _, entry := range []string{"/.np", "/.np/"} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
				t.Fatalf("precondition: create .git: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(entry+"\n"), 0o600); err != nil {
				t.Fatalf("precondition: create .gitignore: %v", err)
			}
			if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
				t.Fatalf("precondition: create .np: %v", err)
			}
			dbPath := filepath.Join(dir, ".np", "nitpicking.db")
			if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
				t.Fatalf("precondition: create db: %v", err)
			}
			svc := newTestSvc()

			// When
			result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
			// Then — passes for both recognized forms.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != nil {
				t.Errorf("entry %q: expected nil (pass), got: NotApplicable=%v, Summary=%q",
					entry, result.NotApplicable, result.Summary)
			}
		})
	}
}

// TestRunGitIgnore_CommentedEntry_DoesNotPass verifies that a commented-out
// ".np/" line is not treated as a valid match.
func TestRunGitIgnore_CommentedEntry_DoesNotPass(t *testing.T) {
	t.Parallel()

	// Given — .gitignore with ".np/" as a comment (preceded by "#").
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# .np/\n"), 0o600); err != nil {
		t.Fatalf("precondition: create .gitignore: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — warning: commented entry does not count as ignored.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding (commented entry should not match), got nil (pass)")
	}
	if result.NotApplicable {
		t.Error("expected a warning finding, got not-applicable")
	}
}

// TestRunGitIgnore_NoGitignoreFileAtAll_ReturnsWarning verifies that the check
// warns when the git repo has no .gitignore files along the path.
func TestRunGitIgnore_NoGitignoreFileAtAll_ReturnsWarning(t *testing.T) {
	t.Parallel()

	// Given — git root with .git/ but no .gitignore anywhere.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o750); err != nil {
		t.Fatalf("precondition: create .git: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".np"), 0o750); err != nil {
		t.Fatalf("precondition: create .np: %v", err)
	}
	dbPath := filepath.Join(dir, ".np", "nitpicking.db")
	if err := os.WriteFile(dbPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("precondition: create db: %v", err)
	}
	svc := newTestSvc()

	// When
	result, err := runGitIgnore(t.Context(), svc, driving.DoctorInput{WorkDir: dir, DBPath: dbPath})
	// Then — warning; no matching entry found.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected a finding, got nil (pass)")
	}
	if result.NotApplicable {
		t.Error("expected a warning finding, got not-applicable")
	}
}

// --- notApplicable propagation in orchestrator ---

// TestRunDoctorChecks_GitIgnoreNotApplicable_OmittedFromAllOutput verifies
// that when git-ignore returns NotApplicable, it is absent from Passed,
// Warnings, Errors, and Skipped.
func TestRunDoctorChecks_GitIgnoreNotApplicable_OmittedFromAllOutput(t *testing.T) {
	t.Parallel()

	// Given — git-ignore is overridden to return NotApplicable; all other stubs pass.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: 1})
	registry := overrideRun(doctorRegistry(), "git-ignore",
		func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
			return &doctorRunResult{NotApplicable: true}, nil
		},
	)
	// Use a doctorTestInput so agent-instructions passes; git-ignore is overridden anyway.
	input := doctorTestInput(t)

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — git-ignore does not appear in any output list.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}

	agentInstructionsInPassed := false
	for _, p := range out.Passed {
		if p.Check == "git-ignore" {
			t.Error("git-ignore appeared in Passed but should be silently omitted")
		}
		if p.Check == "agent-instructions" {
			agentInstructionsInPassed = true
		}
	}
	if !agentInstructionsInPassed {
		t.Errorf("expected agent-instructions in Passed (sibling env check should still run), got passed=%v",
			passedSlugs(out.Passed))
	}
	for _, w := range out.Warnings {
		if w.Check == "git-ignore" {
			t.Error("git-ignore appeared in Warnings but should be silently omitted")
		}
	}
	for _, e := range out.Errors {
		if e.Check == "git-ignore" {
			t.Error("git-ignore appeared in Errors but should be silently omitted")
		}
	}
	for _, s := range out.Skipped {
		if s.Check == "git-ignore" {
			t.Error("git-ignore appeared in Skipped but should be silently omitted")
		}
	}
}

// TestRunDoctorChecks_EnvironmentRunsWhenDotNpDirectoryFails verifies that
// both environment checks still run (and their results appear) even when
// dot-np-directory fails.
func TestRunDoctorChecks_EnvironmentRunsWhenDotNpDirectoryFails(t *testing.T) {
	t.Parallel()

	// Given — dot-np-directory fails; environment checks are overridden to pass.
	svc := newTestSvc()
	registry := doctorRegistry()
	registry = overrideRun(registry, "dot-np-directory", findingStubRun("forced error"))
	// Override environment checks with pass stubs so the test is deterministic
	// regardless of the filesystem state under the temp dir.
	registry = overrideRun(registry, "agent-instructions", func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return nil, nil
	})
	registry = overrideRun(registry, "git-ignore", func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return nil, nil
	})

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, driving.DoctorInput{})
	// Then — both environment checks are in Passed.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}

	wantInPassed := []string{"agent-instructions", "git-ignore"}
	passedSet := make(map[string]bool, len(out.Passed))
	for _, p := range out.Passed {
		passedSet[p.Check] = true
	}
	for _, slug := range wantInPassed {
		if !passedSet[slug] {
			t.Errorf("expected %q in Passed, but it was not: passed=%v", slug, passedSlugs(out.Passed))
		}
	}
}

// TestRunDoctorChecks_EnvironmentRunsWhenColumnDataValidityFails verifies
// that environment checks still run when column-data-validity fails.
func TestRunDoctorChecks_EnvironmentRunsWhenColumnDataValidityFails(t *testing.T) {
	t.Parallel()

	// Given — column-data-validity fails; all DB prereqs pass; env checks pass.
	svc := newSvcWithDB(t, &checkFakeDBRepo{schemaVersion: 1})
	input := doctorTestInput(t)
	registry := doctorRegistry()
	registry = overrideRun(registry, "column-data-validity", findingStubRun("forced error"))
	registry = overrideRun(registry, "agent-instructions", func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return nil, nil
	})
	registry = overrideRun(registry, "git-ignore", func(_ context.Context, _ *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
		return nil, nil
	})

	// When
	out, err := runDoctorChecks(t.Context(), svc, registry, input)
	// Then — environment checks are in Passed, graph/lifecycle checks are skipped.
	if err != nil {
		t.Fatalf("runDoctorChecks: %v", err)
	}

	passedSet := make(map[string]bool, len(out.Passed))
	for _, p := range out.Passed {
		passedSet[p.Check] = true
	}
	for _, slug := range []string{"agent-instructions", "git-ignore"} {
		if !passedSet[slug] {
			t.Errorf("expected %q in Passed when column-data-validity fails, got passed=%v",
				slug, passedSlugs(out.Passed))
		}
	}
}
