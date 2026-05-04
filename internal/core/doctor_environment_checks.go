package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// runAgentInstructions checks that an agent instruction file (CLAUDE.md or
// AGENTS.md) exists in input.WorkDir and references np. Two failure modes are
// distinguished:
//
//  1. No instruction file exists at all.
//  2. At least one file exists but none contain an np reference.
//
// A reference is either "np " (with trailing space) or "`np`" (backtick-wrapped).
func runAgentInstructions(_ context.Context, _ *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error) {
	workDir := input.WorkDir
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return &doctorRunResult{
				Summary: fmt.Sprintf("could not determine working directory: %v", err),
			}, nil
		}
	}

	claudeMDPath := filepath.Join(workDir, "CLAUDE.md")
	agentsMDPath := filepath.Join(workDir, "AGENTS.md")

	claudeExists := fileExists(claudeMDPath)
	agentsExists := fileExists(agentsMDPath)

	if !claudeExists && !agentsExists {
		return &doctorRunResult{
			Summary: "no instruction files exist — create CLAUDE.md or AGENTS.md with a reference to np",
		}, nil
	}

	if (claudeExists && fileContainsNpRef(claudeMDPath)) ||
		(agentsExists && fileContainsNpRef(agentsMDPath)) {
		return nil, nil
	}

	return &doctorRunResult{
		Summary: "instruction files exist, but none mention np — add a reference to np in CLAUDE.md or AGENTS.md",
	}, nil
}

// runGitIgnore checks that the .np/ directory is listed in a .gitignore file
// somewhere between the .np/ parent directory and the git repository root.
// When the workspace is not inside a git repository the check returns
// NotApplicable, causing the orchestrator to omit it from all output lists.
func runGitIgnore(_ context.Context, _ *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error) {
	startDir, err := npParentDirectory(input)
	if err != nil {
		// Cannot determine where .np/ lives — treat as not applicable rather than
		// returning an error that would surface as a doctor failure.
		return &doctorRunResult{NotApplicable: true}, nil
	}

	found, err := npIgnoredByAnyGitignore(startDir)
	if err != nil {
		if errors.Is(err, errNotInGitRepository) {
			return &doctorRunResult{NotApplicable: true}, nil
		}
		return nil, fmt.Errorf("git-ignore check: %w", err)
	}

	if found {
		return nil, nil
	}

	return &doctorRunResult{
		Summary: "The .np/ directory is not ignored by git — it may be accidentally committed to the repository",
	}, nil
}

// errNotInGitRepository is returned by npIgnoredByAnyGitignore when the walk
// reaches the filesystem root without finding a .git marker.
var errNotInGitRepository = errors.New("not inside a git repository")

// npParentDirectory returns the directory that contains the .np/ directory,
// used as the starting point for the git-ignore walk. It prefers input.DBPath
// (the database file path: parent-of-parent gives the .np/ container), then
// input.WorkDir, and finally falls back to os.Getwd().
func npParentDirectory(input driving.DoctorInput) (string, error) {
	if input.DBPath != "" {
		// DBPath is like /path/.np/nitpicking.db; Dir twice gives /path.
		return filepath.Dir(filepath.Dir(input.DBPath)), nil
	}
	if input.WorkDir != "" {
		return input.WorkDir, nil
	}
	return os.Getwd()
}

// npIgnoredByAnyGitignore walks upward from startDir, inspecting each
// .gitignore file it encounters. It stops when it finds a .git marker
// (the repository root) and includes that directory's .gitignore in the
// search. Returns (true, nil) when any matching entry is found,
// (false, nil) when the git root is reached with no match, and
// (false, errNotInGitRepository) when the filesystem root is reached
// without a .git marker.
//
// The walk logic mirrors findTarget in internal/cmd/admincmd/fix/gitignore.
// That function cannot be shared because core must not import cmd packages.
// If the recognised-form set (.np, .np/, /.np, /.np/) ever changes, both
// implementations must be updated together.
func npIgnoredByAnyGitignore(startDir string) (bool, error) {
	dir := startDir
	for {
		// Inspect the .gitignore at this level before checking for .git so that
		// the repository root's own .gitignore is always included in the walk.
		if content, readErr := os.ReadFile(filepath.Join(dir, ".gitignore")); readErr == nil { // #nosec G304 -- path is constructed by walking up from the .np/ directory, not from user input
			if npGitignoreEntryPresent(string(content)) {
				return true, nil
			}
		}
		// A .git file (worktree) or directory marks the repository root.
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
			return false, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding .git.
			return false, errNotInGitRepository
		}
		dir = parent
	}
}

// npGitignoreEntryPresent reports whether content contains a .np/ ignore entry
// in any of the four recognised forms: .np, .np/, /.np, or /.np/. Comment
// lines (starting with "#") and trailing whitespace are ignored.
//
// This mirrors alreadyIgnored in internal/cmd/admincmd/fix/gitignore, which
// cannot be shared because core must not import cmd packages.
func npGitignoreEntryPresent(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		switch trimmed {
		case ".np", ".np/", "/.np", "/.np/":
			return true
		}
	}
	return false
}

// fileExists reports whether a file or directory exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileContainsNpRef reports whether the file at path contains "np " (with a
// trailing space) or "`np`" (backtick-wrapped). The match is a simple
// substring search — any occurrence anywhere in the file counts, including
// inside code blocks, inline examples, or prose. This is intentional: the
// check's purpose is to detect whether the instruction file acknowledges np
// at all, not to verify semantic placement. Read errors are treated as "no
// reference found" rather than surfaced as check errors.
func fileContainsNpRef(path string) bool {
	content, err := os.ReadFile(path) // #nosec G304 -- path is derived from WorkDir (the configured working directory), not from untrusted input
	if err != nil {
		// Best-effort read; if the file cannot be read the reference is absent.
		return false
	}
	s := string(content)
	return strings.Contains(s, "np ") || strings.Contains(s, "`np`")
}
