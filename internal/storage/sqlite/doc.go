// Package sqlite implements the persistence port interfaces using an embedded
// SQLite database via modernc.org/sqlite (pure Go, no CGO). It provides
// database discovery (walking up from cwd to find .np/), schema management,
// and all CRUD operations with transactional guarantees.
package sqlite
