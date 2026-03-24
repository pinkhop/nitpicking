package sqlite

import (
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// migrateWaitingToDeferred is a one-shot migration that changes all issues
// with state "waiting" to "deferred", since the waiting state has been
// removed from the state machine. It is idempotent.
func migrateWaitingToDeferred(conn *sqlite.Conn) error {
	return sqlitex.Execute(conn,
		`UPDATE issues SET state = 'deferred' WHERE state = 'waiting'`,
		nil,
	)
}
