package sqlite

import (
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// migrateNotesToComments is a one-shot migration that renames the "notes"
// table to "comments", the "note_id" column to "comment_id", and rebuilds
// the associated FTS index. It is idempotent: if the "notes" table does not
// exist, it is a no-op.
func migrateNotesToComments(conn *sqlite.Conn) error {
	hasOldTable, err := tableExists(conn, "notes")
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
		`ALTER TABLE notes RENAME TO comments`,
		`ALTER TABLE comments RENAME COLUMN note_id TO comment_id`,

		// Drop and recreate the index with the new name.
		`DROP INDEX IF EXISTS idx_notes_issue`,
		`CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id)`,

		// Rebuild FTS: virtual tables cannot be altered.
		`DROP TABLE IF EXISTS notes_fts`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS comments_fts USING fts5(comment_id, body)`,
		`INSERT INTO comments_fts (comment_id, body) SELECT comment_id, body FROM comments`,
	}

	for _, ddl := range steps {
		if err := sqlitex.Execute(conn, ddl, nil); err != nil {
			return err
		}
	}

	return nil
}
