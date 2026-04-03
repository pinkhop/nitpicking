// Package core implements the driving port interface defined in
// internal/ports/driving. It contains the business logic that orchestrates
// domain operations through the driven port (Transactor) without knowledge of
// the specific storage or transport adapters in use.
//
// Driving adapters (CLI commands, HTTP handlers) depend on the driving port
// interface, not on this package directly. The configurator (internal/wiring)
// wires core to the driven adapter at startup.
package core
