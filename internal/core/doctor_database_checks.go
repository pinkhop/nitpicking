package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// npDirName is the name of the workspace directory that holds the database.
// Defined here to avoid importing the sqlite adapter for a single constant.
const npDirName = ".np"

// runDotNpDirectory checks that a .np/ directory exists somewhere in the
// ancestor chain of input.WorkDir. When WorkDir is empty it falls back to
// os.Getwd(). Returns nil on success; a finding when no .np/ is found.
func runDotNpDirectory(_ context.Context, _ *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error) {
	startDir := input.WorkDir
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return &doctorRunResult{
				Summary: fmt.Sprintf("could not determine working directory: %v", err),
			}, nil
		}
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return &doctorRunResult{
			Summary: fmt.Sprintf("could not resolve working directory %q: %v", startDir, err),
		}, nil
	}

	for {
		if _, statErr := os.Stat(filepath.Join(dir, npDirName)); statErr == nil {
			return nil, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return &doctorRunResult{
		Summary: fmt.Sprintf(
			"no %s/ directory found searching from %q to filesystem root — run 'np init' to create one",
			npDirName, startDir,
		),
	}, nil
}

// runDatabaseExists checks that the database file at input.DBPath exists,
// is non-empty, and could be opened by SQLite (indicated by a nil DBOpenError).
// Returns nil when the file is accessible; a finding otherwise.
func runDatabaseExists(_ context.Context, _ *serviceImpl, input driving.DoctorInput) (*doctorRunResult, error) {
	if input.DBPath == "" {
		return &doctorRunResult{
			Summary: "database path unknown — the .np/ directory discovery did not return a path",
		}, nil
	}

	info, statErr := os.Stat(input.DBPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return &doctorRunResult{
				Summary: fmt.Sprintf(
					"database file not found at %s — restore from backup or run 'np init'",
					input.DBPath,
				),
			}, nil
		}
		return &doctorRunResult{
			Summary: fmt.Sprintf("database file at %s is not accessible: %v", input.DBPath, statErr),
		}, nil
	}

	if info.Size() == 0 {
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"database file at %s has zero bytes — restore from backup or run 'np init'",
				input.DBPath,
			),
		}, nil
	}

	if input.DBOpenError != nil {
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"database file at %s exists but SQLite cannot open it: %v",
				input.DBPath, input.DBOpenError,
			),
		}, nil
	}

	return nil, nil
}

// runStorageIntegrity runs PRAGMA integrity_check and PRAGMA foreign_key_check
// against the open database. Either a non-ok integrity result or any FK
// violations produce an error finding. Returns nil when both pragma checks
// report no problems.
func runStorageIntegrity(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("storage-integrity: database connection unavailable (cascade should have protected this check)")
	}

	var integrityErr error
	var fkViolations int

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		db := uow.Database()

		var txErr error
		integrityErr = db.IntegrityCheck(ctx)
		fkViolations, txErr = db.ForeignKeyCheck(ctx)
		return txErr
	})
	if err != nil {
		return nil, fmt.Errorf("storage-integrity: %w", err)
	}

	if integrityErr != nil {
		return &doctorRunResult{
			Summary: fmt.Sprintf("PRAGMA integrity_check reported problems: %v", integrityErr),
		}, nil
	}

	if fkViolations > 0 {
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"PRAGMA foreign_key_check found %d referential-integrity violation(s)",
				fkViolations,
			),
		}, nil
	}

	return nil, nil
}

// runSchemaVersion reads the schema version from the metadata table and
// compares it to driven.CurrentSchemaVersion. Three outcomes:
//
//   - equal → pass (nil result)
//   - DB version < binary version → error finding without the newer-DB marker
//     (remaining checks still run on a best-effort basis)
//   - DB version > binary version → error finding with doctorSchemaNewerDBMarker,
//     causing the SkipsAll callback to skip all remaining checks including Environment
func runSchemaVersion(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("schema-version: database connection unavailable (cascade should have protected this check)")
	}

	var dbVersion int
	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		var txErr error
		dbVersion, txErr = uow.Database().GetSchemaVersion(ctx)
		return txErr
	})
	if err != nil {
		return nil, fmt.Errorf("schema-version: %w", err)
	}

	switch {
	case dbVersion == driven.CurrentSchemaVersion:
		return nil, nil

	case dbVersion < driven.CurrentSchemaVersion:
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"database schema is at v%d; binary expects v%d — run 'np admin upgrade' to migrate",
				dbVersion, driven.CurrentSchemaVersion,
			),
		}, nil

	default:
		// DB version > binary version: include the marker so SkipsAll fires and
		// every remaining check (including Environment) is skipped.
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"database schema is at v%d; binary only understands v%d — upgrade the np binary",
				dbVersion, driven.CurrentSchemaVersion,
			),
			Affected: []any{doctorSchemaNewerDBMarker{}},
		}, nil
	}
}

// runColumnDataValidity queries all typed columns for parse errors: timestamps
// must be parseable as datetimes, state and priority enum columns must hold
// only their allowed values, and history.changes must be well-formed JSON.
// Returns an error finding when any violations are detected.
func runColumnDataValidity(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("column-data-validity: database connection unavailable (cascade should have protected this check)")
	}

	var violations int
	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		var txErr error
		violations, txErr = uow.Database().ValidateColumnData(ctx)
		return txErr
	})
	if err != nil {
		return nil, fmt.Errorf("column-data-validity: %w", err)
	}

	if violations > 0 {
		return &doctorRunResult{
			Summary: fmt.Sprintf(
				"%d malformed value(s) found in typed columns (timestamps, state/priority enums, JSON blobs)",
				violations,
			),
		}, nil
	}

	return nil, nil
}
