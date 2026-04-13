package comment

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// searchCommentOutput is the JSON representation of a comment search result.
type searchCommentOutput struct {
	CommentID string `json:"comment_id"`
	IssueID   string `json:"issue_id"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

// searchOutput is the JSON representation of the comment search result set.
type searchOutput struct {
	Comments []searchCommentOutput `json:"comments"`
	HasMore  bool                  `json:"has_more"`
}

// RunSearchInput holds the parameters for the comment search operation,
// decoupled from CLI flag parsing so it can be tested directly.
type RunSearchInput struct {
	Service     driving.Service
	Query       string
	Filter      driving.CommentFilterInput
	Limit       int
	JSON        bool
	WriteTo     io.Writer
	ColorScheme *iostreams.ColorScheme
}

// RunSearch executes the comment search workflow: searches comments matching the
// query and writes the result to WriteTo. Limit controls the maximum number of
// results; -1 means unbounded.
func RunSearch(ctx context.Context, input RunSearchInput) error {
	output, err := input.Service.SearchComments(ctx, driving.SearchCommentsInput{
		Query:  input.Query,
		Filter: input.Filter,
		Limit:  input.Limit,
	})
	if err != nil {
		return fmt.Errorf("searching comments: %w", err)
	}

	if input.JSON {
		out := searchOutput{
			Comments: make([]searchCommentOutput, 0, len(output.Comments)),
			HasMore:  output.HasMore,
		}
		for _, c := range output.Comments {
			out.Comments = append(out.Comments, searchCommentOutput{
				CommentID: c.DisplayID,
				IssueID:   c.IssueID,
				Author:    c.Author,
				CreatedAt: c.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
				Body:      c.Body,
			})
		}
		return cmdutil.WriteJSON(input.WriteTo, out)
	}

	// Text output.
	w := input.WriteTo
	cs := input.ColorScheme
	if cs == nil {
		cs = iostreams.NewColorScheme(false)
	}

	if len(output.Comments) == 0 {
		_, _ = fmt.Fprintln(w, "No matching comments found.")
		return nil
	}

	for _, c := range output.Comments {
		authorStr := truncate(c.Author, 24)
		snippet := truncate(c.Body, 80)
		_, _ = fmt.Fprintf(w, "%s  %s  %s\n",
			cs.Cyan(c.IssueID),
			cs.Dim(authorStr),
			snippet,
		)
	}

	if output.HasMore {
		_, _ = fmt.Fprintf(w, "%s\n", cs.Dim("(more results available)"))
	}

	return nil
}

// newSearchCmd constructs "comment search" which performs full-text search
// across comment bodies with optional issue-scoping flags.
func newSearchCmd(f *cmdutil.Factory) *cli.Command {
	var (
		jsonOutput bool
		issueArgs  []string
		parentArgs []string
		treeArgs   []string
		authorArgs []string
		labelArgs  []string
		followRefs bool
		limit      int
		noLimit    bool
	)

	return &cli.Command{
		Name:  "search",
		Usage: "Search comments by text",
		Description: `Performs full-text search across all comment bodies in the tracker. The
positional argument is the search query; results show the issue ID, comment
author, and a truncated body snippet for each match. Use the scoping flags
to narrow results to specific issues, subtrees, authors, or labeled issues.

This command is especially useful for agents investigating an unfamiliar
area of the tracker — for example, searching for prior discussion about a
specific module, error message, or design decision. The --tree flag scopes
to an entire epic hierarchy, and --follow-refs expands the scope to include
referenced issues, making it easy to find related context across loosely
connected issues.

By default, results are limited to a reasonable page size; use --no-limit or
--limit to adjust.`,
		ArgsUsage: "<query>",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "issue",
				Aliases:     []string{"i"},
				Usage:       "Scope to comments on a specific issue (repeatable, OR'd)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &issueArgs,
			},
			&cli.StringSliceFlag{
				Name:        "parent",
				Usage:       "Scope to comments on an issue and its direct children (repeatable, OR'd)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &parentArgs,
			},
			&cli.StringSliceFlag{
				Name:        "tree",
				Usage:       "Scope to comments on all issues in a tree (repeatable, OR'd)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &treeArgs,
			},
			&cli.StringSliceFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Usage:       "Filter by comment author (repeatable, OR'd)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &authorArgs,
			},
			&cli.StringSliceFlag{
				Name:        "label",
				Usage:       "Scope to comments on issues with a label (key:value or key:*; repeatable, OR'd)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &labelArgs,
			},
			&cli.BoolFlag{
				Name:        "follow-refs",
				Usage:       "Expand scope to include referenced issues",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &followRefs,
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
				Usage:       "Output machine-readable JSON",
				Destination: &jsonOutput,
				Category:    cmdutil.FlagCategorySupplemental,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			query := cmd.Args().Get(0)
			if query == "" {
				return cmdutil.FlagErrorf("search query is required")
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}
			resolver := cmdutil.NewIDResolver(svc)

			// Build the filter from flags.
			filter, err := buildSearchFilter(ctx, resolver, issueArgs, parentArgs, treeArgs, authorArgs, labelArgs, followRefs)
			if err != nil {
				return err
			}

			effectiveLimit, err := cmdutil.ResolveLimit(limit, noLimit)
			if err != nil {
				return cmdutil.FlagErrorf("%s", err)
			}

			return RunSearch(ctx, RunSearchInput{
				Service:     svc,
				Query:       query,
				Filter:      filter,
				Limit:       effectiveLimit,
				JSON:        jsonOutput,
				WriteTo:     f.IOStreams.Out,
				ColorScheme: f.IOStreams.ColorScheme(),
			})
		},
	}
}

// buildSearchFilter constructs a CommentFilterInput from the CLI flag values.
// String-typed IDs are resolved through the ID resolver but stored as strings
// in the service-layer DTO; the service parses them into domain types.
func buildSearchFilter(ctx context.Context, resolver *cmdutil.IDResolver, issueArgs, parentArgs, treeArgs, authorArgs, labelArgs []string, followRefs bool) (driving.CommentFilterInput, error) {
	var filter driving.CommentFilterInput

	// Resolve issue IDs.
	for _, raw := range issueArgs {
		id, err := resolver.Resolve(ctx, raw)
		if err != nil {
			return filter, cmdutil.FlagErrorf("invalid --issue: %s", err)
		}
		filter.IssueIDs = append(filter.IssueIDs, id.String())
	}

	// Resolve parent IDs.
	for _, raw := range parentArgs {
		id, err := resolver.Resolve(ctx, raw)
		if err != nil {
			return filter, cmdutil.FlagErrorf("invalid --parent: %s", err)
		}
		filter.ParentIDs = append(filter.ParentIDs, id.String())
	}

	// Resolve tree IDs.
	for _, raw := range treeArgs {
		id, err := resolver.Resolve(ctx, raw)
		if err != nil {
			return filter, cmdutil.FlagErrorf("invalid --tree: %s", err)
		}
		filter.TreeIDs = append(filter.TreeIDs, id.String())
	}

	// Author names are passed as strings; the service validates them.
	filter.Authors = authorArgs

	// Parse label filters.
	labelFilters, err := cmdutil.ParseLabelFilters(labelArgs)
	if err != nil {
		return filter, cmdutil.FlagErrorf("%s", err)
	}
	filter.LabelFilters = labelFilters

	filter.FollowRefs = followRefs

	return filter, nil
}
