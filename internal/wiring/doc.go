// Package wiring is the application's configurator layer — the outermost ring
// of the hexagonal architecture. It constructs the Factory and wires all
// dependencies (driven adapters, IOStreams, build metadata) into it. The
// assembled Factory is returned to the caller via NewCore; the configurator
// does not build CLI commands, execute them, or classify errors — those
// executable-specific concerns belong to the binary entry point (e.g.,
// cmd/np/main.go).
package wiring
