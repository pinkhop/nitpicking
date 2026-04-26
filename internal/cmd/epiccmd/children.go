package epiccmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// childOutput is the JSON representation of a single child item.
type childOutput struct {
	ID              string `json:"id"`
	Role            string `json:"role"`
	State           string `json:"state"`
	DisplayStatus   string `json:"display_status"`
	Priority        string `json:"priority"`
	Title           string `json:"title"`
	ParentID        string `json:"parent_id,omitempty"`
	ParentCreatedAt string `json:"parent_created_at,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// childrenOutput is the JSON representation of the children list.
type childrenOutput struct {
	Issues  []childOutput `json:"issues"`
	HasMore bool          `json:"has_more"`
}

// newChildrenCmd constructs "epic children" which lists all children of an
// epic, including closed issues.
func newChildrenCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput  bool
		order       string
		limit       int
		noLimit     bool
		columnsFlag string
	)

	return &cli.Command{
		Name:      "children",
		Usage:     "List all children of an epic",
		ArgsUsage: "<EPIC-ID>",
		Description: `Lists all child issues of an epic, including closed children. Unlike
"np list" which excludes closed issues by default, this command shows the
complete picture of an epic's decomposition regardless of child state.

Use this to inspect the full scope of an epic — for example, to review which
tasks remain open, to verify that all children have been closed before running
"epic close-completed", or to understand the decomposition of an epic you are
about to claim. Results are sorted by priority.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "order",
				Usage:       "Sort order: " + cmdutil.ValidOrderNames() + "; append :asc or :desc for direction (default: PRIORITY)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &order,
			},
			&cli.StringFlag{
				Name:        "columns",
				Usage:       "Comma-separated list of columns to display; valid columns: " + cmdutil.ValidColumnNames(),
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &columnsFlag,
			},
			&cli.IntFlag{
				Name:        "limit",
				Aliases:     []string{"n"},
				Usage:       "Maximum number of results",
				Value:       cmdutil.DefaultLimit,
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &limit,
			},
			&cli.BoolFlag{
				Name:        "no-limit",
				Usage:       "Return all matching results",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &noLimit,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			rawID := cmd.Args().Get(0)
			if rawID == "" {
				return cmdutil.FlagErrorf("epic ID argument is required")
			}

			orderBy, direction, err := cmdutil.ParseOrderBy(order)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			epicID, err := resolver.Resolve(ctx, rawID)
			if err != nil {
				return cmdutil.FlagErrorf("invalid epic ID: %s", err)
			}

			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			cols, err := cmdutil.ParseColumns(columnsFlag)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			// Include closed children (unlike the default list behavior).
			result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
				Filter:    driving.IssueFilterInput{ParentIDs: []string{epicID.String()}},
				OrderBy:   orderBy,
				Direction: direction,
				Limit:     effectiveLimit,
			})
			if err != nil {
				return fmt.Errorf("listing children: %w", err)
			}

			if jsonOutput {
				out := childrenOutput{
					HasMore: result.HasMore,
					Issues:  make([]childOutput, 0, len(result.Items)),
				}
				for _, item := range result.Items {
					out.Issues = append(out.Issues, childOutput{
						ID:              item.ID,
						Role:            item.Role.String(),
						State:           item.State.String(),
						DisplayStatus:   item.DisplayStatus,
						Priority:        item.Priority.String(),
						Title:           item.Title,
						ParentID:        item.ParentID,
						ParentCreatedAt: cmdutil.FormatJSONTimestamp(item.ParentCreatedAt),
						CreatedAt:       cmdutil.FormatJSONTimestamp(item.CreatedAt),
					})
				}
				return cmdutil.WriteJSON(f.IOStreams.Out, out)
			}

			w := f.IOStreams.Out
			cs := f.IOStreams.ColorScheme()
			if cs == nil {
				cs = iostreams.NewColorScheme(false)
			}

			if len(result.Items) == 0 {
				_, _ = fmt.Fprintln(w, "No children found.")
				return nil
			}

			if len(cols) == 0 {
				cols = cmdutil.DefaultColumns
			}

			overhead := cmdutil.OverheadForColumns(cols)
			maxTitle := cmdutil.AvailableTitleWidth(f.IOStreams.TerminalWidth(), overhead)

			tw := cmdutil.NewTableWriter(w, 2)
			tw.AddRow(cmdutil.ColumnarHeaderCells(cols)...)

			rc := cmdutil.RenderContext{
				ColorScheme:   cs,
				MaxTitleWidth: maxTitle,
			}
			for _, item := range result.Items {
				tw.AddRow(cmdutil.ColumnarRowCells(item, cols, rc)...)
			}
			// Flush error is best-effort — output is going to stdout.
			_ = tw.Flush()

			shown := len(result.Items)
			_, _ = fmt.Fprintf(w, "\n%s\n",
				cs.Dim(fmt.Sprintf("%d children", shown)))
			if result.HasMore {
				_, _ = fmt.Fprintf(f.IOStreams.ErrOut,
					"Showing %d children (use --no-limit for all)\n", shown)
			}

			return nil
		},
	}
}
