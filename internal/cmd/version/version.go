package version

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// Options holds the dependencies and flag values for the version command.
type Options struct {
	// IO provides stdout/stderr writers for output.
	IO *iostreams.IOStreams

	// AppName is the application binary name, displayed in the version line.
	AppName string

	// Version is the application's build version string.
	Version string

	// OS is the operating system target (e.g., "linux", "darwin").
	// Injected from runtime.GOOS so tests can control it.
	OS string

	// Arch is the architecture target (e.g., "amd64", "arm64").
	// Injected from runtime.GOARCH so tests can control it.
	Arch string

	// BuildInfo holds VCS metadata (system name, commit, timestamp) embedded
	// by the Go toolchain at build time.
	BuildInfo cmdutil.BuildInfo

	// Verbose controls whether VCS build metadata is displayed.
	Verbose bool

	// JSON selects machine-readable JSON output instead of the default human-readable text.
	JSON bool
}

// versionOutput is the JSON representation of the version command output.
// Pointer fields marshal as null when VCS metadata is unavailable, signaling
// absence rather than omitting the keys entirely.
type versionOutput struct {
	Name    string  `json:"name"`
	Version string  `json:"version"`
	OS      string  `json:"os"`
	Arch    string  `json:"arch"`
	Commit  *string `json:"commit"`
	Dirty   *bool   `json:"dirty"`
	Built   *string `json:"built"`
}

// NewCmd constructs the "version" command. The optional runFn parameter allows
// tests to intercept execution and inspect the populated Options
// without running the command's business logic.
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, *Options) error) *cli.Command {
	opts := &Options{
		IO:        f.IOStreams,
		AppName:   f.AppName,
		Version:   f.AppVersion,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		BuildInfo: f.BuildInfo,
	}

	return &cli.Command{
		Name:  "version",
		Usage: "Print the application version",
		Description: `Prints the np binary version, OS, and architecture in the same format as
"go version". Use --verbose to include VCS build metadata (commit hash and
build timestamp) when available. Use --json for machine-readable output that
always includes all fields.

This command is useful for diagnosing version mismatches, verifying that a
build was produced from the expected commit, and including version
information in bug reports or agent logs.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "verbose",
				Usage:       "Include VCS build metadata (commit, timestamp)",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &opts.Verbose,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "Output machine-readable JSON instead of human-readable text",
				Category:    cmdutil.FlagCategorySupplemental,
				Destination: &opts.JSON,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if len(runFn) > 0 && runFn[0] != nil {
				return runFn[0](ctx, opts)
			}
			return run(ctx, opts)
		},
	}
}

// run dispatches to the appropriate output renderer based on the selected format.
func run(ctx context.Context, opts *Options) error {
	if opts.JSON {
		return runJSON(ctx, opts)
	}
	return runText(ctx, opts)
}

// runText prints the version in human-readable text format. The version line
// follows the `go version` convention: "<app> version <ver> <os>/<arch>".
// VCS metadata is only shown when --verbose is set.
func runText(_ context.Context, opts *Options) error {
	cs := opts.IO.ColorScheme()
	out := opts.IO.Out

	versionLine := fmt.Sprintf("%s version %s %s/%s", opts.AppName, opts.Version, opts.OS, opts.Arch)
	if _, err := fmt.Fprintln(out, cs.Bold(versionLine)); err != nil {
		return err
	}

	if !opts.Verbose {
		return nil
	}

	bi := opts.BuildInfo
	if bi.VCS == "" {
		return nil
	}

	if bi.Revision != "" {
		revision := bi.Revision
		if len(revision) > 12 {
			revision = revision[:12]
		}

		dirty := ""
		if bi.Dirty {
			dirty = " " + cs.Dim("(dirty)")
		}

		if _, err := fmt.Fprintf(out, "commit: %s%s\n", revision, dirty); err != nil {
			return err
		}
	}

	if bi.Time != "" {
		if _, err := fmt.Fprintf(out, "built:  %s\n", bi.Time); err != nil {
			return err
		}
	}

	return nil
}

// runJSON encodes the version information as indented JSON to stdout.
// JSON output always includes all VCS fields — --json implies --verbose.
// VCS fields are present as values when build metadata is available, or
// null when unavailable.
func runJSON(_ context.Context, opts *Options) error {
	out := opts.IO.Out
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")

	vo := versionOutput{
		Name:    opts.AppName,
		Version: opts.Version,
		OS:      opts.OS,
		Arch:    opts.Arch,
	}

	bi := opts.BuildInfo
	if bi.VCS != "" {
		if bi.Revision != "" {
			revision := bi.Revision
			if len(revision) > 12 {
				revision = revision[:12]
			}
			vo.Commit = &revision
		}
		vo.Dirty = &bi.Dirty
		if bi.Time != "" {
			parsed, parseErr := time.Parse(time.RFC3339, bi.Time)
			if parseErr == nil {
				formatted := cmdutil.FormatJSONTimestamp(parsed)
				vo.Built = &formatted
			} else {
				vo.Built = &bi.Time
			}
		}
	}

	return enc.Encode(vo)
}
