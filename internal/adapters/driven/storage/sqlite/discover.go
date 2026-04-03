package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
)

// npDirName is the name of the directory that holds the database.
const npDirName = ".np"

// dbFileName is the name of the SQLite database file within .np/.
const dbFileName = "nitpicking.db"

// DiscoverDatabase walks up from startDir looking for a .np/ directory.
// Returns the full path to the database file, or an error if not found.
//
// Permission and sandbox errors are silently ignored per §3.3 — if a
// directory cannot be read, it is skipped.
func DiscoverDatabase(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolving start directory: %w", err)
	}

	for {
		npPath := filepath.Join(dir, npDirName)

		_, err := os.Stat(npPath)
		if err == nil {
			return filepath.Join(npPath, dbFileName), nil
		}

		// All stat errors (not-found, permission denied, kernel sandbox,
		// etc.) are treated as "not present in this directory" — traversal
		// continues upward regardless of the error kind.

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s directory found (searched from %s to filesystem root)", npDirName, startDir)
		}
		dir = parent
	}
}

// LookupDatabase checks a single directory for a .np/ workspace without
// walking up to parent directories. Returns the full path to the database
// file, or an error if the directory does not contain an np workspace. Use
// this when the caller has specified an explicit workspace directory.
func LookupDatabase(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving workspace directory: %w", err)
	}

	npPath := filepath.Join(absDir, npDirName)
	if _, err := os.Stat(npPath); err != nil {
		return "", fmt.Errorf("no %s directory found in workspace %s", npDirName, absDir)
	}

	return filepath.Join(npPath, dbFileName), nil
}

// InitDatabaseDir creates the .np/ directory and returns the database path.
func InitDatabaseDir(baseDir string) (string, error) {
	npPath := filepath.Join(baseDir, npDirName)

	if err := os.MkdirAll(npPath, 0o750); err != nil {
		return "", fmt.Errorf("creating %s directory: %w", npDirName, err)
	}

	return filepath.Join(npPath, dbFileName), nil
}
