package core_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// fakeMigrator is a test double that implements driven.Migrator. Each method
// delegates to a function-type field so individual tests can control behaviour
// without building a new type per test.
type fakeMigrator struct {
	// checkSchemaVersionFn is called by CheckSchemaVersion. If nil, the method
	// returns nil (no migration required).
	checkSchemaVersionFn func(ctx context.Context) error

	// migrateV1ToV2Fn is called by MigrateV1ToV2. If nil, the method returns
	// an empty MigrationResult and nil error.
	migrateV1ToV2Fn func(ctx context.Context) (driven.MigrationResult, error)

	// migrateV2ToV3Fn is called by MigrateV2ToV3. If nil, the method returns
	// an empty MigrationResult and nil error.
	migrateV2ToV3Fn func(ctx context.Context) (driven.MigrationResult, error)
}

// CheckSchemaVersion delegates to checkSchemaVersionFn if set, otherwise
// returns nil.
func (f *fakeMigrator) CheckSchemaVersion(ctx context.Context) error {
	if f.checkSchemaVersionFn != nil {
		return f.checkSchemaVersionFn(ctx)
	}
	return nil
}

// MigrateV1ToV2 delegates to migrateV1ToV2Fn if set, otherwise returns zero
// MigrationResult and nil error.
func (f *fakeMigrator) MigrateV1ToV2(ctx context.Context) (driven.MigrationResult, error) {
	if f.migrateV1ToV2Fn != nil {
		return f.migrateV1ToV2Fn(ctx)
	}
	return driven.MigrationResult{}, nil
}

// MigrateV2ToV3 delegates to migrateV2ToV3Fn if set, otherwise returns zero
// MigrationResult and nil error.
func (f *fakeMigrator) MigrateV2ToV3(ctx context.Context) (driven.MigrationResult, error) {
	if f.migrateV2ToV3Fn != nil {
		return f.migrateV2ToV3Fn(ctx)
	}
	return driven.MigrationResult{}, nil
}

// setupServiceWithMigrator creates a core service wired with the given
// fakeMigrator and an in-memory transactor. The service is initialised with a
// "TEST" prefix. Callers must not use this helper when testing migration guard
// behaviour because Init requires the store to be ready — use core.New directly
// with nil migrator or an uninitialised transactor instead.
func setupServiceWithMigrator(t *testing.T, m *fakeMigrator) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, m)
	if err := svc.Init(t.Context(), "TEST"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// --- MigrateV2ToV3: errNoMigrator guard ---

// TestMigrateV2ToV3_NoMigrator_ReturnsError verifies that calling MigrateV2ToV3
// on a service constructed without a Migrator returns an error rather than
// silently succeeding. This mirrors the same guard on MigrateV1ToV2.
func TestMigrateV2ToV3_NoMigrator_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a service with no migrator (nil).
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	// When — MigrateV2ToV3 is called.
	_, err := svc.MigrateV2ToV3(t.Context())

	// Then — an error is returned.
	if err == nil {
		t.Fatal("expected error from MigrateV2ToV3 with nil migrator, got nil")
	}
}

// --- MigrateV2ToV3: successful delegation ---

// TestMigrateV2ToV3_WithMigrator_DelegatesAndMapsCounts verifies that
// MigrateV2ToV3 delegates to the Migrator port and maps all three
// v2→v3-specific counters into the driving DTO without loss.
func TestMigrateV2ToV3_WithMigrator_DelegatesAndMapsCounts(t *testing.T) {
	t.Parallel()

	// Given — a fake migrator that returns known non-zero counts.
	m := &fakeMigrator{
		migrateV2ToV3Fn: func(_ context.Context) (driven.MigrationResult, error) {
			return driven.MigrationResult{
				IdempotencyKeysMigrated:   5,
				IdempotencyKeysSkipped:    2,
				InvalidLabelValuesSkipped: 1,
			}, nil
		},
	}
	svc := setupServiceWithMigrator(t, m)

	// When — MigrateV2ToV3 is called.
	result, err := svc.MigrateV2ToV3(t.Context())
	// Then — no error and all three counters are mapped correctly.
	if err != nil {
		t.Fatalf("unexpected error from MigrateV2ToV3: %v", err)
	}
	if result.IdempotencyKeysMigrated != 5 {
		t.Errorf("IdempotencyKeysMigrated: got %d, want 5", result.IdempotencyKeysMigrated)
	}
	if result.IdempotencyKeysSkipped != 2 {
		t.Errorf("IdempotencyKeysSkipped: got %d, want 2", result.IdempotencyKeysSkipped)
	}
	if result.InvalidLabelValuesSkipped != 1 {
		t.Errorf("InvalidLabelValuesSkipped: got %d, want 1", result.InvalidLabelValuesSkipped)
	}
}

