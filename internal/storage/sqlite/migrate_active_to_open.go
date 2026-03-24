package sqlite

import (
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// migrateActiveToOpen is a one-shot migration that changes all issues with
// state "active" to "open", unifying the epic and task state lifecycles.
// It is idempotent: if no issues have state "active", it is a no-op.
func migrateActiveToOpen(conn *sqlite.Conn) error {
	// Determine the current issues table name — could be "issues" (post
	// ticket→issue migration) or "tickets" (legacy, though that migration
	// runs first). Use "issues" since migrateTicketsToIssues runs before this.
	return sqlitex.Execute(conn,
		`UPDATE issues SET state = 'open' WHERE state = 'active'`,
		nil,
	)
}
