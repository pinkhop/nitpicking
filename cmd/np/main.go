// Package main is the thin entry point for the application.
// It calls app.Main() — which constructs all dependencies, runs the root
// command, and classifies errors — then passes the resulting exit code to
// os.Exit. This is the ONLY place os.Exit is called, ensuring that deferred
// cleanup functions throughout the application always execute.
package main

import (
	"os"

	"github.com/pinkhop/nitpicking/internal/app"
)

func main() {
	os.Exit(int(app.Main()))
}
