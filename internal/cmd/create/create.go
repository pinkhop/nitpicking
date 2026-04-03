package create

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/cmd/jsoncmd"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RunInput holds the parameters for the unified create command's core logic,
// decoupled from CLI flag parsing so it can be tested directly.
type RunInput struct {
	Service   driving.Service
	Author    string
	IOStreams *iostreams.IOStreams
	WriteTo   io.Writer

	// FormRunner presents the interactive form and populates data. In
	// production this runs the huh TUI; in tests it is replaced with a
	// function that sets fields directly. Only used in TTY mode.
	FormRunner func(data *formcmd.CreateFormData) error
}

// Run routes to the appropriate create implementation based on whether stdin
// is a TTY. When stdin is a pipe, it delegates to jsoncmd.RunCreate (JSON
// mode). When stdin is a TTY, it delegates to formcmd.RunFormCreate
// (interactive form mode).
func Run(ctx context.Context, input RunInput) error {
	if input.IOStreams.IsStdinTTY() {
		return formcmd.RunFormCreate(ctx, formcmd.RunFormCreateInput{
			Service:    input.Service,
			WriteTo:    input.WriteTo,
			FormRunner: input.FormRunner,
		})
	}

	return jsoncmd.RunCreate(ctx, jsoncmd.RunCreateInput{
		Service: input.Service,
		Author:  input.Author,
		Stdin:   input.IOStreams.In,
		WriteTo: input.WriteTo,
	})
}

// NewCmd constructs the root-level "create" command, which auto-detects its
// input mode: when stdin is a pipe, it reads a JSON object and creates the
// issue (matching "json create" behavior); when stdin is a TTY, it launches
// the interactive form (matching "form create" behavior).
//
// The --author flag is required for pipe mode (JSON). In TTY mode, the author
// is collected by the interactive form.
func NewCmd(f *cmdutil.Factory) *cli.Command {
	var author string

	return &cli.Command{
		Name:  "create",
		Usage: "Create an issue (auto-detects JSON pipe or interactive form)",
		Description: `Creates an issue using automatic input detection:

  - When stdin is a pipe, reads a JSON object from stdin (like "json create").
    The --author flag is required.
  - When stdin is a terminal, launches an interactive form (like "form create").
    The author is collected in the form.

Examples:
  echo '{"role":"task","title":"Fix bug"}' | np create --author alice
  np create    # launches interactive form`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "author",
				Aliases:     []string{"a"},
				Sources:     cli.EnvVars("NP_AUTHOR"),
				Usage:       "Author name (required for pipe mode; collected by form in TTY mode)",
				Category:    cmdutil.FlagCategoryRequired,
				Destination: &author,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			svc, err := cmdutil.NewTracker(f)
			if err != nil {
				return err
			}

			// In pipe mode, author is required on the command line.
			if !f.IOStreams.IsStdinTTY() && author == "" {
				return cli.Exit("--author is required when stdin is a pipe", 1)
			}

			return Run(ctx, RunInput{
				Service:    svc,
				Author:     author,
				IOStreams:  f.IOStreams,
				WriteTo:    f.IOStreams.Out,
				FormRunner: formcmd.DefaultFormRunner,
			})
		},
	}
}
