package version_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/version"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// ---------------------------------------------------------------------------
// Text output — no verbose
// ---------------------------------------------------------------------------

func TestRun_NoBuildInfo_PrintsVersionLineOnly(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "dev",
		OS:      "linux",
		Arch:    "amd64",
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "np version dev linux/amd64\n"
	if got := stdout.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestRun_WithBuildInfo_NoVerbose_PrintsVersionLineOnly(t *testing.T) {
	t.Parallel()

	// Given — VCS info is present but --verbose is not set
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "darwin",
		Arch:    "arm64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Time:     "2025-06-15T10:30:00Z",
		},
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	want := "np version 1.0.0 darwin/arm64\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// Text output — verbose
// ---------------------------------------------------------------------------

func TestRun_Verbose_WithBuildInfo_NonTTY_PrintsPlainOutput(t *testing.T) {
	t.Parallel()

	// Given — stdout is not a TTY (default for iostreams.Test)
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Time:     "2025-06-15T10:30:00Z",
		},
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	wantLines := []string{
		"np version 1.0.0 linux/amd64",
		"commit: abc123def456",
		"built:  2025-06-15T10:30:00Z",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, got)
		}
	}

	// Non-TTY output must not contain ANSI escape sequences.
	if strings.Contains(got, "\033[") {
		t.Errorf("non-TTY output contains ANSI codes:\n%s", got)
	}
}

func TestRun_Verbose_WithBuildInfo_TTY_PrintsBoldVersion(t *testing.T) {
	t.Parallel()

	// Given — stdout is a TTY with color enabled
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "2.5.0",
		OS:      "darwin",
		Arch:    "arm64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "deadbeef1234567",
			Time:     "2025-12-01T08:00:00Z",
		},
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()

	// Version line should be bold (\033[1m...\033[0m).
	if !strings.Contains(got, "\033[1mnp version 2.5.0 darwin/arm64\033[0m") {
		t.Errorf("expected bold version line in TTY output, got:\n%s", got)
	}

	// Labels should be plain text — no ANSI decoration.
	if strings.Contains(got, "\033[") && strings.Contains(got, "commit:") {
		// Extract just the commit line to check it has no escape around the label.
		for _, line := range strings.Split(got, "\n") {
			if strings.Contains(line, "commit:") && strings.HasPrefix(strings.TrimSpace(line), "\033[") {
				t.Errorf("commit label should be plain text, got: %q", line)
			}
		}
	}
	if !strings.Contains(got, "commit: deadbeef1234") {
		t.Errorf("expected plain commit label in output, got:\n%s", got)
	}
	if !strings.Contains(got, "built:  2025-12-01T08:00:00Z") {
		t.Errorf("expected plain built label in output, got:\n%s", got)
	}
}

func TestRun_Verbose_DirtyBuild_TTY_ShowsYellowDirtyMarker(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Dirty:    true,
		},
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()

	// "(dirty)" should be dim in TTY mode.
	if !strings.Contains(got, "\033[2m(dirty)\033[0m") {
		t.Errorf("expected dim dirty marker in TTY output, got:\n%s", got)
	}
}

func TestRun_Verbose_DirtyBuild_NonTTY_ShowsPlainDirtyMarker(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Dirty:    true,
		},
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "(dirty)") {
		t.Errorf("expected dirty marker in output, got:\n%s", got)
	}
}

func TestRun_Verbose_ShortRevision_NoTruncation(t *testing.T) {
	t.Parallel()

	// Given — revision shorter than 12 characters
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123",
		},
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "abc123") {
		t.Errorf("expected full short revision, got:\n%s", stdout.String())
	}
}

func TestRun_Verbose_NoBuildInfo_PrintsVersionLineOnly(t *testing.T) {
	t.Parallel()

	// Given — verbose is set but no VCS info available
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "dev",
		OS:      "linux",
		Arch:    "amd64",
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "np version dev linux/amd64\n"
	if got := stdout.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// JSON output
// ---------------------------------------------------------------------------

func TestRun_JSON_NoVerboseFlag_AlwaysIncludesVCSKeys(t *testing.T) {
	t.Parallel()

	// Given — --json is set but --verbose is not; JSON implies verbose so
	// VCS keys must be present (as null when no build info is available).
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.2.3",
		OS:      "linux",
		Arch:    "amd64",
		JSON:    true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	if got["version"] != "1.2.3" {
		t.Errorf("expected version %q, got %v", "1.2.3", got["version"])
	}
	if got["os"] != "linux" {
		t.Errorf("expected os %q, got %v", "linux", got["os"])
	}
	if got["arch"] != "amd64" {
		t.Errorf("expected arch %q, got %v", "amd64", got["arch"])
	}
	if got["name"] != "np" {
		t.Errorf("expected name %q, got %v", "np", got["name"])
	}

	// --json implies verbose: VCS keys must always be present, null when unavailable.
	for _, key := range []string{"commit", "dirty", "built"} {
		if _, exists := got[key]; !exists {
			t.Errorf("JSON output missing key %q (--json implies verbose)", key)
		}
	}
}

func TestRun_FormatJSON_Verbose_WithBuildInfo_PrintsFullJSON(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "darwin",
		Arch:    "arm64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Time:     "2025-06-15T10:30:00Z",
		},
		JSON:    true,
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	if got["name"] != "np" {
		t.Errorf("expected name %q, got %v", "np", got["name"])
	}
	if got["version"] != "1.0.0" {
		t.Errorf("expected version %q, got %v", "1.0.0", got["version"])
	}
	if got["commit"] != "abc123def456" {
		t.Errorf("expected commit %q, got %v", "abc123def456", got["commit"])
	}
	if got["built"] != "2025-06-15T10:30:00Z" {
		t.Errorf("expected built %q, got %v", "2025-06-15T10:30:00Z", got["built"])
	}

	// dirty should be a boolean false, not absent.
	dirty, exists := got["dirty"]
	if !exists {
		t.Fatal("expected dirty key in verbose JSON")
	}
	if dirty != false {
		t.Errorf("expected dirty false, got %v", dirty)
	}
}

