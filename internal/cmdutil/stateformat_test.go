package cmdutil_test

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// ansiRe matches ANSI CSI SGR sequences for stripping in test assertions.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// --- FormatState (single secondary state, list view) ---

func TestFormatState_NoSecondary_ReturnsPrimaryOnly(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state domain.State
		want  string
	}{
		{"closed", domain.StateClosed, "closed"},
	}

	cs := iostreams.NewColorScheme(false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.FormatState(cs, tc.state, domain.SecondaryNone)

			// Then
			if got != tc.want {
				t.Errorf("FormatState() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatState_WithSecondary_ReturnsPrimaryParenSecondary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		state     domain.State
		secondary domain.SecondaryState
		want      string
	}{
		{"open ready", domain.StateOpen, domain.SecondaryReady, "open (ready)"},
		{"open blocked", domain.StateOpen, domain.SecondaryBlocked, "open (blocked)"},
		{"open active", domain.StateOpen, domain.SecondaryActive, "open (active)"},
		{"open completed", domain.StateOpen, domain.SecondaryCompleted, "open (completed)"},
		{"deferred blocked", domain.StateDeferred, domain.SecondaryBlocked, "deferred (blocked)"},
	}

	cs := iostreams.NewColorScheme(false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.FormatState(cs, tc.state, tc.secondary)

			// Then
			if got != tc.want {
				t.Errorf("FormatState() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatState_ColorEnabled_AppliesColor(t *testing.T) {
	t.Parallel()

	cs := iostreams.NewColorScheme(true)

	// When — ready should be green.
	got := cmdutil.FormatState(cs, domain.StateOpen, domain.SecondaryReady)

	// Then — should contain ANSI escape codes.
	if got == "open (ready)" {
		t.Error("expected ANSI-colored output when color is enabled, got plain text")
	}
}

// --- FormatDetailState (compound secondary states, detail view) ---

func TestFormatDetailState_NoSecondary_ReturnsPrimaryOnly(t *testing.T) {
	t.Parallel()

	cs := iostreams.NewColorScheme(false)

	// When
	got := cmdutil.FormatDetailState(cs, domain.StateClosed, nil)

	// Then
	if got != "closed" {
		t.Errorf("FormatDetailState() = %q, want %q", got, "closed")
	}
}

func TestFormatDetailState_SingleSecondary(t *testing.T) {
	t.Parallel()

	cs := iostreams.NewColorScheme(false)

	// When
	got := cmdutil.FormatDetailState(cs, domain.StateOpen, []domain.SecondaryState{domain.SecondaryReady})

	// Then
	if got != "open (ready)" {
		t.Errorf("FormatDetailState() = %q, want %q", got, "open (ready)")
	}
}

func TestFormatDetailState_CompoundSecondary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		state   domain.State
		details []domain.SecondaryState
		want    string
	}{
		{
			"blocked and unplanned",
			domain.StateOpen,
			[]domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryUnplanned},
			"open (blocked, unplanned)",
		},
		{
			"blocked and active",
			domain.StateOpen,
			[]domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryActive},
			"open (blocked, active)",
		},
		{
			"blocked and completed",
			domain.StateOpen,
			[]domain.SecondaryState{domain.SecondaryBlocked, domain.SecondaryCompleted},
			"open (blocked, completed)",
		},
	}

	cs := iostreams.NewColorScheme(false)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.FormatDetailState(cs, tc.state, tc.details)

			// Then
			if got != tc.want {
				t.Errorf("FormatDetailState() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- ColorState (exported, primary state coloring) ---

func TestColorState_ColorEnabled_AppliesCorrect256Color(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		state domain.State
		code  string // expected 256-color ANSI code substring
	}{
		{"closed uses 246", domain.StateClosed, "38;5;246"},
		{"open uses 071", domain.StateOpen, "38;5;071"},
		{"deferred uses 073", domain.StateDeferred, "38;5;073"},
	}

	cs := iostreams.NewColorScheme(true)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.ColorState(cs, tc.state)

			// Then
			if !strings.Contains(got, tc.code) {
				t.Errorf("ColorState(%s) = %q, want ANSI code %q", tc.state, got, tc.code)
			}
			if !strings.Contains(got, tc.state.String()) {
				t.Errorf("ColorState(%s) should contain state text, got %q", tc.state, got)
			}
		})
	}
}

