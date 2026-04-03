// Package driving defines the driving port — the use-case boundary that CLI
// and other adapters invoke. The Service interface declares every operation
// the application supports; the DTO types carry data across that boundary.
//
// Driving adapters (CLI commands, future HTTP handlers, etc.) depend on this
// package for the interface and its input/output types. The core implementation
// lives in a separate package and is wired to the interface by the configurator.
package driving
