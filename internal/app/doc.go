// Package app is the application's entry point, called from cmd/np/main.go.
// It constructs the Factory, builds the root command, runs it, and classifies
// any returned error into a typed exit code. All error-to-exit-code mapping
// is centralized here so that individual commands never call os.Exit directly.
package app
