package iostreams

import (
	"bytes"
	"io"
	"os"

	"golang.org/x/term"
)

// IOStreams abstracts standard I/O with TTY awareness and color control.
// Commands read from In and write to Out (stdout) or ErrOut (stderr).
// In production, these are connected to real file descriptors; in tests,
// they are backed by in-memory buffers via Test().
type IOStreams struct {
	// In is the input reader, typically connected to os.Stdin.
	In io.ReadCloser

	// Out is the primary output writer, typically connected to os.Stdout.
	Out io.Writer

	// ErrOut is the error/diagnostic output writer, typically connected to os.Stderr.
	ErrOut io.Writer

	stdinIsTTY  bool
	stdoutIsTTY bool

	colorEnabled bool
}

// isTerminal reports whether the given file is connected to an interactive
// terminal. It uses golang.org/x/term, which calls GetConsoleMode on Windows
// and isatty on Unix — both giving correct results regardless of platform.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd())) // #nosec G115 -- file descriptors are small non-negative integers; this conversion is safe on all Go platforms
}

// System returns IOStreams connected to the real standard file descriptors
// with TTY detection and color support based on terminal capabilities.
// This is the production constructor — call it once at startup.
func System() *IOStreams {
	stdoutTTY := isTerminal(os.Stdout)

	return &IOStreams{
		In:           os.Stdin,
		Out:          os.Stdout,
		ErrOut:       os.Stderr,
		stdinIsTTY:   isTerminal(os.Stdin),
		stdoutIsTTY:  stdoutTTY,
		colorEnabled: stdoutTTY,
	}
}

// Test returns IOStreams backed by in-memory buffers, suitable for tests.
// The returned buffers let tests inspect what a command wrote to stdout
// and stderr without touching the real terminal. TTY flags default to false;
// use SetStdoutTTY and SetStdinTTY to simulate terminal behavior.
func Test() (streams *IOStreams, stdin *bytes.Buffer, stdout *bytes.Buffer, stderr *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &IOStreams{
		In:     io.NopCloser(in),
		Out:    out,
		ErrOut: errOut,
	}, in, out, errOut
}

// IsStdinTTY reports whether stdin is connected to an interactive terminal.
func (s *IOStreams) IsStdinTTY() bool {
	return s.stdinIsTTY
}

// IsStdoutTTY reports whether stdout is connected to an interactive terminal.
// Commands use this to decide between human-friendly output (colors, tables,
// relative timestamps) and machine-parseable output (plain text, absolute
// timestamps, tab-separated values).
func (s *IOStreams) IsStdoutTTY() bool {
	return s.stdoutIsTTY
}

// IsColorEnabled reports whether color output is enabled.
// Color is enabled by default when stdout is a TTY.
func (s *IOStreams) IsColorEnabled() bool {
	return s.colorEnabled
}

// CanPrompt reports whether the application can prompt the user for input.
// This requires both stdin and stdout to be TTYs — if either is piped,
// interactive prompts would hang or produce garbled output.
func (s *IOStreams) CanPrompt() bool {
	return s.stdinIsTTY && s.stdoutIsTTY
}

// SetStdinTTY overrides the stdin TTY flag. Used in tests to simulate
// terminal vs. piped input.
func (s *IOStreams) SetStdinTTY(isTTY bool) {
	s.stdinIsTTY = isTTY
}

// SetStdoutTTY overrides the stdout TTY flag and updates color enablement
// accordingly. Used in tests to simulate terminal vs. piped output.
func (s *IOStreams) SetStdoutTTY(isTTY bool) {
	s.stdoutIsTTY = isTTY
	s.colorEnabled = isTTY
}

// SetColorEnabled overrides the automatic color detection. Use this to
// force color on or off regardless of TTY status, for example when the
// user has set a NO_COLOR environment variable.
func (s *IOStreams) SetColorEnabled(enabled bool) {
	s.colorEnabled = enabled
}

// ColorScheme returns a ColorScheme configured for this IOStreams instance.
// When color is disabled, the scheme returns strings unmodified.
func (s *IOStreams) ColorScheme() *ColorScheme {
	return NewColorScheme(s.colorEnabled)
}
