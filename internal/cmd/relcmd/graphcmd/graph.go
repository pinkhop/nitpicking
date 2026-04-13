// Package graphcmd implements the "graph" CLI command, which renders the
// issue hierarchy and relationships as a Graphviz DOT file.
package graphcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// Format identifies the output format for the graph command.
type Format string

const (
	// FormatDOT renders a Graphviz DOT file.
	FormatDOT Format = "dot"
	// FormatJSON renders a nested JSON graph structure.
	FormatJSON Format = "json"
	// FormatText renders an ASCII tree.
	FormatText Format = "text"
)

// ParseFormat converts a user-provided string into a Format constant.
// Returns an error for unrecognized values.
func ParseFormat(s string) (Format, error) {
	switch s {
	case "dot":
		return FormatDOT, nil
	case "json":
		return FormatJSON, nil
	case "text":
		return FormatText, nil
	default:
		return "", fmt.Errorf("invalid format %q: must be dot, json, or text", s)
	}
}

// RunInput holds the parameters for the graph command's core logic, decoupled
// from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service       driving.Service
	Format        Format
	IncludeClosed bool
	WriteTo       io.Writer
}

// Run executes the graph workflow: fetches all issues and relationships,
// filters by visibility, and dispatches to the appropriate format renderer.
func Run(ctx context.Context, input RunInput) error {
	switch input.Format {
	case FormatDOT:
		return runDOT(ctx, input)
	case FormatJSON:
		return runJSON(ctx, input)
	case FormatText:
		return runText(ctx, input)
	default:
		return fmt.Errorf("unknown format %q", input.Format)
	}
}

// filteredGraph holds the visible nodes and relationships after filtering
// closed issues. Shared by all format renderers.
type filteredGraph struct {
	nodes         []driving.IssueListItemDTO
	relationships []driving.RelationshipDTO
	visibleSet    map[string]bool
}

// fetchAndFilter loads graph data from the service and filters out closed
// issues (unless includeClosed is true). Returns the filtered data for use
// by format-specific renderers.
func fetchAndFilter(ctx context.Context, input RunInput) (filteredGraph, error) {
	data, err := input.Service.GetGraphData(ctx)
	if err != nil {
		return filteredGraph{}, fmt.Errorf("fetching graph data: %w", err)
	}

	visibleNodes := data.Nodes
	if !input.IncludeClosed {
		visibleNodes = make([]driving.IssueListItemDTO, 0, len(data.Nodes))
		for _, item := range data.Nodes {
			if item.State != domain.StateClosed {
				visibleNodes = append(visibleNodes, item)
			}
		}
	}

	visibleSet := make(map[string]bool, len(visibleNodes))
	for _, item := range visibleNodes {
		visibleSet[item.ID] = true
	}

	return filteredGraph{
		nodes:         visibleNodes,
		relationships: data.Relationships,
		visibleSet:    visibleSet,
	}, nil
}

// runDOT renders the issue graph as Graphviz DOT output.
func runDOT(ctx context.Context, input RunInput) error {
	fg, err := fetchAndFilter(ctx, input)
	if err != nil {
		return err
	}

	nodes := toGraphNodes(fg.nodes)
	edges := toGraphEdges(fg.relationships, fg.visibleSet)

	dot := RenderGraphDOT(nodes, edges)
	_, err = fmt.Fprint(input.WriteTo, dot)
	return err
}

// runJSON renders the issue graph as a nested JSON structure.
func runJSON(ctx context.Context, input RunInput) error {
	fg, err := fetchAndFilter(ctx, input)
	if err != nil {
		return err
	}

	nodes := toGraphNodes(fg.nodes)
	edges := toGraphEdges(fg.relationships, fg.visibleSet)

	jsonStr := RenderGraphJSON(nodes, edges)
	_, err = fmt.Fprint(input.WriteTo, jsonStr)
	return err
}

// toGraphNodes converts service-layer DTOs into GraphNode values for
// rendering. The DTO fields are strings; this function parses them back into
// domain types required by the graph rendering functions.
func toGraphNodes(items []driving.IssueListItemDTO) []GraphNode {
	nodes := make([]GraphNode, 0, len(items))
	for _, item := range items {
		id, _ := domain.ParseID(item.ID)
		role := item.Role
		state := item.State
		var parentID domain.ID
		if item.ParentID != "" {
			parentID, _ = domain.ParseID(item.ParentID)
		}
		nodes = append(nodes, GraphNode{
			ID:       id,
			Role:     role,
			State:    state,
			Title:    item.Title,
			ParentID: parentID,
		})
	}
	return nodes
}

