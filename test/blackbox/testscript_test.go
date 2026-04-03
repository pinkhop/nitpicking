//go:build blackbox

package blackbox_test

import (
	"context"
	"os"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/root"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/wiring"
	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain registers the np binary as a testscript command and runs all tests.
// When the test binary is re-invoked by testscript as "np", it dispatches
// through the same path as the real binary: wiring.NewCore assembles the
// dependency graph, root.NewRootCmd builds the CLI, and ClassifyError maps
// errors to exit codes. When invoked normally by go test, it runs m.Run().
func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"np": func() int {
			f := wiring.NewCore(wiring.AppNameFromArgs(os.Args))
			rootCmd := root.NewRootCmd(f)
			err := rootCmd.Run(context.Background(), os.Args)
			if err == nil {
				return int(cmdutil.ExitOK)
			}
			return int(cmdutil.ClassifyError(f.IOStreams.ErrOut, err, f.SignalCancelIsError))
		},
	}))
}

// TestBlackbox_Script runs all .txtar testscript files found in testdata/.
// Each .txtar file is an independent blackbox component test that exercises np
// commands through the testscript framework.
func TestBlackbox_Script(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata",
		RequireExplicitExec: true,
		Cmds:                testscriptCmds(),
	})
}
