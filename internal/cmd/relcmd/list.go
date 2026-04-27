package relcmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RelListCategory is the canonical section identifier used by the np rel list
// --rel flag. Aliases from np rel add are accepted as input but always resolve
// to one of these three values.
type RelListCategory string

const (
	// RelListCategoryParentChild selects the parent-child hierarchy section.
	RelListCategoryParentChild RelListCategory = "parent-child"

	// RelListCategoryBlocking selects the blocking dependency section.
	RelListCategoryBlocking RelListCategory = "blocking"

	// RelListCategoryRefs selects the contextual reference section.
	RelListCategoryRefs RelListCategory = "refs"
)

// validRelListCanonical lists the three canonical --rel values.
const validRelListCanonical = "blocking, refs, parent-child"

// validRelListAliases lists the np rel add aliases accepted as --rel values.
const validRelListAliases = "blocked_by, blocks, parent_of, child_of"

// validRelListArgs is the combined list of canonical values and aliases for
// --rel flag Usage text.
const validRelListArgs = validRelListCanonical + ", " + validRelListAliases

// sectionSeparatorFallbackWidth is the separator line width used on non-TTY
// output (when TerminalWidth returns a non-positive value). TTY output uses
// the actual terminal width instead.
const sectionSeparatorFallbackWidth = 80

// ParseRelListCategory maps a raw --rel value to its canonical RelListCategory.
// It accepts both the canonical names (blocking, refs, parent-child) and the
// aliases used by np rel add (blocked_by, blocks, parent_of, child_of). The
// empty string is not accepted; call sites that represent "no filter" should
// skip calling ParseRelListCategory rather than passing an empty string.
// Returns an error for any unrecognized value.
func ParseRelListCategory(s string) (RelListCategory, error) {
	switch s {
	case "blocking", "blocked_by", "blocks":
		return RelListCategoryBlocking, nil
	case "refs":
		return RelListCategoryRefs, nil
	case "parent-child", "parent_of", "child_of":
		return RelListCategoryParentChild, nil
	default:
		return "", fmt.Errorf(
			"invalid --rel value %q: canonical values are %s; aliases are %s",
			s, validRelListCanonical, validRelListAliases,
		)
	}
}

// SectionRenderer is the function type for rendering a single section of
// np rel list output. It receives the service for data access and the IOStreams
// for TTY-aware, color-enabled output.
type SectionRenderer func(ctx context.Context, svc driving.Service, ios *iostreams.IOStreams) error

// RunListInput holds all parameters for RunList, decoupled from CLI flag
// parsing to allow direct invocation from tests with injectable renderers.
type RunListInput struct {
	// Service provides access to the issue tracker data.
	Service driving.Service

	// IOStreams provides output and TTY/color detection to section renderers.
	IOStreams *iostreams.IOStreams

	// RelFilter restricts the run to one section. The zero value (empty
	// string) means all sections run. Any non-empty value must be one of the
	// three canonical RelListCategory constants; callers are responsible for
	// using ParseRelListCategory to validate user-supplied strings before
	// placing them here. An unrecognized non-empty value causes RunList to
	// return an error rather than silently running zero sections.
	RelFilter RelListCategory

	// RenderParentChild renders the parent-child hierarchy section.
	RenderParentChild SectionRenderer

	// RenderBlocking renders the blocking dependency section.
	RenderBlocking SectionRenderer

	// RenderRefs renders the contextual reference section.
	RenderRefs SectionRenderer
}

// RunList dispatches to section renderers based on RelFilter. With an empty
// filter, all three sections run in order: parent-child, blocking, refs. With
// a filter set to one of the canonical RelListCategory constants, only the
// matching section runs. The section header is always included in the
// renderer's output regardless of whether a filter is active. Returns an error
// if RelFilter is non-empty and does not match any of the three known sections.
func RunList(ctx context.Context, input RunListInput) error {
	type pair struct {
		cat      RelListCategory
		renderer SectionRenderer
	}
	sections := []pair{
		{RelListCategoryParentChild, input.RenderParentChild},
		{RelListCategoryBlocking, input.RenderBlocking},
		{RelListCategoryRefs, input.RenderRefs},
	}

	// Guard against callers that supply an unrecognized non-empty filter,
	// which would silently produce no output.
	if input.RelFilter != "" {
		known := false
		for _, s := range sections {
			if s.cat == input.RelFilter {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown section category %q", input.RelFilter)
		}
	}

	printedAny := false
	for _, s := range sections {
		if input.RelFilter != "" && s.cat != input.RelFilter {
			continue
		}
		if printedAny {
			// Print a horizontal rule between sections. The rule spans the
			// terminal width on TTY; it falls back to a fixed width on non-TTY.
			width := input.IOStreams.TerminalWidth()
			if width <= 0 {
				width = sectionSeparatorFallbackWidth
			}
			if _, err := fmt.Fprintln(input.IOStreams.Out, strings.Repeat("-", width)); err != nil {
				return fmt.Errorf("writing section separator: %w", err)
			}
		}
		if err := s.renderer(ctx, input.Service, input.IOStreams); err != nil {
			return err
		}
		printedAny = true
	}
	return nil
}

// newListCmd constructs the "rel list" command, which lists all relationships
// across active issues. With no --rel flag, all three sections (parent-child,
// blocking, refs) are rendered in order. With --rel, only the specified
// section is shown.
func newListCmd(f *cmdutil.Factory) *cli.Command {
	var relFilter string

	return &cli.Command{
		Name:  "list",
		Usage: "List all relationships across active issues",
		Description: `Shows all relationships between non-closed issues, organized into three
sections: parent-child hierarchy, blocking dependencies, and contextual
references. Use --rel to restrict output to a single section.

Valid --rel values:
  blocking     — blocking dependency chains (aliases: blocked_by, blocks)
  refs         — contextual reference clusters
  parent-child — parent-child hierarchy (aliases: parent_of, child_of)`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "rel",
				Usage:       "Filter to one section: " + validRelListArgs,
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &relFilter,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			var cat RelListCategory
			if relFilter != "" {
				var err error
				cat, err = ParseRelListCategory(relFilter)
				if err != nil {
					return cmdutil.FlagErrorf("%s", err)
				}
			}

			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			return RunList(ctx, RunListInput{
				Service:           svc,
				IOStreams:         f.IOStreams,
				RelFilter:         cat,
				RenderParentChild: RenderParentChildSection,
				RenderBlocking:    RenderBlockingSection,
				RenderRefs:        RenderRefsSection,
			})
		},
	}
}
