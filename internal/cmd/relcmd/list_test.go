package relcmd_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/relcmd"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- ParseRelListCategory Tests ---

func TestParseRelListCategory_CanonicalValues_ReturnsSelf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  relcmd.RelListCategory
	}{
		{"blocking", relcmd.RelListCategoryBlocking},
		{"refs", relcmd.RelListCategoryRefs},
		{"parent-child", relcmd.RelListCategoryParentChild},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			// When: parsing a canonical category value.
			got, err := relcmd.ParseRelListCategory(tc.input)
			// Then: no error and the correct category is returned.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("category: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRelListCategory_Aliases_MapsToExpectedCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		alias string
		want  relcmd.RelListCategory
	}{
		{"blocked_by", relcmd.RelListCategoryBlocking},
		{"blocks", relcmd.RelListCategoryBlocking},
		{"parent_of", relcmd.RelListCategoryParentChild},
		{"child_of", relcmd.RelListCategoryParentChild},
	}

	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			t.Parallel()

			// When: parsing a rel-add-style alias value.
			got, err := relcmd.ParseRelListCategory(tc.alias)
			// Then: no error and the alias resolves to the correct category.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("category: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRelListCategory_InvalidValue_ReturnsError(t *testing.T) {
	t.Parallel()

	// When: parsing a value that is not a recognized canonical or alias.
	_, err := relcmd.ParseRelListCategory("depends_on")

	// Then: an error is returned.
	if err == nil {
		t.Fatal("expected error for invalid --rel value, got nil")
	}
}

// --- RunList Tests ---

func TestRunList_NoFilter_InvokesAllSectionsInOrder(t *testing.T) {
	t.Parallel()

	// Given: spy renderers that record the invocation order.
	svc := setupService(t)
	ios, _, _, _ := iostreams.Test()
	var callOrder []string

	input := relcmd.RunListInput{
		Service:   svc,
		IOStreams: ios,
		RenderParentChild: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
			callOrder = append(callOrder, "parent-child")
			return nil
		},
		RenderBlocking: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
			callOrder = append(callOrder, "blocking")
			return nil
		},
		RenderRefs: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
			callOrder = append(callOrder, "refs")
			return nil
		},
	}

	// When: running list with no filter.
	err := relcmd.RunList(t.Context(), input)
	// Then: all three sections are called in parent-child → blocking → refs order.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"parent-child", "blocking", "refs"}
	if len(callOrder) != len(want) {
		t.Fatalf("section call count: got %d, want %d; calls: %v", len(callOrder), len(want), callOrder)
	}
	for i, w := range want {
		if callOrder[i] != w {
			t.Errorf("section[%d]: got %q, want %q", i, callOrder[i], w)
		}
	}
}

func TestRunList_SingleFilter_InvokesOnlyMatchingSection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filter      relcmd.RelListCategory
		wantSection string
	}{
		{"blocking", relcmd.RelListCategoryBlocking, "blocking"},
		{"parent-child", relcmd.RelListCategoryParentChild, "parent-child"},
		{"refs", relcmd.RelListCategoryRefs, "refs"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given: spy renderers.
			svc := setupService(t)
			ios, _, _, _ := iostreams.Test()
			var called []string

			input := relcmd.RunListInput{
				Service:   svc,
				IOStreams: ios,
				RelFilter: tc.filter,
				RenderParentChild: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
					called = append(called, "parent-child")
					return nil
				},
				RenderBlocking: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
					called = append(called, "blocking")
					return nil
				},
				RenderRefs: func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
					called = append(called, "refs")
					return nil
				},
			}

			// When: running list with a specific filter.
			err := relcmd.RunList(t.Context(), input)
			// Then: only the matching section renderer is invoked.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(called) != 1 {
				t.Fatalf("section call count: got %d, want 1; calls: %v", len(called), called)
			}
			if called[0] != tc.wantSection {
				t.Errorf("section: got %q, want %q", called[0], tc.wantSection)
			}
		})
	}
}