func TestRun_FormatJSON_Verbose_NoBuildInfo_PrintsNullVCSFields(t *testing.T) {
	t.Parallel()

	// Given — verbose with no VCS info
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "dev",
		OS:      "linux",
		Arch:    "amd64",
		JSON:    true,
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	// VCS keys should be present but null.
	for _, key := range []string{"commit", "dirty", "built"} {
		val, exists := got[key]
		if !exists {
			t.Errorf("verbose JSON should contain key %q", key)
		}
		if val != nil {
			t.Errorf("expected %q to be null, got %v", key, val)
		}
	}
}

func TestRun_FormatJSON_Verbose_DirtyBuild_DirtyIsTrue(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	opts := &version.Options{
		IO:      ios,
		AppName: "np",
		Version: "1.0.0",
		OS:      "linux",
		Arch:    "amd64",
		BuildInfo: cmdutil.BuildInfo{
			VCS:      "git",
			Revision: "abc123def456789",
			Dirty:    true,
		},
		JSON:    true,
		Verbose: true,
	}

	// When
	err := version.ExportRun(context.Background(), opts)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, stdout.String())
	}

	// dirty should be a boolean true, separate from the commit string.
	dirty, exists := got["dirty"]
	if !exists {
		t.Fatal("expected dirty key in verbose JSON")
	}
	if dirty != true {
		t.Errorf("expected dirty true, got %v", dirty)
	}

	// commit should not contain "(dirty)".
	commit, _ := got["commit"].(string)
	if strings.Contains(commit, "dirty") {
		t.Errorf("commit should not contain dirty marker, got %q", commit)
	}
}

// ---------------------------------------------------------------------------
// Flag parsing
// ---------------------------------------------------------------------------

func TestFlagParsing_VerboseFlag_SetsVerboseTrue(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppName:    "np",
		AppVersion: "1.0.0",
		IOStreams:  ios,
	}

	var captured *version.Options
	cmd := version.NewCmd(f, func(_ context.Context, opts *version.Options) error {
		captured = opts
		return nil
	})

	// When
	err := cmd.Run(context.Background(), []string{"version", "--verbose"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("runFn was not called")
	}
	if !captured.Verbose {
		t.Error("expected Verbose to be true")
	}
}

func TestFlagParsing_JSONFlag_SetsJSONTrue(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppName:    "np",
		AppVersion: "1.0.0",
		IOStreams:  ios,
	}

	var captured *version.Options
	cmd := version.NewCmd(f, func(_ context.Context, opts *version.Options) error {
		captured = opts
		return nil
	})

	// When
	err := cmd.Run(context.Background(), []string{"version", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("runFn was not called")
	}
	if !captured.JSON {
		t.Error("expected JSON to be true")
	}
}

func TestFlagParsing_JSONFlag_DefaultIsFalse(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppName:    "np",
		AppVersion: "1.0.0",
		IOStreams:  ios,
	}

	var captured *version.Options
	cmd := version.NewCmd(f, func(_ context.Context, opts *version.Options) error {
		captured = opts
		return nil
	})

	// When
	err := cmd.Run(context.Background(), []string{"version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("runFn was not called")
	}
	if captured.JSON {
		t.Error("expected JSON to be false by default")
	}
}

func TestFlagParsing_InvokesRunFn(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		AppName:    "np",
		AppVersion: "1.0.0",
		IOStreams:  ios,
	}

	var called bool
	cmd := version.NewCmd(f, func(_ context.Context, opts *version.Options) error {
		called = true
		if opts.Version != "1.0.0" {
			t.Errorf("expected version %q, got %q", "1.0.0", opts.Version)
		}
		if opts.AppName != "np" {
			t.Errorf("expected app name %q, got %q", "np", opts.AppName)
		}
		if opts.OS == "" {
			t.Error("expected OS to be populated")
		}
		if opts.Arch == "" {
			t.Error("expected Arch to be populated")
		}
		return nil
	})

	// When
	err := cmd.Run(context.Background(), []string{"version"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected runFn to be called")
	}
}
