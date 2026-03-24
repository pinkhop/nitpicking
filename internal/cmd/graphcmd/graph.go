// Package graphcmd implements the "graph" CLI command, which renders the
// issue hierarchy and relationships as a Graphviz DOT file.
package graphcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain/graph"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// graphOutput is the JSON representation of the graph command result.
type graphOutput struct {
	DOT string `json:"dot"`
}

// NewCmd constructs the "graph" command, which generates a Graphviz DOT
// representation of all issues and their relationships.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput    bool
		outputFile    string
		includeClosed bool
	)

	return &cli.Command{
		Name:  "graph",
		Usage: "Generate a Graphviz DOT graph of issues and relationships",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "Write DOT output to a file instead of stdout",
				Category:    "Options",
				Destination: &outputFile,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Usage:       "Include closed issues in the graph (hidden by default)",
				Category:    "Options",
				Destination: &includeClosed,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output DOT content as a JSON string field",
				Category:    "Options",
				Destination: &jsonOutput,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			data, err := svc.GetGraphData(ctx)
			if err != nil {
				return fmt.Errorf("fetching graph data: %w", err)
			}

			// Filter closed issues unless --include-closed is set.
			visibleNodes := data.Nodes
			if !includeClosed {
				visibleNodes = make([]port.IssueListItem, 0, len(data.Nodes))
				for _, item := range data.Nodes {
					if item.State != issue.StateClosed {
						visibleNodes = append(visibleNodes, item)
					}
				}
			}

			// Build a set of visible node IDs for edge filtering.
			visibleSet := make(map[string]bool, len(visibleNodes))
			for _, item := range visibleNodes {
				visibleSet[item.ID.String()] = true
			}

			// Convert service output to graph renderer input.
			nodes := make([]graph.Node, 0, len(visibleNodes))
			for _, item := range visibleNodes {
				nodes = append(nodes, graph.Node{
					ID:       item.ID,
					Role:     item.Role,
					State:    item.State,
					Title:    item.Title,
					ParentID: item.ParentID,
				})
			}

			edges := make([]graph.Edge, 0, len(data.Relationships))
			for _, rel := range data.Relationships {
				// Only include blocked_by and cites (not their inverses) to
				// avoid duplicate edges. Also skip edges where either endpoint
				// is not in the visible set (filtered out by --include-closed).
				if !visibleSet[rel.SourceID().String()] || !visibleSet[rel.TargetID().String()] {
					continue
				}
				if rel.Type() == issue.RelBlockedBy || rel.Type() == issue.RelCites {
					edges = append(edges, graph.Edge{
						SourceID: rel.SourceID(),
						TargetID: rel.TargetID(),
						Type:     rel.Type(),
					})
				}
			}

			dot := graph.RenderDOT(nodes, edges)

			if outputFile != "" {
				if err := os.WriteFile(outputFile, []byte(dot), 0o644); err != nil {
					return fmt.Errorf("writing to %s: %w", outputFile, err)
				}
				cs := f.IOStreams.ColorScheme()
				_, err := fmt.Fprintf(f.IOStreams.Out, "%s Wrote graph to %s\n",
					cs.SuccessIcon(), outputFile)
				return err
			}

			if jsonOutput {
				return cmdutil.WriteJSON(f.IOStreams.Out, graphOutput{DOT: dot})
			}

			_, err = fmt.Fprint(f.IOStreams.Out, dot)
			return err
		},
	}
}
