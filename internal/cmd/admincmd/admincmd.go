// Package admincmd provides the "admin" parent command, which groups
// maintenance and administrative operations under a single namespace.
package admincmd

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/backupcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/completion"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/doctor"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/gc"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/restorecmd"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/tally"
	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/where"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

var validateDatabase = func(f *cmdutil.Factory) error {
	_, err := cmdutil.NewTracker(f)
	return err
}

// NewCmd constructs the "admin" parent command with maintenance subcommands.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "admin",
		Usage: "Administrative and maintenance commands",
		Description: `Groups maintenance and housekeeping operations that do not directly
create, update, or close issues. These commands manage the database
itself — backups, restores, resets, garbage collection, diagnostics,
and introspection.

Most admin subcommands require an existing .np/ database discovered by
walking up from the current directory. Use "admin where" to confirm which
database np has found, "admin doctor" to check its health, and "admin
backup" / "admin restore" for disaster recovery. "admin gc" reclaims
space from deleted issues, and "admin reset" is the nuclear option that
wipes all data after a two-step key verification.`,
		Commands: []*cli.Command{
			backupcmd.NewCmd(f),
			completion.NewCmd(f),
			doctor.NewCmd(f),
			gc.NewCmd(f),
			newResetCmd(f),
			restorecmd.NewCmd(f),
			tally.NewCmd(f),
			newUpgradeCmd(f),
			where.NewCmd(f),
		},
	}
}

// resetKeyHashFile is the name of the file that stores the SHA-512 hash of the
// active reset key. Stored in the .np/ directory alongside the database.
const resetKeyHashFile = "reset-key-hash.txt"

// resetInitiateOutput is the JSON representation of a reset key generation.
type resetInitiateOutput struct {
	Action     string `json:"action"`
	ResetKey   string `json:"reset_key"`
	Warning    string `json:"warning"`
	IssueCount int    `json:"issue_count"`
}

// resetExecuteOutput is the JSON representation of a successful reset.
type resetExecuteOutput struct {
	Action     string `json:"action"`
	BackupPath string `json:"backup_path"`
}

