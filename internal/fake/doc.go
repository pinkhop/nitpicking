// Package fake provides in-memory implementations of the persistence port
// interfaces for use in unit tests. The fake repository stores all data in
// maps and slices, supports pagination and filtering, and provides the same
// transactional guarantees as the real SQLite adapter.
package fake
