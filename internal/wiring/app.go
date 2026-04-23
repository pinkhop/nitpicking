package wiring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
)

// AppNameFromArgs derives the application name from the executable path in
// args (typically os.Args). It strips the directory component and, on Windows,
// the .exe extension, so the name matches the binary filename regardless of
// how it was invoked. Falls back to "app" if args is empty, which indicates
// an abnormal process launch.
func AppNameFromArgs(args []string) string {
	if len(args) == 0 {
		return "app"
	}
	// filepath.Base is OS-aware: on Unix it treats only / as a separator;
	// on Windows it treats both / and \ as separators. This means the function
	// is correct for any path produced by the current platform's os.Args[0].
	name := filepath.Base(args[0])
	name = strings.TrimSuffix(name, ".exe")
	return name
}

// NewCore assembles the application's dependency graph and returns the wired
// Factory. This is the configurator's only job — it constructs infrastructure
// (IOStreams, Store, BuildInfo) but does not build CLI commands, execute them,
// or classify errors. Those responsibilities belong to the executable entry
// point (e.g., cmd/np/main.go).
//
// The appName parameter identifies the binary (typically derived from
// os.Args[0] via AppNameFromArgs). The version is injected at build time via
// ldflags into the package-level variable and used automatically.
func NewCore(appName string) *cmdutil.Factory {
	f := &cmdutil.Factory{
		AppName:    appName,
		AppVersion: version,
		BuildInfo:  cmdutil.ReadBuildInfo(),
	}

	// Phase 1: IOStreams — constructed eagerly because it is cheap and
	// needed by virtually every command.
	f.IOStreams = iostreams.System()

	// Phase 2: DatabasePath — workspace-aware database path resolution.
	// When --workspace is specified, it checks that directory directly;
	// otherwise it walks up from the current working directory.
	f.DatabasePath = func() (string, error) {
		if f.Workspace != "" {
			return sqlite.LookupDatabase(f.Workspace)
		}
		cwd, _ := os.Getwd()
		return sqlite.DiscoverDatabase(cwd)
	}

	// Phase 3: Store — the SQLite database connection, constructed lazily.
	// Database discovery runs once on first access; the Store is memoized.
	//
	// Store never creates the .np/ directory or the database file — that is
	// exclusively the responsibility of the init command. When no database is
	// found it returns an error so that the command can surface a user-friendly
	// message. The only exception is when no .np/ directory exists at all and
	// no explicit workspace was provided: in that case the directory and an
	// empty-schema database are created, which is what np init relies on for its
	// first-run flow before the user has any workspace.
	var store *sqlite.Store
	f.Store = func() (*sqlite.Store, error) {
		if store != nil {
			return store, nil
		}

		dbPath, pathErr := f.DatabasePath()
		if pathErr != nil {
			if f.Workspace != "" {
				// Explicit workspace must exist — do not auto-create.
				return nil, pathErr
			}

			// No .np/ directory found — create the directory and a new
			// database with schema so that np init can populate it.
			cwd, _ := os.Getwd()
			var createErr error
			dbPath, createErr = sqlite.InitDatabaseDir(cwd)
			if createErr != nil {
				return nil, createErr
			}

			store, createErr = sqlite.Create(dbPath)
			if createErr != nil {
				return nil, createErr
			}
			return store, nil
		}

		// DatabasePath succeeded, meaning .np/ exists. When the database file
		// itself is absent the workspace has not been initialized yet — return
		// ErrDatabaseNotInitialized so commands can guide the user to np init
		// without silently creating a database as a side effect.
		if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("run 'np init' to initialize the workspace: %w",
				domain.ErrDatabaseNotInitialized)
		}

		var openErr error
		store, openErr = sqlite.Open(dbPath)
		if openErr != nil {
			return nil, openErr
		}
		return store, nil
	}

	return f
}
