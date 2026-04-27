package relcmd_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// addRefs adds a "refs" relationship between issueA and issueB.
func addRefs(t *testing.T, svc driving.Service, issueA, issueB, author string) {
	t.Helper()
	err := svc.AddRelationship(t.Context(), issueA, driving.RelationshipInput{
		Type:     domain.RelRefs,
		TargetID: issueB,
	}, author)
	if err != nil {
		t.Fatalf("precondition: add refs %q ↔ %q: %v", issueA, issueB, err)
	}
}

// renderRefs invokes RenderRefsSection in non-TTY mode and returns the output
// as a string. Tests that need TTY or a specific terminal width should build
// their own ios via iostreams.Test() and call RenderRefsSection directly.
func renderRefs(t *testing.T, svc driving.Service) string {
	t.Helper()

	ios, _, out, _ := iostreams.Test()

	if err := relcmd.RenderRefsSection(t.Context(), svc, ios); err != nil {
		t.Fatalf("RenderRefsSection failed: %v", err)
	}
	return out.String()
}

// TestRenderRefsSection_EmptyGraph_ShowsZeroCounts verifies that an empty
// database renders the section header with zero counts and no rows.
func TestRenderRefsSection_EmptyGraph_ShowsZeroCounts(t *testing.T) {
	t.Parallel()

	// Given: an empty database.
	svc := setupService(t)

	// When: rendering the refs section.
	output := renderRefs(t, svc)

	// Then: the header shows zero components and zero edges.
	if !strings.Contains(output, "Refs (0 components, 0 edges)") {
		t.Errorf("expected zero-count header; output:\n%s", output)
	}
	// Then: only the header line (no edge rows).
	lines := nonEmptyLines(output)
	if len(lines) != 1 {
		t.Errorf("line count: got %d, want 1 (header only); output:\n%s", len(lines), output)
	}
}

// TestRenderRefsSection_NoRefsEdges_ShowsZeroCounts verifies that issues with
// no refs relationships produce zero counts.
func TestRenderRefsSection_NoRefsEdges_ShowsZeroCounts(t *testing.T) {
	t.Parallel()

	// Given: two tasks with no refs relationship between them.
	svc := setupService(t)
	_ = createTask(t, svc, "Task A")
	_ = createTask(t, svc, "Task B")

	// When: rendering the refs section.
	output := renderRefs(t, svc)

	// Then: still zero counts.
	if !strings.Contains(output, "Refs (0 components, 0 edges)") {
		t.Errorf("expected zero-count header with no refs edges; output:\n%s", output)
	}
}

// TestRenderRefsSection_SingleEdge_RendersOneComponentOneEdge verifies that a
// single refs edge produces the correct header, one component, and one edge row.
func TestRenderRefsSection_SingleEdge_RendersOneComponentOneEdge(t *testing.T) {
	t.Parallel()

	// Given: task A refs task B.
	svc := setupService(t)
	a := createTask(t, svc, "Task A")
	b := createTask(t, svc, "Task B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")

	// When: rendering the refs section.
	output := renderRefs(t, svc)

	// Then: 1 component, 1 edge in section header.
	if !strings.Contains(output, "Refs (1 components, 1 edges)") {
		t.Errorf("expected 1-component header; output:\n%s", output)
	}
	// Then: component sub-header shows 2 issues and 1 edge.
	if !strings.Contains(output, "Component 1 (2 issues, 1 edges)") {
		t.Errorf("expected component sub-header; output:\n%s", output)
	}
	// Then: both issue IDs appear in the output.
	if !strings.Contains(output, a.String()) {
		t.Errorf("ID %q missing from output:\n%s", a.String(), output)
	}
	if !strings.Contains(output, b.String()) {
		t.Errorf("ID %q missing from output:\n%s", b.String(), output)
	}
	// Then: the "—" separator appears between the two IDs.
	if !strings.Contains(output, "—") {
		t.Errorf("expected edge separator '—' in output:\n%s", output)
	}
}

