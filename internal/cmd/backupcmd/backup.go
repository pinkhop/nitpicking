// Package backupcmd provides the "admin backup" command, which creates
// a JSONL backup of the entire issue database.
package backupcmd

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// backupOutput is the JSON representation of the backup command result.
type backupOutput struct {
	Path       string `json:"path"`
	IssueCount int    `json:"issue_count"`
}

// RunInput holds the parameters for the backup command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	// DiscoverFunc locates the database file, returning its absolute path.
	// In production this is Factory.DatabasePath (workspace-aware); tests
	// provide a stub. Only called when Output is empty — the discovered
	// .np/ directory determines the default backup location.
	DiscoverFunc func() (string, error)

	// BackupFunc performs the actual backup, writing issue data to the
	// provided writer and returning the number of issues written. In
	// production this wraps svc.Backup; tests provide a stub. The
	// writer is a gzip.Writer that the caller manages — BackupFunc
	// should not close it.
	BackupFunc func(w io.WriteCloser) (int, error)

	// Output is the user-specified destination path. When empty, the
	// backup file is written to the discovered .np/ directory with a
	// timestamp-based filename.
	Output string

	// JSON enables machine-readable JSON output.
	JSON bool

	// WriteTo receives the command's human-readable or JSON output.
	WriteTo io.Writer

	// Prefix is the database's issue ID prefix (e.g., "NP"). When
	// non-empty and the default filename is used (Output is empty or
	// Output is a directory), the prefix is included in the filename:
	// backup-<prefix>.<timestamp>.jsonl.gz.
	Prefix string

	// SuccessIcon returns a colored or plain success indicator for
	// human-readable output. When nil, a default "[ok]" is used.
	SuccessIcon func() string
}

// Run executes the backup workflow: determines the output path, creates
// a gzip-compressed JSONL file, invokes the backup function, and prints
// the result.
func Run(_ context.Context, input RunInput) error {
	backupPath, err := resolveBackupPath(input)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- path is either from the discovered .np/ directory or explicitly provided by the user via --output
	if err != nil {
		return fmt.Errorf("creating backup file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	gzw := gzip.NewWriter(file)

	issueCount, err := input.BackupFunc(gzw)
	if err != nil {
		// Best-effort cleanup of partial backup file.
		_ = gzw.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("creating backup: %w", err)
	}

	// Close the gzip writer to flush the compressed stream. The deferred
	// file.Close() handles the underlying file.
	if err := gzw.Close(); err != nil {
		return fmt.Errorf("closing backup: %w", err)
	}

	if input.JSON {
		return cmdutil.WriteJSON(input.WriteTo, backupOutput{
			Path:       backupPath,
			IssueCount: issueCount,
		})
	}

	icon := "[ok]"
	if input.SuccessIcon != nil {
		icon = input.SuccessIcon()
	}
	_, _ = fmt.Fprintf(input.WriteTo, "%s Backup created: %s (%d issues)\n", // #nosec G705 -- output is a terminal stream, not HTML
		icon, backupPath, issueCount)

	return nil
}

// resolveBackupPath returns the output file path. When Output is empty, a
// timestamped filename is placed in the discovered .np/ directory. When
// Output is a directory, the default filename is placed inside it. When
// Output is a file path, it is used as-is.
func resolveBackupPath(input RunInput) (string, error) {
	filename := DefaultBackupFilename(input.Prefix)

	if input.Output != "" {
		info, err := os.Stat(input.Output)
		if err == nil && info.IsDir() {
			return filepath.Join(input.Output, filename), nil
		}
		return input.Output, nil
	}

	dbPath, err := input.DiscoverFunc()
	if err != nil {
		return "", fmt.Errorf("no database found: %w", err)
	}
	npDir := filepath.Dir(dbPath)

	return filepath.Join(npDir, filename), nil
}

// DefaultBackupFilename returns a timestamped backup filename, optionally
// including the database prefix. The prefix is sanitized to contain only
// ASCII letters, preventing path traversal from a corrupt or malicious
// database prefix.
func DefaultBackupFilename(prefix string) string {
	safe := sanitizePrefix(prefix)
	if safe != "" {
		return fmt.Sprintf("backup-%s.%d.jsonl.gz", strings.ToLower(safe), time.Now().Unix())
	}
	return fmt.Sprintf("backup.%d.jsonl.gz", time.Now().Unix())
}

// sanitizePrefix strips any character that is not an ASCII letter from the
// prefix. A well-formed prefix should be all uppercase ASCII letters (see
// domain.ValidatePrefix), but the database value may come from a restored
// backup and is not guaranteed to be clean.
func sanitizePrefix(prefix string) string {
	var b strings.Builder
	for _, r := range prefix {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NewCmd constructs the "admin backup" command, which writes a JSONL
// backup file into the .np/ directory or a user-specified path.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		output     string
	)

	return &cli.Command{
		Name:  "backup",
		Usage: "Create a JSONL backup of the issue database",
		Description: `Creates a gzip-compressed JSONL snapshot of every issue, comment,
relationship, and label in the database. The backup file is written to
the .np/ directory by default (with a Unix-timestamp filename) or to a
path you specify with --output. If --output is a directory, the default
filename is used inside that directory.

Use this before any destructive operation — resets, restores, schema
experiments — or as a periodic safety net. The backup format is the same
JSONL that "import jsonl" and "admin restore" consume, so a backup can
be round-tripped back into a fresh database. AI agents should run this
before operations they cannot undo.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "Destination file or directory for the backup (default: .np/backup-<prefix>.<timestamp>.jsonl.gz)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &output,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			store, err := f.Store()
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}

			svc := core.New(store)
			cs := f.IOStreams.ColorScheme()

			prefix, err := svc.GetPrefix(ctx)
			if err != nil {
				return fmt.Errorf("reading database prefix: %w", err)
			}

			return Run(ctx, RunInput{
				DiscoverFunc: f.DatabasePath,
				Prefix:       prefix,
				BackupFunc: func(w io.WriteCloser) (int, error) {
					writer := jsonl.NewWriter(w)
					result, err := svc.Backup(ctx, driving.BackupInput{Writer: writer})
					if err != nil {
						_ = writer.Close()
						return 0, err
					}
					if err := writer.Close(); err != nil {
						return 0, err
					}
					return result.IssueCount, nil
				},
				Output:      output,
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				SuccessIcon: cs.SuccessIcon,
			})
		},
	}
}
