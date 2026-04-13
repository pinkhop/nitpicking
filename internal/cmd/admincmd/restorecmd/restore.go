// Package restorecmd provides the "admin restore" command, which
// restores the issue database from a JSONL backup file.
package restorecmd

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// confirmPhrase is the exact text the user must type to approve a
// restore. Intentionally verbose to prevent accidental or automated
// execution.
const confirmPhrase = "delete existing issues and restore"

// restoreOutput is the JSON representation of the restore result.
type restoreOutput struct {
	Action string `json:"action"`
	Source string `json:"source"`
}

// NewCmd constructs the "admin restore" command, which replaces the
// entire database with the contents of a JSONL backup file. This is
// a dangerously destructive operation that requires interactive
// confirmation.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var jsonOutput bool

	return &cli.Command{
		Name:      "restore",
		Usage:     "Restore the database from a JSONL backup file (destructive)",
		ArgsUsage: "<backup-file>",
		Description: `Replaces the entire contents of the database with the issues, comments,
relationships, and labels from a gzip-compressed JSONL backup file. All
existing data is deleted before the backup is loaded — this is a full
replacement, not a merge.

Because the operation is destructive, it requires interactive
confirmation: the user must type a specific phrase before the restore
proceeds. This is intentionally designed to block automated agents from
running restores without human oversight. Use this to recover from
accidental resets, to move a database between machines, or to roll back
to a known-good state captured by "admin backup".`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() < 1 {
				return cmdutil.FlagErrorf("backup file path is required")
			}
			backupPath := cmd.Args().First()

			// Verify the file exists before prompting.
			if _, err := os.Stat(backupPath); err != nil {
				return fmt.Errorf("backup file not found: %w", err)
			}

			// Interactive confirmation — intentionally designed to block
			// automated agents from running this command.
			_, _ = fmt.Fprintf(f.IOStreams.ErrOut,
				"WARNING: This will DELETE all existing issues and replace them with the backup.\n"+
					"Type %q to proceed: ", confirmPhrase)

			scanner := bufio.NewScanner(f.IOStreams.In)
			if !scanner.Scan() {
				return fmt.Errorf("restore aborted: no input received")
			}
			input := strings.TrimSpace(scanner.Text())
			if input != confirmPhrase {
				return fmt.Errorf("restore aborted: confirmation phrase did not match")
			}

			// Open the backup file and decompress the gzip stream.
			file, err := os.Open(backupPath) // #nosec G304 -- path is provided as a CLI argument; the user explicitly chooses which file to restore from
			if err != nil {
				return fmt.Errorf("opening backup file: %w", err)
			}
			defer func() {
				_ = file.Close()
			}()

			gzr, err := gzip.NewReader(file)
			if err != nil {
				return fmt.Errorf("decompressing backup file: %w", err)
			}
			reader := jsonl.NewReader(gzr)
			defer func() {
				_ = reader.Close()
			}()

			// Perform the restore.
			store, err := f.Store()
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			svc := core.New(store, store)
			if err := svc.Restore(ctx, driving.RestoreInput{Reader: reader}); err != nil {
				return fmt.Errorf("restoring database: %w", err)
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, restoreOutput{
					Action: "restored",
					Source: backupPath,
				})
			}

			cs := f.IOStreams.ColorScheme()
			_, _ = fmt.Fprintf(f.IOStreams.Out, "%s Database restored from %s\n",
				cs.SuccessIcon(), backupPath)

			return nil
		},
	}
}
