// Package memory provides a first-class in-memory implementation of the
// persistence port interfaces. The repository stores all data in maps and
// slices, supports the same pagination, filtering, and ordering semantics as
// the SQLite adapter, and is safe for concurrent use.
//
// This adapter is not a mock or test-only fake — it is a fully functional,
// behaviorally correct implementation of the driven port contract. Its
// existence keeps the port abstraction honest: every driven port method must
// be implementable without importing adapter-specific types (SQLite handles,
// query builders, etc.). See docs/developer/architecture/architecture.md for
// the full rationale.
//
// Primary uses:
//   - Unit-testing the core without database I/O.
//   - Validating that the driven port interface is a real abstraction, not a
//     thin wrapper over the SQLite adapter.
package memory
