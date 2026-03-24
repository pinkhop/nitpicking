package welcome

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// NewCmd constructs the "welcome" command, which displays the full setup guide
// with live status indicators. This command is read-only — it inspects the
// current project state but does not modify any files or create the database.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	return &cli.Command{
		Name:  "welcome",
		Usage: "Show the setup guide with live status indicators",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}

			cs := f.IOStreams.ColorScheme()
			w := f.IOStreams.Out

			// Detect step completion.
			dbInit := IsDatabaseInitialized(cwd)
			gitIgnored := IsGitIgnored(cwd)
			hasInstructions := HasAgentInstructions(cwd)

			// Resolve the prefix for the status line if database is initialized.
			var prefix string
			if dbInit {
				svc, svcErr := cmdutil.NewTracker(f)
				if svcErr == nil {
					p, prefixErr := svc.GetPrefix(ctx)
					if prefixErr == nil {
						prefix = p
					}
				}
			}

			// Header.
			_, _ = fmt.Fprintf(w, "%s\n\n", cs.Bold("np — local-only issue tracker for AI agent workflows"))

			// Setup checklist.
			_, _ = fmt.Fprintf(w, "%s\n\n", cs.Bold("Setup"))
			_, _ = fmt.Fprintf(w, "  - %s Set up issue tracking for this project   np init <PREFIX>\n", checkbox(cs, dbInit))
			_, _ = fmt.Fprintf(w, "  - %s Exclude .np/ from version control              add .np/ to .gitignore\n", checkbox(cs, gitIgnored))
			_, _ = fmt.Fprintf(w, "  - %s Tell your AI agent how to use np               paste np agent prime output\n", checkbox(cs, hasInstructions))
			// Author step is always unchecked — it's informational only.
			_, _ = fmt.Fprintln(w, "  - [ ] Choose an author name for yourself              np agent name or pick your own")

			// Step details.
			_, _ = fmt.Fprintf(w, "\n%s\n\n", cs.Bold("Set up issue tracking for this project"))
			_, _ = fmt.Fprintln(w, "  Issue IDs use a project prefix (e.g., prefix \"NP\" produces NP-a3bxr).")
			_, _ = fmt.Fprintln(w, "  Choose something short and project-specific — convention is 2–4 uppercase letters.")
			_, _ = fmt.Fprintf(w, "\n    %s\n", cs.Cyan("np init <PREFIX>"))
			if dbInit && prefix != "" {
				_, _ = fmt.Fprintf(w, "\n  %s\n", cs.Dim("Current: initialized with prefix "+prefix))
			}

			_, _ = fmt.Fprintf(w, "\n%s\n\n", cs.Bold("Add .np/ to .gitignore"))
			_, _ = fmt.Fprintln(w, "  np stores its database locally in .np/ — you probably don't want to commit it.")
			_, _ = fmt.Fprintln(w, "  Add this line to your .gitignore:")
			_, _ = fmt.Fprintf(w, "\n    %s\n", cs.Cyan(".np/"))
			_, _ = fmt.Fprintln(w, "\n  Or run:")
			_, _ = fmt.Fprintf(w, "    %s\n", cs.Cyan("echo '.np/' >> .gitignore"))

			_, _ = fmt.Fprintf(w, "\n%s\n\n", cs.Bold("Add agent instructions to your project"))
			_, _ = fmt.Fprintln(w, "  np works best when your AI agent knows it exists. Run:")
			_, _ = fmt.Fprintf(w, "\n    %s\n", cs.Cyan("np agent prime"))
			_, _ = fmt.Fprintln(w, "\n  This prints Markdown workflow instructions. Paste the output into your agent's")
			_, _ = fmt.Fprintln(w, "  instruction file:")
			_, _ = fmt.Fprintf(w, "\n    • %s   — for Claude Code\n", cs.Cyan("CLAUDE.md"))
			_, _ = fmt.Fprintf(w, "    • %s   — for GitHub Copilot and other agents\n", cs.Cyan("AGENTS.md"))
			_, _ = fmt.Fprintf(w, "    • %s — Copilot alternate location\n", cs.Cyan(".github/copilot-instructions.md"))
			_, _ = fmt.Fprintf(w, "\n  Or tell your agent to run %s at the start of each session.\n", cs.Cyan("np agent prime"))
			_, _ = fmt.Fprintln(w, "  No hooks or integrations required — np is just a CLI.")

			_, _ = fmt.Fprintf(w, "\n%s\n\n", cs.Bold("Pick an author name"))
			_, _ = fmt.Fprintf(w, "  Every np command that changes data requires an %s flag. Use any\n", cs.Cyan("--author"))
			_, _ = fmt.Fprintln(w, "  stable identifier (your name, handle, etc.), or generate one:")
			_, _ = fmt.Fprintf(w, "\n    %s\n", cs.Cyan("np agent name"))
			_, _ = fmt.Fprintln(w, "\n  Agents should generate their own name at session start. Humans can use whatever")
			_, _ = fmt.Fprintln(w, "  they like — consistency across a session is what matters.")

			_, _ = fmt.Fprintf(w, "\n%s\n\n", cs.Bold("Quick reference"))
			_, _ = fmt.Fprintf(w, "    %s  Create an issue\n", cs.Cyan(`np create --role task --title "..." --author <name>`))
			_, _ = fmt.Fprintf(w, "    %s                                      Find available work\n", cs.Cyan("np list --ready"))
			_, _ = fmt.Fprintf(w, "    %s                       Claim next ready issue\n", cs.Cyan("np claim ready --author <name>"))
			_, _ = fmt.Fprintf(w, "    %s               Complete a task\n", cs.Cyan("np state close <ID> --claim <CLAIM-ID>"))
			_, _ = fmt.Fprintf(w, "    %s                                            Run diagnostics\n", cs.Cyan("np doctor"))
			_, _ = fmt.Fprintf(w, "    %s                                              Full command reference\n", cs.Cyan("np help"))
			_, _ = fmt.Fprintf(w, "    %s                                       Agent workflow instructions\n", cs.Cyan("np agent prime"))

			return nil
		},
	}
}

// checkbox returns a green checkmark for completed steps or an empty checkbox
// for incomplete steps.
func checkbox(cs *iostreams.ColorScheme, done bool) string {
	if done {
		return cs.Green("✓")
	}
	return "[ ]"
}
