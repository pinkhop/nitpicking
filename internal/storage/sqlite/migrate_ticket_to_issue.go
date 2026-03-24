package sqlite

import (
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// migrateTicketsToIssues is a one-shot migration that renames the "tickets"
// table and all "ticket_id" columns to "issues" / "issue_id". It is idempotent:
// if the "tickets" table does not exist, it is a no-op.
//
// The migration also rebuilds the FTS index (virtual tables cannot be altered)
// and recreates all indexes under their new names.
func migrateTicketsToIssues(conn *sqlite.Conn) error {
	// Guard: skip if the old table no longer exists.
	hasOldTable, err := tableExists(conn, "tickets")
	if err != nil {
		return err
	}
	if !hasOldTable {
		return nil
	}

	endFn, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return err
	}
	defer endFn(&err)

	steps := []string{
		// Rename the primary table and its PK column.
		`ALTER TABLE tickets RENAME TO issues`,
		`ALTER TABLE issues RENAME COLUMN ticket_id TO issue_id`,

		// Rename the FK column in child tables.
		`ALTER TABLE facets RENAME COLUMN ticket_id TO issue_id`,
		`ALTER TABLE notes RENAME COLUMN ticket_id TO issue_id`,
		`ALTER TABLE claims RENAME COLUMN ticket_id TO issue_id`,
		`ALTER TABLE history RENAME COLUMN ticket_id TO issue_id`,

		// Drop old indexes and create new ones.
		`DROP INDEX IF EXISTS idx_tickets_parent`,
		`DROP INDEX IF EXISTS idx_tickets_state`,
		`DROP INDEX IF EXISTS idx_tickets_priority_created`,
		`DROP INDEX IF EXISTS idx_tickets_idempotency`,
		`DROP INDEX IF EXISTS idx_notes_ticket`,
		`DROP INDEX IF EXISTS idx_claims_ticket`,
		`DROP INDEX IF EXISTS idx_history_ticket`,

		`CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues(parent_id) WHERE parent_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_issues_state ON issues(state) WHERE deleted = 0`,
		`CREATE INDEX IF NOT EXISTS idx_issues_priority_created ON issues(priority, created_at) WHERE deleted = 0`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_issues_idempotency ON issues(idempotency_key) WHERE idempotency_key IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_notes_issue ON notes(issue_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_issue ON claims(issue_id)`,
		`CREATE INDEX IF NOT EXISTS idx_history_issue ON history(issue_id, revision)`,

		// Rebuild FTS: virtual tables cannot be altered, so drop and recreate.
		`DROP TABLE IF EXISTS tickets_fts`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS issues_fts USING fts5(issue_id, title, description, acceptance_criteria)`,
		`INSERT INTO issues_fts (issue_id, title, description, acceptance_criteria) SELECT issue_id, title, description, acceptance_criteria FROM issues WHERE deleted = 0`,
	}

	for _, ddl := range steps {
		if err := sqlitex.Execute(conn, ddl, nil); err != nil {
			return err
		}
	}

	return nil
}

// tableExists reports whether a table with the given name exists in the database.
func tableExists(conn *sqlite.Conn, name string) (bool, error) {
	var found bool
	err := sqlitex.Execute(conn, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?`, &sqlitex.ExecOptions{
		Args: []any{name},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = stmt.ColumnInt(0) == 1
			return nil
		},
	})
	return found, err
}
