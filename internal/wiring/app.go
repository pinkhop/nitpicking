package wiring

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/sqlite"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
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
	// When an existing database is found, Open is used (no DDL). When no
	// database exists, Create is used to apply the schema — this only
	// happens during "np init".
	var store *sqlite.Store
	f.Store = func() (*sqlite.Store, error) {
		if store != nil {
			return store, nil
		}

		dbPath, err := f.DatabasePath()
		if err != nil {
			if f.Workspace != "" {
				// Explicit workspace must exist — do not auto-create.
				return nil, err
			}

			// No database found — create the .np/ directory and a new
			// database with schema so that init can populate it.
			cwd, _ := os.Getwd()
			dbPath, err = sqlite.InitDatabaseDir(cwd)
			if err != nil {
				return nil, err
			}

			store, err = sqlite.Create(dbPath)
			if err != nil {
				return nil, err
			}
			return store, nil
		}

		store, err = sqlite.Open(dbPath)
		if err != nil {
			return nil, err
		}
		return store, nil
	}

	return f
}
