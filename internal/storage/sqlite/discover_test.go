package sqlite_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

func TestDiscoverDatabase_FindsInCurrentDir(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".np"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// When
	dbPath, err := sqlite.DiscoverDatabase(dir)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(dbPath, ".np/nitpicking.db") {
		t.Errorf("expected .np/nitpicking.db, got %s", dbPath)
	}
}

func TestDiscoverDatabase_FindsInParentDir(t *testing.T) {
	t.Parallel()

	// Given
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(parent, ".np"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// When
	dbPath, err := sqlite.DiscoverDatabase(child)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dbPath, parent) {
		t.Errorf("expected path in parent, got %s", dbPath)
	}
}

func TestDiscoverDatabase_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()

	// When
	_, err := sqlite.DiscoverDatabase(dir)

	// Then
	if err == nil {
		t.Fatal("expected error when .np not found")
	}
}

func TestInitDatabaseDir_CreatesDirectory(t *testing.T) {
	t.Parallel()

	// Given
	dir := t.TempDir()

	// When
	dbPath, err := sqlite.InitDatabaseDir(dir)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(dbPath, ".np/nitpicking.db") {
		t.Errorf("unexpected path: %s", dbPath)
	}

	// Verify directory exists.
	info, err := os.Stat(filepath.Join(dir, ".np"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
