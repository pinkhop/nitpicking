// Package main is the entry point for the np binary. It owns the full CLI
// lifecycle: constructing the dependency graph via wiring.NewCore, building the
// root command, executing it, classifying errors into exit codes, and calling
// os.Exit. This is the ONLY place os.Exit is called, ensuring that deferred
// cleanup functions throughout the application always execute.
package main

import (
	"context"
	"os"

	"github.com/pinkhop/nitpicking/internal/cmd/root"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/wiring"
)

func main() {
	os.Exit(int(run()))
}

// run assembles the dependency graph, builds and executes the CLI root command,
// and classifies the result into an exit code. Separated from main() so that
// every defer in the call tree executes before os.Exit terminates the process.
func run() cmdutil.ExitCode {
	f := wiring.NewCore(wiring.AppNameFromArgs(os.Args))

	rootCmd := root.NewRootCmd(f)

	err := rootCmd.Run(context.Background(), os.Args)
	if err == nil {
		return cmdutil.ExitOK
	}

	return cmdutil.ClassifyError(f.IOStreams.ErrOut, err, f.SignalCancelIsError)
}