// newResetCmd constructs "admin reset" which implements a two-step reset-key
// flow. Without --reset-key, it generates a key and stores its hash. With
// --reset-key, it verifies the key, creates a backup, and clears all data.
func newResetCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		resetKeyIn string
	)

	return &cli.Command{
		Name:  "reset",
		Usage: "Reset the database (two-step key verification)",
		Description: `Permanently destroys all issues, comments, relationships, and history in
the database. This is a two-step process to prevent accidental data loss.
Run "admin reset" once to generate a one-time reset key; run it again
with --reset-key to execute the reset. A gzip-compressed JSONL backup is
automatically created before the data is deleted.

Use this when the database is corrupt beyond repair, when you want to
start fresh with a clean project, or during development when you need to
wipe test data. Because the operation is irreversible (except via restore
from backup), the two-step key flow ensures that neither a typo nor an
automated script can trigger it accidentally.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "reset-key",
				Usage:       "Reset key from step 1 (executes the reset when provided)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &resetKeyIn,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dbPath, err := f.DatabasePath()
			if err != nil {
				return fmt.Errorf("no database found: %w", err)
			}
			npDir := filepath.Dir(dbPath)

			store, err := f.Store()
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			svc := core.New(store)

			if resetKeyIn == "" {
				return resetInitiate(ctx, f, svc, npDir, jsonOutput)
			}
			return resetExecute(ctx, f, svc, store, npDir, resetKeyIn, jsonOutput)
		},
	}
}

// resetInitiate generates a reset key, stores its hash, and displays a warning.
func resetInitiate(ctx context.Context, f *cmdutil.Factory, svc driving.Service, npDir string, jsonOutput bool) error {
	issueCount, err := svc.CountAllIssues(ctx)
	if err != nil {
		return fmt.Errorf("counting issues: %w", err)
	}

	key := domain.ResetKeyGenerate()
	hashHex, err := domain.ResetKeyHash(key)
	if err != nil {
		return fmt.Errorf("hashing reset key: %w", err)
	}

	hashPath := filepath.Join(npDir, resetKeyHashFile)
	if err := os.WriteFile(hashPath, []byte(hashHex), 0o600); err != nil {
		return fmt.Errorf("writing reset key hash: %w", err)
	}

	warning := fmt.Sprintf(
		"Resetting the issue database is a destructive action that will permanently remove all %d of your issues and their history.",
		issueCount,
	)

	if jsonOutput {
		return cmdutil.WriteJSON(f.IOStreams.Out, resetInitiateOutput{
			Action:     "reset_key_generated",
			ResetKey:   key,
			Warning:    warning,
			IssueCount: issueCount,
		})
	}

	cs := f.IOStreams.ColorScheme()
	_, _ = fmt.Fprintf(f.IOStreams.Out, "%s\n\nReset key: %s\n\nRun %s to execute the reset.\n",
		cs.Bold(warning),
		cs.Cyan(key),
		cs.Cyan("np admin reset --reset-key "+key),
	)
	return nil
}

// resetExecute verifies the reset key, creates a backup, clears all data,
// vacuums, and removes the hash file.
func resetExecute(ctx context.Context, f *cmdutil.Factory, svc driving.Service, store *sqlite.Store, npDir, key string, jsonOutput bool) error {
	// Step 1: Read and verify the reset key hash.
	hashPath := filepath.Join(npDir, resetKeyHashFile)
	storedHash, err := os.ReadFile(hashPath) // #nosec G304 -- path constructed from discovered .np/ directory
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no reset key has been generated — run %q first", "np admin reset")
		}
		return fmt.Errorf("reading reset key hash: %w", err)
	}

	providedHash, err := domain.ResetKeyHash(key)
	if err != nil {
		return fmt.Errorf("invalid reset key: %w", err)
	}

	if providedHash != string(storedHash) {
		return fmt.Errorf("reset key does not match — generate a new key with %q", "np admin reset")
	}

	// Step 2: Create a backup.
	backupPath, err := createBackup(ctx, svc, npDir)
	if err != nil {
		return fmt.Errorf("creating pre-reset backup: %w", err)
	}

	// Step 3: Clear all data and vacuum.
	if err := svc.ResetDatabase(ctx); err != nil {
		return fmt.Errorf("resetting database: %w", err)
	}

	// Step 4: Remove the hash file.
	_ = os.Remove(hashPath) // Best-effort: the reset succeeded, so the key is consumed.

	if jsonOutput {
		return cmdutil.WriteJSON(f.IOStreams.Out, resetExecuteOutput{
			Action:     "reset",
			BackupPath: backupPath,
		})
	}

	cs := f.IOStreams.ColorScheme()
	_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Database reset. Backup saved to %s\n",
		cs.SuccessIcon(), backupPath)
	return nil
}

// createBackup writes a JSONL backup to the .np/ directory and returns the
// file path. Delegates filename generation to backupcmd.DefaultBackupFilename
// to keep the format consistent across all backup commands.
func createBackup(ctx context.Context, svc driving.Service, npDir string) (string, error) {
	prefix, err := svc.GetPrefix(ctx)
	if err != nil {
		return "", fmt.Errorf("reading database prefix: %w", err)
	}

	backupPath := filepath.Join(npDir, backupcmd.DefaultBackupFilename(prefix))

	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- path constructed from discovered .np/ directory
	if err != nil {
		return "", fmt.Errorf("creating backup file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	gzw := gzip.NewWriter(file)
	writer := jsonl.NewWriter(gzw)

	_, err = svc.Backup(ctx, driving.BackupInput{Writer: writer})
	if err != nil {
		_ = writer.Close()
		_ = os.Remove(backupPath)
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("closing backup: %w", err)
	}

	return backupPath, nil
}

// newUpgradeCmd constructs "admin upgrade" which checks for and applies
// database schema upgrades. Currently the schema has no versioning, so
// this always reports the database is up to date.
func newUpgradeCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:  "upgrade",
		Usage: "Check for and apply database schema upgrades",
		Description: `Checks whether the database schema is current and applies any pending
upgrades. Currently the schema has no versioning, so this command always
reports the database as up to date. It exists as a placeholder for future
schema evolution — when new columns, tables, or indexes are introduced,
this command will apply them non-destructively.

Run this after updating the np binary to a new version to ensure the
database schema matches the expectations of the new code.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Verify the database exists and is accessible.
			if err := validateDatabase(f); err != nil {
				return err
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, map[string]string{
					"status": "up_to_date",
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, err := fmt.Fprintf(f.IOStreams.Out, "%s Database is up to date\n",
				cs.SuccessIcon())
			return err
		},
	}
}
