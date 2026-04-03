// Package cmdutil provides shared infrastructure for command implementations:
// the Factory for dependency injection, typed errors for centralized
// classification, build metadata extraction, and flag helpers. Command packages
// import cmdutil to access these building blocks; cmdutil itself has no
// knowledge of individual commands.
package cmdutil