// TestRenderRefsSection_SingleEdge_RenderedExactlyOnce verifies that an
// undirected refs edge appears exactly once in the output, regardless of
// which endpoint was the source of the AddRelationship call.
func TestRenderRefsSection_SingleEdge_RenderedExactlyOnce(t *testing.T) {
	t.Parallel()

	// Given: task A refs task B.
	svc := setupService(t)
	a := createTask(t, svc, "Task A")
	b := createTask(t, svc, "Task B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: exactly three non-empty lines: section header, component sub-header,
	// and one edge row.
	lines := nonEmptyLines(output)
	if len(lines) != 3 {
		t.Errorf("expected 3 non-empty lines (header + component-header + edge); got %d; output:\n%s",
			len(lines), output)
	}
}

// TestRenderRefsSection_SingleComponent_MultiEdge verifies that multiple
// connected edges form one component with the correct counts.
func TestRenderRefsSection_SingleComponent_MultiEdge(t *testing.T) {
	t.Parallel()

	// Given: A—B, B—C, C—D forming one connected component of 4 issues and 3 edges.
	svc := setupService(t)
	a := createTask(t, svc, "A")
	b := createTask(t, svc, "B")
	c := createTask(t, svc, "C")
	d := createTask(t, svc, "D")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	addRefs(t, svc, b.String(), c.String(), "test-agent")
	addRefs(t, svc, c.String(), d.String(), "test-agent")

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: 1 component, 3 edges in section header.
	if !strings.Contains(output, "Refs (1 components, 3 edges)") {
		t.Errorf("expected 1-component header with 3 edges; output:\n%s", output)
	}
	// Then: component sub-header shows 4 issues and 3 edges.
	if !strings.Contains(output, "Component 1 (4 issues, 3 edges)") {
		t.Errorf("expected 4-issue component sub-header; output:\n%s", output)
	}
	// Then: all four IDs appear.
	for _, id := range []string{a.String(), b.String(), c.String(), d.String()} {
		if !strings.Contains(output, id) {
			t.Errorf("ID %q missing from output:\n%s", id, output)
		}
	}
}

// TestRenderRefsSection_MultipleComponents_OrderedLargestFirst verifies that
// components are printed largest first by issue count.
func TestRenderRefsSection_MultipleComponents_OrderedLargestFirst(t *testing.T) {
	t.Parallel()

	// Given: component 1 (A—B—C: 3 issues, 2 edges) and component 2 (D—E: 2 issues, 1 edge).
	svc := setupService(t)
	a := createTask(t, svc, "A")
	b := createTask(t, svc, "B")
	c := createTask(t, svc, "C")
	d := createTask(t, svc, "D")
	e := createTask(t, svc, "E")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	addRefs(t, svc, b.String(), c.String(), "test-agent")
	addRefs(t, svc, d.String(), e.String(), "test-agent")

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: 2 components, 3 edges total.
	if !strings.Contains(output, "Refs (2 components, 3 edges)") {
		t.Errorf("expected 2-component header; output:\n%s", output)
	}
	// Then: "Component 1 (3 issues, 2 edges)" appears before "Component 2 (2 issues, 1 edge)".
	pos1 := strings.Index(output, "Component 1 (3 issues")
	pos2 := strings.Index(output, "Component 2 (2 issues")
	if pos1 < 0 {
		t.Fatalf("expected 'Component 1 (3 issues...)' in output:\n%s", output)
	}
	if pos2 < 0 {
		t.Fatalf("expected 'Component 2 (2 issues...)' in output:\n%s", output)
	}
	if pos1 >= pos2 {
		t.Errorf("larger component should appear before smaller; output:\n%s", output)
	}
}

// TestRenderRefsSection_TieBreaking_SmallestIDFirst verifies that when two
// components have the same issue count, the one with the smaller minimum ID
// is printed first. IDs are random, so we determine expected ordering from
// the actual created IDs.
func TestRenderRefsSection_TieBreaking_SmallestIDFirst(t *testing.T) {
	t.Parallel()

	// Given: two isolated pairs forming two equal-size components.
	svc := setupService(t)
	a := createTask(t, svc, "Pair1-A")
	b := createTask(t, svc, "Pair1-B")
	c := createTask(t, svc, "Pair2-C")
	d := createTask(t, svc, "Pair2-D")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	addRefs(t, svc, c.String(), d.String(), "test-agent")

	// When: rendering.
	output := renderRefs(t, svc)

	// Determine which pair has the smaller minimum ID so we know the expected
	// print order (smaller minimum ID should be Component 1).
	minAB := min(a.String(), b.String())
	minCD := min(c.String(), d.String())

	var firstMin, secondMin string
	if minAB < minCD {
		firstMin, secondMin = minAB, minCD
	} else {
		firstMin, secondMin = minCD, minAB
	}

	// Then: the component whose minimum ID is smaller appears first.
	posFirst := strings.Index(output, firstMin)
	posSecond := strings.Index(output, secondMin)
	if posFirst < 0 || posSecond < 0 {
		t.Fatalf("expected both minimum IDs in output; got:\n%s", output)
	}
	if posFirst >= posSecond {
		t.Errorf("component with smaller min ID (%s) should appear before (%s); output:\n%s",
			firstMin, secondMin, output)
	}
}

// TestRenderRefsSection_EndpointsSortedLexicographically verifies that within
// each edge row, the endpoint with the lexicographically smaller ID appears on
// the left, regardless of which endpoint was the source of the AddRelationship
// call.
func TestRenderRefsSection_EndpointsSortedLexicographically(t *testing.T) {
	t.Parallel()

	// Given: two tasks. IDs are random, so we determine the expected order
	// from the actual created IDs.
	svc := setupService(t)
	a := createTask(t, svc, "Task A")
	b := createTask(t, svc, "Task B")
	// Add refs with b as source (so b would naively be "left" if unsorted).
	addRefs(t, svc, b.String(), a.String(), "test-agent")

	// When: rendering.
	output := renderRefs(t, svc)

	// Determine which ID is lexicographically smaller.
	var smallerID, largerID string
	if a.String() < b.String() {
		smallerID, largerID = a.String(), b.String()
	} else {
		smallerID, largerID = b.String(), a.String()
	}

	// Then: the smaller ID appears to the left of the "—" separator.
	posSmaller := strings.Index(output, smallerID)
	posSep := strings.Index(output, "—")
	posLarger := strings.Index(output, largerID)
	if posSmaller < 0 || posSep < 0 || posLarger < 0 {
		t.Fatalf("expected both IDs and separator in output; got:\n%s", output)
	}
	if !(posSmaller < posSep && posSep < posLarger) {
		t.Errorf("expected smaller ID left of separator left of larger ID; "+
			"positions smaller=%d sep=%d larger=%d; output:\n%s",
			posSmaller, posSep, posLarger, output)
	}
}

// TestRenderRefsSection_ClosedEndpoint_EdgeSuppressed verifies that an edge
// where one endpoint is closed does not appear in the refs output.
func TestRenderRefsSection_ClosedEndpoint_EdgeSuppressed(t *testing.T) {
	t.Parallel()

	// Given: A refs B, but B is closed.
	svc := setupService(t)
	a := createTask(t, svc, "Open task A")
	b := createTask(t, svc, "Closed task B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	closeTestIssue(t, svc, b.String())

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: zero counts (B is closed, so the edge is suppressed).
	if !strings.Contains(output, "Refs (0 components, 0 edges)") {
		t.Errorf("expected zero counts when one endpoint is closed; output:\n%s", output)
	}
	// Then: neither the open A nor the closed B appears in any edge row.
	if strings.Contains(output, b.String()) {
		t.Errorf("closed issue %q should not appear in output:\n%s", b.String(), output)
	}
}

// TestRenderRefsSection_BothEndpointsClosed_EdgeSuppressed verifies that an
// edge where both endpoints are closed is suppressed.
func TestRenderRefsSection_BothEndpointsClosed_EdgeSuppressed(t *testing.T) {
	t.Parallel()

	// Given: A refs B, and both A and B are closed.
	svc := setupService(t)
	a := createTask(t, svc, "Closed A")
	b := createTask(t, svc, "Closed B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	closeTestIssue(t, svc, a.String())
	closeTestIssue(t, svc, b.String())

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: zero counts.
	if !strings.Contains(output, "Refs (0 components, 0 edges)") {
		t.Errorf("expected zero counts when both endpoints are closed; output:\n%s", output)
	}
}

// TestRenderRefsSection_PartialSuppression_ClosedPeerDropsEdge verifies that
// when one issue refs two peers and only one peer is closed, only the edge to
// the open peer remains and the resulting single component is correct.
func TestRenderRefsSection_PartialSuppression_ClosedPeerDropsEdge(t *testing.T) {
	t.Parallel()

	// Given: A refs B and A refs C; then C is closed.
	svc := setupService(t)
	a := createTask(t, svc, "A (open)")
	b := createTask(t, svc, "B (open)")
	c := createTask(t, svc, "C (closed)")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	addRefs(t, svc, a.String(), c.String(), "test-agent")
	closeTestIssue(t, svc, c.String())

	// When: rendering.
	output := renderRefs(t, svc)

	// Then: 1 component, 1 edge (the A—C edge is suppressed).
	if !strings.Contains(output, "Refs (1 components, 1 edges)") {
		t.Errorf("expected 1-component header after partial suppression; output:\n%s", output)
	}
	// Then: A and B appear (the surviving edge).
	if !strings.Contains(output, a.String()) {
		t.Errorf("open issue A missing from output:\n%s", output)
	}
	if !strings.Contains(output, b.String()) {
		t.Errorf("open issue B missing from output:\n%s", output)
	}
	// Then: the closed issue C does not appear.
	if strings.Contains(output, c.String()) {
		t.Errorf("closed issue C should not appear in output:\n%s", output)
	}
}

// TestRenderRefsSection_NonTTY_FullTitles verifies that non-TTY output does
// not truncate titles.
func TestRenderRefsSection_NonTTY_FullTitles(t *testing.T) {
	t.Parallel()

	// Given: two tasks with very long titles.
	svc := setupService(t)
	longTitleA := strings.Repeat("A", 120)
	longTitleB := strings.Repeat("B", 120)
	a := createTask(t, svc, longTitleA)
	b := createTask(t, svc, longTitleB)
	addRefs(t, svc, a.String(), b.String(), "test-agent")

	// When: rendering in non-TTY mode.
	output := renderRefs(t, svc)

	// Then: the full titles appear unchanged (no ellipsis).
	if !strings.Contains(output, longTitleA) {
		t.Errorf("expected full title A in non-TTY output; output:\n%s", output)
	}
	if !strings.Contains(output, longTitleB) {
		t.Errorf("expected full title B in non-TTY output; output:\n%s", output)
	}
	if strings.Contains(output, "…") {
		t.Errorf("unexpected truncation ellipsis in non-TTY output; output:\n%s", output)
	}
}

// TestRenderRefsSection_TTY_TruncatesLongTitles verifies that TTY output
// truncates titles that exceed half the available terminal width.
func TestRenderRefsSection_TTY_TruncatesLongTitles(t *testing.T) {
	t.Parallel()

	// Given: a narrow terminal (80 cols) and two very long titles.
	svc := setupService(t)
	longTitle := strings.Repeat("X", 200)
	a := createTask(t, svc, longTitle)
	b := createTask(t, svc, longTitle)
	addRefs(t, svc, a.String(), b.String(), "test-agent")

	ios, _, out, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetTerminalWidth(80)

	// When: rendering with a narrow TTY.
	if err := relcmd.RenderRefsSection(t.Context(), svc, ios); err != nil {
		t.Fatalf("RenderRefsSection failed: %v", err)
	}
	output := out.String()

	// Then: ellipsis appears (at least one title was truncated).
	if !strings.Contains(output, "…") {
		t.Errorf("expected truncation ellipsis in narrow TTY output; output:\n%s", output)
	}
	// Then: the full 200-char title does NOT appear.
	if strings.Contains(output, longTitle) {
		t.Errorf("full 200-char title should be truncated; output:\n%s", output)
	}
}

// noopRenderer is a SectionRenderer that does nothing and returns no error.
// Used in dispatcher tests that want to isolate one section renderer.
func noopRenderer(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
	return nil
}

// TestRenderRefsSection_NoFilter_RefsSectionIncluded verifies that running
// RunList with no --rel filter includes the refs section in combined output.
func TestRenderRefsSection_NoFilter_RefsSectionIncluded(t *testing.T) {
	t.Parallel()

	// Given: a service with one refs edge.
	svc := setupService(t)
	a := createTask(t, svc, "A")
	b := createTask(t, svc, "B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	ios, _, out, _ := iostreams.Test()

	// When: running list with no filter, using the real refs renderer.
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RenderParentChild: noopRenderer,
		RenderBlocking:    noopRenderer,
		RenderRefs:        relcmd.RenderRefsSection,
	}
	if err := relcmd.RunList(t.Context(), input); err != nil {
		t.Fatalf("RunList failed: %v", err)
	}

	// Then: the refs section header appears.
	if !strings.Contains(out.String(), "Refs (") {
		t.Errorf("expected refs section header in no-filter output; got:\n%s", out.String())
	}
}

// TestRenderRefsSection_RunList_DispatchedCorrectly verifies that RunList
// dispatches to the refs renderer when the refs filter is active.
func TestRenderRefsSection_RunList_DispatchedCorrectly(t *testing.T) {
	t.Parallel()

	// Given: a service with one refs edge.
	svc := setupService(t)
	a := createTask(t, svc, "A")
	b := createTask(t, svc, "B")
	addRefs(t, svc, a.String(), b.String(), "test-agent")
	ios, _, out, _ := iostreams.Test()

	// When: running list with refs filter.
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RelFilter:         relcmd.RelListCategoryRefs,
		RenderParentChild: noopRenderer,
		RenderBlocking:    noopRenderer,
		RenderRefs:        relcmd.RenderRefsSection,
	}
	if err := relcmd.RunList(t.Context(), input); err != nil {
		t.Fatalf("RunList failed: %v", err)
	}

	// Then: the refs section header appears in output.
	if !strings.Contains(out.String(), "Refs (") {
		t.Errorf("expected refs section header in output; got:\n%s", out.String())
	}
}
