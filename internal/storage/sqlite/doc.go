// Package sqlite implements the persistence port interfaces using an embedded
// SQLite database via zombiezen.com/go/sqlite (pure Go, no CGO). It provides
// database discovery (walking up from cwd to find .np/), schema management,
// and all CRUD operations with transactional guarantees.
//
// The Store uses a connection pool (sqlitex.Pool) with per-connection pragma
// setup for WAL mode, foreign keys, and a busy timeout. Write transactions
// use BEGIN IMMEDIATE so that lock contention is detected — and retried via
// the busy handler — at BEGIN rather than mid-transaction.
package sqlite
