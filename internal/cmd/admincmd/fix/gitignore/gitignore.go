// Package gitignore provides the "admin fix git-ignore" subcommand, which
// appends ".np/" to the project's .gitignore so the issue database is not
// accidentally committed to git.
package gitignore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
)

// ErrNotInGit is returned (as a wrapped sentinel) when the walk reaches the
// filesystem root without encountering a .git marker. Callers can use
// errors.Is(err, gitignore.ErrNotInGit) to branch on this condition without
// string-matching the error message.
var ErrNotInGit = errors.New("not inside a git repository")

// RunInput holds the parameters for the git-ignore fix's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	// StartDir is D₀: the directory that contains the .np/ directory. The walk
	// begins here, looking for a .gitignore to amend or a .git marker indicating
	// where to create one.
	StartDir string

	// DryRun, when true, previews what would change without making filesystem
	// mutations. The exit code is always 0 for a successful dry-run.
	DryRun bool

	// JSON enables machine-readable JSON output instead of human-readable text.
	JSON bool

	// Out receives all command output.
	Out io.Writer
}

// addedOutput is the JSON representation of a successful fix (entry was added).
type addedOutput struct {
	// Added is always true in this struct — the entry was written to the file.
	Added bool `json:"added"`
	// Created is true when the .gitignore was newly created; false when amended.
	Created bool `json:"created"`
	// Path is the absolute path to the .gitignore that was modified or created.
	Path string `json:"path"`
}

// noopOutput is the JSON representation of a no-op fix (entry already present).
type noopOutput struct {
	// Added is always false in this struct — no write occurred.
	Added bool `json:"added"`
	// Reason explains why nothing was done; currently always "already_ignored".
	Reason string `json:"reason"`
	// Path is the absolute path to the .gitignore that already contains the entry.
	Path string `json:"path"`
}

// dryRunOutput is the JSON representation of a dry-run preview.
type dryRunOutput struct {
	// WouldAdd is always true in this struct — the entry would be written.
	WouldAdd bool `json:"would_add"`
	// WouldCreate is true when the .gitignore would be newly created; false when
	// it would be amended.
	WouldCreate bool `json:"would_create"`
	// Path is the absolute path to the .gitignore that would be modified or created.
	Path string `json:"path"`
}

// Run executes the git-ignore fix: walks up from input.StartDir to locate the
// target .gitignore (or determine it needs to be created at the repository
// root), then amends or creates the file. Returns a wrapped ErrNotInGit error
// when the workspace is not inside a git repository, which produces exit code 1
// via the standard error classifier.
func Run(_ context.Context, input RunInput) error {
	target, createNew, err := findTarget(input.StartDir)
	if err != nil {
		if errors.Is(err, ErrNotInGit) {
			return fmt.Errorf(".gitignore management is irrelevant outside git: %w", err)
		}
		return fmt.Errorf("locating .gitignore: %w", err)
	}

	if createNew {
		if input.DryRun {
			return emitDryRun(input.Out, target, true, input.JSON)
		}
		if err := os.WriteFile(target, []byte(".np/\n"), 0o644); err != nil { // #nosec G304 G306 -- target is derived from the discovered .np/ directory; 0o644 is standard for a world-readable git-committed file
			return fmt.Errorf("creating %s: %w", target, err)
		}
		return emitCreated(input.Out, target, input.JSON)
	}

	// Amending an existing .gitignore.
	content, err := os.ReadFile(target) // #nosec G304 -- target is derived by walking up from the discovered .np/ directory
	if err != nil {
		return fmt.Errorf("reading %s: %w", target, err)
	}

	if alreadyIgnored(string(content)) {
		return emitAlreadyIgnored(input.Out, target, input.JSON)
	}

	if input.DryRun {
		return emitDryRun(input.Out, target, false, input.JSON)
	}

	newContent := appendEntry(content)
	if err := os.WriteFile(target, newContent, 0o644); err != nil { // #nosec G304 G306 -- target is derived from the discovered .np/ directory; 0o644 is standard for a world-readable git-committed file
		return fmt.Errorf("writing %s: %w", target, err)
	}
	return emitAdded(input.Out, target, input.JSON)
}

// findTarget walks upward from startDir, recording the deepest .gitignore seen
// along the way, until it finds a .git marker (proving the walk is inside a
// git repository) or reaches the filesystem root (in which case it returns
// ErrNotInGit). Only after the .git marker is found is a target returned —
// either the deepest .gitignore between startDir and the repo root (amend) or
// the path where one should be created at the repo root. The .git check
// treats both files and directories identically, which correctly handles git
// worktrees and submodules.
//
// This ordering matters: returning a .gitignore before proving the walk is
// inside a git repository would let the command silently modify an unrelated
// .gitignore living above a non-git workspace (e.g. a stray ~/.gitignore).
func findTarget(startDir string) (path string, createNew bool, err error) {
	var nearestGitignore string
	dir := startDir
	for {
		if nearestGitignore == "" {
			if _, statErr := os.Stat(filepath.Join(dir, ".gitignore")); statErr == nil {
				nearestGitignore = filepath.Join(dir, ".gitignore")
			}
		}
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
			if nearestGitignore != "" {
				return nearestGitignore, false, nil
			}
			return filepath.Join(dir, ".gitignore"), true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the filesystem root without finding .git.
			return "", false, ErrNotInGit
		}
		dir = parent
	}
}