func TestRunList_UnknownFilter_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: spy renderers and an unrecognized RelFilter value (bypassing
	// ParseRelListCategory, which would normally reject it).
	svc := setupService(t)
	ios, _, _, _ := iostreams.Test()
	var called []string

	neverCalled := func(_ context.Context, _ driving.Service, _ *iostreams.IOStreams) error {
		called = append(called, "called")
		return nil
	}
	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RelFilter:         relcmd.RelListCategory("unknown-category"),
		RenderParentChild: neverCalled,
		RenderBlocking:    neverCalled,
		RenderRefs:        neverCalled,
	}

	// When: running list with an unrecognized filter.
	err := relcmd.RunList(t.Context(), input)

	// Then: an error is returned and no renderer is invoked.
	if err == nil {
		t.Fatal("expected error for unknown RelFilter, got nil")
	}
	if len(called) != 0 {
		t.Errorf("renderers should not be called for unknown filter; got %v", called)
	}
}

// --- Separator Tests ---

// sectionSpy returns a SectionRenderer that writes its name to ios.Out so the
// separator test can verify where separators appear relative to section output.
func sectionSpy(name string) relcmd.SectionRenderer {
	return func(_ context.Context, _ driving.Service, ios *iostreams.IOStreams) error {
		_, err := fmt.Fprintln(ios.Out, name)
		return err
	}
}

func TestRunList_NoFilter_PrintsSeparatorBetweenSections(t *testing.T) {
	t.Parallel()

	// Given: spy renderers writing their names, and non-TTY streams (TerminalWidth == 0).
	svc := setupService(t)
	ios, _, stdout, _ := iostreams.Test()

	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RenderParentChild: sectionSpy("parent-child"),
		RenderBlocking:    sectionSpy("blocking"),
		RenderRefs:        sectionSpy("refs"),
	}

	// When: running list with no filter.
	err := relcmd.RunList(t.Context(), input)
	// Then: a separator appears between sections but not before the first.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sep := strings.Repeat("-", 80) // fallback width for non-TTY
	want := "parent-child\n" + sep + "\n" + "blocking\n" + sep + "\n" + "refs\n"
	got := stdout.String()
	if got != want {
		t.Errorf("output mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestRunList_SingleFilter_NoSeparator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter relcmd.RelListCategory
	}{
		{"blocking", relcmd.RelListCategoryBlocking},
		{"parent-child", relcmd.RelListCategoryParentChild},
		{"refs", relcmd.RelListCategoryRefs},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Given: spy renderers and a single-section filter.
			svc := setupService(t)
			ios, _, stdout, _ := iostreams.Test()

			input := relcmd.RunListInput{
				Service:           svc,
				IOStreams:         ios,
				RelFilter:         tc.filter,
				RenderParentChild: sectionSpy("parent-child"),
				RenderBlocking:    sectionSpy("blocking"),
				RenderRefs:        sectionSpy("refs"),
			}

			// When: running list with a single-section filter.
			err := relcmd.RunList(t.Context(), input)
			// Then: output is exactly the matching section's spy line — no separator.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := tc.name + "\n"
			if got := stdout.String(); got != want {
				t.Errorf("output mismatch:\ngot:  %q\nwant: %q", got, want)
			}
		})
	}
}

func TestRunList_NoFilter_SeparatorWidthMatchesTerminalWidth(t *testing.T) {
	t.Parallel()

	// Given: spy renderers and a TTY with a known terminal width.
	svc := setupService(t)
	ios, _, stdout, _ := iostreams.Test()
	ios.SetTerminalWidth(40)

	input := relcmd.RunListInput{
		Service:           svc,
		IOStreams:         ios,
		RenderParentChild: sectionSpy("parent-child"),
		RenderBlocking:    sectionSpy("blocking"),
		RenderRefs:        sectionSpy("refs"),
	}

	// When: running list with no filter.
	err := relcmd.RunList(t.Context(), input)
	// Then: separators are 40 dashes wide (the configured terminal width).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sep := strings.Repeat("-", 40)
	want := "parent-child\n" + sep + "\n" + "blocking\n" + sep + "\n" + "refs\n"
	got := stdout.String()
	if got != want {
		t.Errorf("output mismatch:\ngot:  %q\nwant: %q", got, want)
	}
}
