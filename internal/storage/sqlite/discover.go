package sqlite

import (
	"errors"
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

		// Skip permission/sandbox errors silently.
		if !errors.Is(err, os.ErrNotExist) && !os.IsPermission(err) {
			// Other errors — skip silently too (kernel sandbox, etc.).
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s directory found (searched from %s to filesystem root)", npDirName, startDir)
		}
		dir = parent
	}
}

// InitDatabaseDir creates the .np/ directory and returns the database path.
func InitDatabaseDir(baseDir string) (string, error) {
	npPath := filepath.Join(baseDir, npDirName)

	if err := os.MkdirAll(npPath, 0o755); err != nil {
		return "", fmt.Errorf("creating %s directory: %w", npDirName, err)
	}

	return filepath.Join(npPath, dbFileName), nil
}