func TestColorState_ColorDisabled_ReturnsPlainText(t *testing.T) {
	t.Parallel()

	cs := iostreams.NewColorScheme(false)

	cases := []struct {
		name  string
		state domain.State
	}{
		{"closed", domain.StateClosed},
		{"open", domain.StateOpen},
		{"deferred", domain.StateDeferred},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.ColorState(cs, tc.state)

			// Then
			if got != tc.state.String() {
				t.Errorf("ColorState(%s) = %q, want plain %q", tc.state, got, tc.state.String())
			}
		})
	}
}

// --- FormatState colors primary state ---

func TestFormatState_ColorEnabled_ColorsPrimaryState(t *testing.T) {
	t.Parallel()

	cs := iostreams.NewColorScheme(true)

	// When — open with ready secondary
	got := cmdutil.FormatState(cs, domain.StateOpen, domain.SecondaryReady)

	// Then — primary "open" should be colored with 256-color code 71
	if !strings.Contains(got, "38;5;071") {
		t.Errorf("expected primary state colored with 38;5;071 (open), got %q", got)
	}
}

func TestFormatState_ColorEnabled_ColorsSecondaryWithNewScheme(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		secondary domain.SecondaryState
		code      string // expected ANSI code
	}{
		{"ready uses open color 071", domain.SecondaryReady, "38;5;071"},
		{"blocked uses blocked color 134", domain.SecondaryBlocked, "38;5;134"},
		{"completed uses closed color 246", domain.SecondaryCompleted, "38;5;246"},
		{"active uses claimed color 172", domain.SecondaryActive, "38;5;172"},
		{"unplanned uses open color 071", domain.SecondaryUnplanned, "38;5;071"},
	}

	cs := iostreams.NewColorScheme(true)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			got := cmdutil.FormatState(cs, domain.StateOpen, tc.secondary)

			// Then
			if !strings.Contains(got, tc.code) {
				t.Errorf("FormatState secondary %s: expected ANSI code %q in %q", tc.secondary, tc.code, got)
			}
		})
	}
}

// --- Tabwriter alignment (ANSI byte-width normalization) ---

// TestFormatState_TabwriterAlignment_AllSecondaryStates verifies that formatted
// state strings produce correct visual alignment when rendered through
// text/tabwriter with color enabled. Different secondary states use different
// 256-color codes, which historically caused byte-width differences that
// misaligned the column following the state.
func TestFormatState_TabwriterAlignment_AllSecondaryStates(t *testing.T) {
	t.Parallel()

	// Given — all secondary states that can appear in list views.
	cs := iostreams.NewColorScheme(true)
	type row struct {
		primary   domain.State
		secondary domain.SecondaryState
	}
	rows := []row{
		{domain.StateOpen, domain.SecondaryReady},
		{domain.StateOpen, domain.SecondaryActive},
		{domain.StateOpen, domain.SecondaryBlocked},
		{domain.StateOpen, domain.SecondaryCompleted},
		{domain.StateOpen, domain.SecondaryUnplanned},
		{domain.StateClosed, domain.SecondaryNone},
		{domain.StateDeferred, domain.SecondaryNone},
		{domain.StateDeferred, domain.SecondaryBlocked},
	}

	// When — write all states through tabwriter (same format as list command).
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		stateCol := cmdutil.FormatState(cs, r.primary, r.secondary)
		_, _ = fmt.Fprintf(tw, "NP-12345\t%s\tP0\tSome title\n", stateCol)
	}
	_ = tw.Flush()

	// Then — the P0 column should start at the same visible position in every
	// line. Strip ANSI from the output and compare column offsets.
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("expected %d lines, got %d", len(rows), len(lines))
	}

	var expectedPos int
	for i, line := range lines {
		stripped := ansiRe.ReplaceAllString(line, "")
		pos := strings.Index(stripped, "P0")
		if pos < 0 {
			t.Fatalf("line %d missing P0: %q", i, stripped)
		}
		if i == 0 {
			expectedPos = pos
		} else if pos != expectedPos {
			t.Errorf("P0 misaligned on line %d: got position %d, want %d\n  line 0: %q\n  line %d: %q",
				i, pos, expectedPos,
				ansiRe.ReplaceAllString(lines[0], ""), i, stripped)
		}
	}
}