// TestMigrateV2ToV3_MigratorReturnsError_PropagatesError verifies that if the
// Migrator port returns an error, MigrateV2ToV3 propagates it to the caller
// without modification.
func TestMigrateV2ToV3_MigratorReturnsError_PropagatesError(t *testing.T) {
	t.Parallel()

	// Given — a fake migrator that fails.
	sentinel := &domain.DatabaseError{Op: "migrate v2→v3", Err: errors.New("disk full")}
	m := &fakeMigrator{
		migrateV2ToV3Fn: func(_ context.Context) (driven.MigrationResult, error) {
			return driven.MigrationResult{}, sentinel
		},
	}
	svc := setupServiceWithMigrator(t, m)

	// When — MigrateV2ToV3 is called.
	_, err := svc.MigrateV2ToV3(t.Context())

	// Then — the error from the migrator is returned.
	if err == nil {
		t.Fatal("expected error from MigrateV2ToV3 when migrator fails, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected error wrapping sentinel, got: %T — %v", err, err)
	}
}

// --- MigrateV2ToV3: v1→v2 DTO fields are zero ---

// TestMigrateV2ToV3_WithMigrator_V1ToV2FieldsAreZero verifies that the three
// v1→v2-specific fields (ClaimedIssuesConverted, HistoryRowsRemoved,
// LegacyRelationshipsTranslated) are zero in a MigrateV2ToV3 result, because
// the core maps only the v2→v3 counters.
func TestMigrateV2ToV3_WithMigrator_V1ToV2FieldsAreZero(t *testing.T) {
	t.Parallel()

	// Given — a fake migrator that returns non-zero v2→v3 counters only.
	m := &fakeMigrator{
		migrateV2ToV3Fn: func(_ context.Context) (driven.MigrationResult, error) {
			return driven.MigrationResult{
				IdempotencyKeysMigrated: 3,
				// v1→v2 fields deliberately absent (zero) from the driven result.
			}, nil
		},
	}
	svc := setupServiceWithMigrator(t, m)

	// When — MigrateV2ToV3 is called.
	result, err := svc.MigrateV2ToV3(t.Context())
	// Then — v1→v2 fields are zero in the driving DTO.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ClaimedIssuesConverted != 0 {
		t.Errorf("ClaimedIssuesConverted: got %d, want 0", result.ClaimedIssuesConverted)
	}
	if result.HistoryRowsRemoved != 0 {
		t.Errorf("HistoryRowsRemoved: got %d, want 0", result.HistoryRowsRemoved)
	}
	if result.LegacyRelationshipsTranslated != 0 {
		t.Errorf("LegacyRelationshipsTranslated: got %d, want 0", result.LegacyRelationshipsTranslated)
	}
}

// --- CheckSchemaVersion: errNoMigrator guard (mirrors v1→v2 coverage) ---

// TestCheckSchemaVersion_NoMigrator_ReturnsError verifies that CheckSchemaVersion
// returns an error when the service has no migrator configured.
func TestCheckSchemaVersion_NoMigrator_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a service with no migrator.
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	// When — CheckSchemaVersion is called.
	err := svc.CheckSchemaVersion(t.Context())

	// Then — an error is returned.
	if err == nil {
		t.Fatal("expected error from CheckSchemaVersion with nil migrator, got nil")
	}
}

// --- MigrateV1ToV2: errNoMigrator guard (existing coverage, reproduced for
// symmetry so a single file covers all migration delegate guards) ---

// TestMigrateV1ToV2_NoMigrator_ReturnsError verifies that calling MigrateV1ToV2
// on a service constructed without a Migrator returns an error.
func TestMigrateV1ToV2_NoMigrator_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a service with no migrator (nil).
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	// When — MigrateV1ToV2 is called.
	_, err := svc.MigrateV1ToV2(t.Context())

	// Then — an error is returned.
	if err == nil {
		t.Fatal("expected error from MigrateV1ToV2 with nil migrator, got nil")
	}
}
