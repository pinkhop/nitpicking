package sqlite

import (
	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// migrateFacetsToDimensions is a one-shot migration that renames the "facets"
// table to "dimensions". It is idempotent: if the "facets" table does not
// exist, it is a no-op.
func migrateFacetsToDimensions(conn *sqlite.Conn) error {
	hasOldTable, err := tableExists(conn, "facets")
	if err != nil {
		return err
	}
	if !hasOldTable {
		return nil
	}

	return sqlitex.Execute(conn, `ALTER TABLE facets RENAME TO dimensions`, nil)
}