// toGraphEdges converts service-layer relationship DTOs into GraphEdge values
// for rendering. Only blocked_by and cites relationships are included
// (not their inverses) to avoid duplicate edges. Edges where either
// endpoint is not in the visible set are skipped.
func toGraphEdges(rels []driving.RelationshipDTO, visibleSet map[string]bool) []GraphEdge {
	edges := make([]GraphEdge, 0, len(rels))
	for _, rel := range rels {
		if !visibleSet[rel.SourceID] || !visibleSet[rel.TargetID] {
			continue
		}
		if rel.Type == domain.RelBlockedBy.String() || rel.Type == domain.RelCites.String() {
			srcID, _ := domain.ParseID(rel.SourceID)
			tgtID, _ := domain.ParseID(rel.TargetID)
			relType, _ := domain.ParseRelationType(rel.Type)
			edges = append(edges, GraphEdge{
				SourceID: srcID,
				TargetID: tgtID,
				Type:     relType,
			})
		}
	}
	return edges
}

// NewCmd constructs the "graph" command, which renders issue relationships
// in one of several output formats (dot, json, text).
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var (
		formatStr     string
		jsonFlag      bool
		outputFile    string
		includeClosed bool
	)

	return &cli.Command{
		Name:  "graph",
		Usage: "Render a graph of issues and relationships",
		Description: `Renders the entire issue graph — all issues and their relationships — in one
of several output formats. The graph includes parent-child hierarchy edges and
blocking/citation dependency edges, giving a complete picture of how issues
relate to each other.

Output formats:
  dot   — Graphviz DOT language, suitable for rendering with "dot", "neato",
           or other Graphviz tools (e.g., "np rel graph -f dot | dot -Tpng -o graph.png").
  json  — Nested JSON structure for programmatic consumption.
  text  — ASCII tree for terminal viewing.

By default, closed issues are excluded to reduce noise. Use --include-closed
to show them. Use --output to write directly to a file instead of stdout.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "format",
				Aliases:     []string{"f"},
				Usage:       "Output format: dot, json, or text",
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &formatStr,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Alias for --format=json",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &jsonFlag,
			},
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Usage:       "Write output to a file instead of stdout",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &outputFile,
			},
			&cli.BoolFlag{
				Name:        "include-closed",
				Usage:       "Include closed issues in the graph (hidden by default)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &includeClosed,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Resolve format from --format and --json flags.
			format, err := resolveFormat(cmd.IsSet("format"), formatStr, jsonFlag)
			if err != nil {
				return err
			}

			svc, svcErr := cmdutil.NewTracker(f)
			if svcErr != nil {
				return svcErr
			}

			if outputFile != "" {
				// Write to file, then print confirmation message.
				var buf bytes.Buffer
				if err := Run(ctx, RunInput{
					Service:       svc,
					Format:        format,
					IncludeClosed: includeClosed,
					WriteTo:       &buf,
				}); err != nil {
					return err
				}
				if err := os.WriteFile(outputFile, buf.Bytes(), 0o600); err != nil {
					return fmt.Errorf("writing to %s: %w", outputFile, err)
				}
				cs := f.IOStreams.ColorScheme()
				_, err := fmt.Fprintf(f.IOStreams.Out, "%s Wrote graph to %s\n",
					cs.SuccessIcon(), outputFile)
				return err
			}

			return Run(ctx, RunInput{
				Service:       svc,
				Format:        format,
				IncludeClosed: includeClosed,
				WriteTo:       f.IOStreams.Out,
			})
		},
	}
}

// resolveFormat determines the output format from the --format and --json
// flags. It is an error to specify both, or neither.
func resolveFormat(formatSet bool, formatStr string, jsonFlag bool) (Format, error) {
	if formatSet && jsonFlag {
		return "", cmdutil.FlagErrorf("--format and --json are mutually exclusive")
	}
	if jsonFlag {
		return FormatJSON, nil
	}
	if !formatSet {
		return "", cmdutil.FlagErrorf("--format is required (dot, json, or text)")
	}
	return ParseFormat(formatStr)
}