// alreadyIgnored reports whether content contains a .np/ ignore entry in any
// of the four recognized forms: .np, .np/, /.np, or /.np/. Lines beginning
// with "#" and trailing whitespace are ignored when matching.
func alreadyIgnored(content string) bool {
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

// appendEntry returns content with ".np/" appended. When content is non-empty
// and does not already end with a blank line, a blank-line separator is
// inserted before the new entry to keep it visually distinct from preceding
// rules.
func appendEntry(content []byte) []byte {
	const entry = ".np/\n"
	if len(content) == 0 {
		return []byte(entry)
	}
	switch {
	case len(content) >= 2 && content[len(content)-2] == '\n' && content[len(content)-1] == '\n':
		// Already ends with a blank line; append the entry directly.
		return append(content, []byte(entry)...)
	case content[len(content)-1] == '\n':
		// Ends with a single newline; add one more to create the blank line.
		return append(content, []byte("\n"+entry)...)
	default:
		// Does not end with a newline; add two newlines to create the blank line.
		return append(content, []byte("\n\n"+entry)...)
	}
}

// emitAdded writes the "Added" output (text or JSON) for a successful amendment.
func emitAdded(w io.Writer, path string, jsonOutput bool) error {
	if jsonOutput {
		return cmdutil.WriteJSON(w, addedOutput{Added: true, Created: false, Path: path})
	}
	_, err := fmt.Fprintf(w, "Added '.np/' to %s.\n", path)
	return err
}

// emitCreated writes the "Created" output (text or JSON) for a newly created
// .gitignore file.
func emitCreated(w io.Writer, path string, jsonOutput bool) error {
	if jsonOutput {
		return cmdutil.WriteJSON(w, addedOutput{Added: true, Created: true, Path: path})
	}
	_, err := fmt.Fprintf(w, "Created %s with '.np/'.\n", path)
	return err
}

// emitAlreadyIgnored writes the no-op output (text or JSON) when .np/ is
// already present in the target .gitignore.
func emitAlreadyIgnored(w io.Writer, path string, jsonOutput bool) error {
	if jsonOutput {
		return cmdutil.WriteJSON(w, noopOutput{Added: false, Reason: "already_ignored", Path: path})
	}
	_, err := fmt.Fprintf(w, "'.np/' is already ignored by %s. No changes made.\n", path)
	return err
}

// emitDryRun writes the dry-run preview output (text or JSON) for a would-be
// change. wouldCreate is true when the fix would create a new .gitignore.
func emitDryRun(w io.Writer, path string, wouldCreate bool, jsonOutput bool) error {
	if jsonOutput {
		return cmdutil.WriteJSON(w, dryRunOutput{WouldAdd: true, WouldCreate: wouldCreate, Path: path})
	}
	if wouldCreate {
		_, err := fmt.Fprintf(w, "Would create %s with '.np/'.\nRe-run without --dry-run to apply.\n", path)
		return err
	}
	_, err := fmt.Fprintf(w, "Would add '.np/' to %s.\nRe-run without --dry-run to apply.\n", path)
	return err
}

// NewCmd constructs the "admin fix git-ignore" subcommand, which appends ".np/"
// to the project's .gitignore so the issue database is not accidentally committed
// to git. An optional runFn replaces Run for testing; when provided, the Factory
// is used only to resolve the .np/ directory path for the StartDir field.
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, RunInput) error) *cli.Command {
	var (
		dryRun     bool
		jsonOutput bool
	)

	return &cli.Command{
		Name:  "git-ignore",
		Usage: "Add .np/ to the project's .gitignore",
		Description: `Appends ".np/" to the project's .gitignore file so that the issue
database is not accidentally committed to git.

The fix walks upward from the directory containing .np/ to find the
nearest .gitignore file. If no .gitignore exists between the .np/
directory and the repository root, it creates one at the repository root
(the directory that contains .git). The operation is idempotent —
re-running after a successful add reports no changes needed.

Use --dry-run to preview what would change without modifying any files.
Use --json for machine-readable output.

Exit codes: 0 success or no-op; 1 not inside a git repository.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "dry-run",
				Usage:       "Preview what would change without modifying the filesystem",
				Destination: &dryRun,
				Category:    cmdutil.FlagCategorySupplemental,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Destination: &jsonOutput,
				Category:    cmdutil.FlagCategorySupplemental,
			},
		},
		Action: func(ctx context.Context, _ *cli.Command) error {
			dbPath, err := f.DatabasePath()
			if err != nil {
				return fmt.Errorf("no database found: %w", err)
			}
			// D₀ is the directory that contains the .np/ directory, not .np/ itself.
			// DatabasePath returns the DB file path (e.g. /foo/.np/np.db), so
			// filepath.Dir twice gives us /foo/.
			npDir := filepath.Dir(dbPath)
			startDir := filepath.Dir(npDir)

			input := RunInput{
				StartDir: startDir,
				DryRun:   dryRun,
				JSON:     jsonOutput,
				Out:      f.IOStreams.Out,
			}
			if len(runFn) > 0 {
				return runFn[0](ctx, input)
			}
			return Run(ctx, input)
		},
	}
}
