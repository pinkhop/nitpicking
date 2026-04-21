package upgrade_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/upgrade"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- upgradeServiceStub ---

// upgradeServiceStub is a minimal stub implementing driving.Service with only
// the methods used by the upgrade command's Run function. All other methods
// panic to ensure tests do not inadvertently exercise them.
//
// The function fields allow each test to configure the exact response for the
// schema-version check and each migration step independently.
type upgradeServiceStub struct {
	// checkSchemaVersionFn returns the result for CheckSchemaVersion.
	checkSchemaVersionFn func(ctx context.Context) error

	// migrateV1ToV2Fn returns the result for MigrateV1ToV2.
	migrateV1ToV2Fn func(ctx context.Context) (driving.MigrationResult, error)

	// migrateV2ToV3Fn returns the result for MigrateV2ToV3.
	migrateV2ToV3Fn func(ctx context.Context) (driving.MigrationResult, error)
}

// CheckSchemaVersion delegates to checkSchemaVersionFn.
func (s *upgradeServiceStub) CheckSchemaVersion(ctx context.Context) error {
	return s.checkSchemaVersionFn(ctx)
}

// MigrateV1ToV2 delegates to migrateV1ToV2Fn.
func (s *upgradeServiceStub) MigrateV1ToV2(ctx context.Context) (driving.MigrationResult, error) {
	return s.migrateV1ToV2Fn(ctx)
}

// MigrateV2ToV3 delegates to migrateV2ToV3Fn.
func (s *upgradeServiceStub) MigrateV2ToV3(ctx context.Context) (driving.MigrationResult, error) {
	return s.migrateV2ToV3Fn(ctx)
}

// The remaining methods are required to satisfy the driving.Service interface
// but are never called during an upgrade. They panic on invocation so that any
// accidental call is caught immediately during testing.

func (s *upgradeServiceStub) Init(_ context.Context, _ string) error {
	panic("upgradeServiceStub: Init not expected during upgrade")
}

func (s *upgradeServiceStub) AgentName(_ context.Context) (string, error) {
	panic("upgradeServiceStub: AgentName not expected during upgrade")
}

func (s *upgradeServiceStub) GetPrefix(_ context.Context) (string, error) {
	panic("upgradeServiceStub: GetPrefix not expected during upgrade")
}

func (s *upgradeServiceStub) CreateIssue(_ context.Context, _ driving.CreateIssueInput) (driving.CreateIssueOutput, error) {
	panic("upgradeServiceStub: CreateIssue not expected during upgrade")
}

func (s *upgradeServiceStub) ClaimByID(_ context.Context, _ driving.ClaimInput) (driving.ClaimOutput, error) {
	panic("upgradeServiceStub: ClaimByID not expected during upgrade")
}

func (s *upgradeServiceStub) ClaimNextReady(_ context.Context, _ driving.ClaimNextReadyInput) (driving.ClaimOutput, error) {
	panic("upgradeServiceStub: ClaimNextReady not expected during upgrade")
}

func (s *upgradeServiceStub) LookupClaimIssueID(_ context.Context, _ string) (string, error) {
	panic("upgradeServiceStub: LookupClaimIssueID not expected during upgrade")
}

func (s *upgradeServiceStub) LookupClaimAuthor(_ context.Context, _ string) (string, error) {
	panic("upgradeServiceStub: LookupClaimAuthor not expected during upgrade")
}

func (s *upgradeServiceStub) OneShotUpdate(_ context.Context, _ driving.OneShotUpdateInput) error {
	panic("upgradeServiceStub: OneShotUpdate not expected during upgrade")
}

func (s *upgradeServiceStub) UpdateIssue(_ context.Context, _ driving.UpdateIssueInput) error {
	panic("upgradeServiceStub: UpdateIssue not expected during upgrade")
}

func (s *upgradeServiceStub) ExtendStaleThreshold(_ context.Context, _, _ string, _ time.Duration) error {
	panic("upgradeServiceStub: ExtendStaleThreshold not expected during upgrade")
}

func (s *upgradeServiceStub) TransitionState(_ context.Context, _ driving.TransitionInput) error {
	panic("upgradeServiceStub: TransitionState not expected during upgrade")
}

func (s *upgradeServiceStub) CloseWithReason(_ context.Context, _ driving.CloseWithReasonInput) error {
	panic("upgradeServiceStub: CloseWithReason not expected during upgrade")
}

func (s *upgradeServiceStub) DeferIssue(_ context.Context, _ driving.DeferIssueInput) error {
	panic("upgradeServiceStub: DeferIssue not expected during upgrade")
}

func (s *upgradeServiceStub) ReopenIssue(_ context.Context, _ driving.ReopenInput) error {
	panic("upgradeServiceStub: ReopenIssue not expected during upgrade")
}

func (s *upgradeServiceStub) DeleteIssue(_ context.Context, _ driving.DeleteInput) error {
	panic("upgradeServiceStub: DeleteIssue not expected during upgrade")
}

func (s *upgradeServiceStub) ShowIssue(_ context.Context, _ string) (driving.ShowIssueOutput, error) {
	panic("upgradeServiceStub: ShowIssue not expected during upgrade")
}

func (s *upgradeServiceStub) ListIssues(_ context.Context, _ driving.ListIssuesInput) (driving.ListIssuesOutput, error) {
	panic("upgradeServiceStub: ListIssues not expected during upgrade")
}

func (s *upgradeServiceStub) SearchIssues(_ context.Context, _ driving.SearchIssuesInput) (driving.ListIssuesOutput, error) {
	panic("upgradeServiceStub: SearchIssues not expected during upgrade")
}

func (s *upgradeServiceStub) GetIssueSummary(_ context.Context) (driving.IssueSummaryOutput, error) {
	panic("upgradeServiceStub: GetIssueSummary not expected during upgrade")
}

func (s *upgradeServiceStub) EpicProgress(_ context.Context, _ driving.EpicProgressInput) (driving.EpicProgressOutput, error) {
	panic("upgradeServiceStub: EpicProgress not expected during upgrade")
}

func (s *upgradeServiceStub) CloseCompletedEpics(_ context.Context, _ driving.CloseCompletedEpicsInput) (driving.CloseCompletedEpicsOutput, error) {
	panic("upgradeServiceStub: CloseCompletedEpics not expected during upgrade")
}

func (s *upgradeServiceStub) ListDistinctLabels(_ context.Context) ([]driving.LabelOutput, error) {
	panic("upgradeServiceStub: ListDistinctLabels not expected during upgrade")
}

func (s *upgradeServiceStub) AddRelationship(_ context.Context, _ string, _ driving.RelationshipInput, _ string) error {
	panic("upgradeServiceStub: AddRelationship not expected during upgrade")
}

func (s *upgradeServiceStub) RemoveRelationship(_ context.Context, _ string, _ driving.RelationshipInput, _ string) error {
	panic("upgradeServiceStub: RemoveRelationship not expected during upgrade")
}

func (s *upgradeServiceStub) RemoveBidirectionalBlock(_ context.Context, _, _ string, _ string) error {
	panic("upgradeServiceStub: RemoveBidirectionalBlock not expected during upgrade")
}

func (s *upgradeServiceStub) PropagateLabel(_ context.Context, _ driving.PropagateLabelInput) (driving.PropagateLabelOutput, error) {
	panic("upgradeServiceStub: PropagateLabel not expected during upgrade")
}

func (s *upgradeServiceStub) AddComment(_ context.Context, _ driving.AddCommentInput) (driving.AddCommentOutput, error) {
	panic("upgradeServiceStub: AddComment not expected during upgrade")
}

func (s *upgradeServiceStub) ShowComment(_ context.Context, _ int64) (driving.CommentDTO, error) {
	panic("upgradeServiceStub: ShowComment not expected during upgrade")
}

func (s *upgradeServiceStub) ListComments(_ context.Context, _ driving.ListCommentsInput) (driving.ListCommentsOutput, error) {
	panic("upgradeServiceStub: ListComments not expected during upgrade")
}

func (s *upgradeServiceStub) SearchComments(_ context.Context, _ driving.SearchCommentsInput) (driving.ListCommentsOutput, error) {
	panic("upgradeServiceStub: SearchComments not expected during upgrade")
}

func (s *upgradeServiceStub) ShowHistory(_ context.Context, _ driving.ListHistoryInput) (driving.ListHistoryOutput, error) {
	panic("upgradeServiceStub: ShowHistory not expected during upgrade")
}

func (s *upgradeServiceStub) GetGraphData(_ context.Context) (driving.GraphDataOutput, error) {
	panic("upgradeServiceStub: GetGraphData not expected during upgrade")
}

func (s *upgradeServiceStub) Doctor(_ context.Context, _ driving.DoctorInput) (driving.DoctorOutput, error) {
	panic("upgradeServiceStub: Doctor not expected during upgrade")
}

func (s *upgradeServiceStub) GC(_ context.Context, _ driving.GCInput) (driving.GCOutput, error) {
	panic("upgradeServiceStub: GC not expected during upgrade")
}

func (s *upgradeServiceStub) Backup(_ context.Context, _ driving.BackupInput) (driving.BackupOutput, error) {
	panic("upgradeServiceStub: Backup not expected during upgrade")
}

func (s *upgradeServiceStub) Restore(_ context.Context, _ driving.RestoreInput) error {
	panic("upgradeServiceStub: Restore not expected during upgrade")
}

func (s *upgradeServiceStub) ImportIssues(_ context.Context, _ driving.ImportInput) (driving.ImportOutput, error) {
	panic("upgradeServiceStub: ImportIssues not expected during upgrade")
}

func (s *upgradeServiceStub) CountAllIssues(_ context.Context) (int, error) {
	panic("upgradeServiceStub: CountAllIssues not expected during upgrade")
}

func (s *upgradeServiceStub) ResetDatabase(_ context.Context) error {
	panic("upgradeServiceStub: ResetDatabase not expected during upgrade")
}

// --- Helpers ---

// errMigrationRequired is a sentinel that wraps domain.ErrSchemaMigrationRequired
// for use in stub functions, mirroring the error shape the real sqlite adapter
// produces.
var errMigrationRequired = &domain.DatabaseError{
	Op:  "schema version check",
	Err: fmt.Errorf("%w: database needs migration", domain.ErrSchemaMigrationRequired),
}

// newV3Stub returns an upgradeServiceStub whose CheckSchemaVersion returns nil
// (the database is already at v3). Migration methods panic because they must
// not be called for an up-to-date database.
func newV3Stub() *upgradeServiceStub {
	return &upgradeServiceStub{
		checkSchemaVersionFn: func(_ context.Context) error { return nil },
		migrateV1ToV2Fn: func(_ context.Context) (driving.MigrationResult, error) {
			panic("upgradeServiceStub: MigrateV1ToV2 must not be called on v3 database")
		},
		migrateV2ToV3Fn: func(_ context.Context) (driving.MigrationResult, error) {
			panic("upgradeServiceStub: MigrateV2ToV3 must not be called on v3 database")
		},
	}
}

// newV2Stub returns an upgradeServiceStub whose CheckSchemaVersion returns
// ErrSchemaMigrationRequired (v2 database), MigrateV1ToV2 returns a zero-count
// result (no-op, since v1→v2 was already applied), and MigrateV2ToV3 returns
// the provided v23Result.
func newV2Stub(v23Result driving.MigrationResult) *upgradeServiceStub {
	return &upgradeServiceStub{
		checkSchemaVersionFn: func(_ context.Context) error {
			return errMigrationRequired
		},
		migrateV1ToV2Fn: func(_ context.Context) (driving.MigrationResult, error) {
			// v1→v2 is a no-op on a v2 database — all SQL changes match zero rows
			// and SetSchemaVersion(2) is idempotent. Return zero counts to mirror
			// what the real adapter would return in this scenario.
			return driving.MigrationResult{}, nil
		},
		migrateV2ToV3Fn: func(_ context.Context) (driving.MigrationResult, error) {
			return v23Result, nil
		},
	}
}

// newV1Stub returns an upgradeServiceStub whose CheckSchemaVersion returns
// ErrSchemaMigrationRequired (v1 database), MigrateV1ToV2 returns v12Result,
// and MigrateV2ToV3 returns v23Result.
func newV1Stub(v12Result, v23Result driving.MigrationResult) *upgradeServiceStub {
	return &upgradeServiceStub{
		checkSchemaVersionFn: func(_ context.Context) error {
			return errMigrationRequired
		},
		migrateV1ToV2Fn: func(_ context.Context) (driving.MigrationResult, error) {
			return v12Result, nil
		},
		migrateV2ToV3Fn: func(_ context.Context) (driving.MigrationResult, error) {
			return v23Result, nil
		},
	}
}

// --- NewCmd flag structure ---

func TestNewCmd_HasJSONFlag(t *testing.T) {
	t.Parallel()

	// Given — a command constructed with a minimal factory.
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When — the command is constructed.
	cmd := upgrade.NewCmd(f)

	// Then — the command exposes a --json flag.
	var found bool
	for _, fl := range cmd.Flags {
		for _, name := range fl.Names() {
			if name == "json" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected --json flag on admin upgrade command")
	}
}

func TestNewCmd_CommandName(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	// When
	cmd := upgrade.NewCmd(f)

	// Then — the command name is "upgrade".
	if cmd.Name != "upgrade" {
		t.Errorf("expected command name %q, got %q", "upgrade", cmd.Name)
	}
}

// --- NewCmd via runFn injection ---

// TestNewCmd_RunFn_UpToDate_TextOutput verifies that when Run returns "up to
// date", the text output contains the expected phrase.
func TestNewCmd_RunFn_UpToDate_TextOutput(t *testing.T) {
	t.Parallel()

	// Given — a factory with captured output streams and a stub runFn.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		cs := iostreams.NewColorScheme(false)
		_, err := input.Out.Write([]byte(cs.SuccessIcon() + " Database is up to date\n"))
		return err
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Database is up to date") {
		t.Errorf("expected 'Database is up to date' in output, got: %q", stdout.String())
	}
}

// TestNewCmd_RunFn_Migrated_JSONOutput verifies that the migrated status is
// emitted as valid JSON with the correct structure.
func TestNewCmd_RunFn_Migrated_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given — a factory with captured output streams and a stub runFn that
	// simulates a completed migration.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		return cmdutil.WriteJSON(input.Out, map[string]interface{}{
			"status":                   "migrated",
			"claimed_issues_converted": 3,
			"history_rows_removed":     7,
		})
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — no error and valid JSON with "migrated" status.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON output: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "migrated" {
		t.Errorf("status: got %v, want %q", out["status"], "migrated")
	}
}

// TestNewCmd_RunFn_UpToDate_JSONOutput verifies that up_to_date is emitted as
// valid JSON when --json is passed.
func TestNewCmd_RunFn_UpToDate_JSONOutput(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	runFn := func(_ context.Context, input upgrade.RunInput) error {
		return cmdutil.WriteJSON(input.Out, map[string]interface{}{
			"status":                   "up_to_date",
			"claimed_issues_converted": 0,
			"history_rows_removed":     0,
		})
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON output: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "up_to_date" {
		t.Errorf("status: got %v, want %q", out["status"], "up_to_date")
	}
}

// --- Run — business logic via stub service ---

// TestRun_V3Database_ReportsUpToDate verifies that a database already at v3
// causes Run to emit "up_to_date" status without invoking any migration.
func TestRun_V3Database_ReportsUpToDate(t *testing.T) {
	t.Parallel()

	// Given — a service stub that reports the database is already at v3, and a
	// runFn that wires it in and delegates to Run.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	stub := newV3Stub()
	runFn := func(ctx context.Context, input upgrade.RunInput) error {
		input.Svc = stub
		return upgrade.Run(ctx, input)
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — no error, status is "up_to_date", all counters are zero.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "up_to_date" {
		t.Errorf("status: got %v, want %q", out["status"], "up_to_date")
	}
	if out["claimed_issues_converted"] != float64(0) {
		t.Errorf("claimed_issues_converted: got %v, want 0", out["claimed_issues_converted"])
	}
	if out["idempotency_keys_migrated"] != float64(0) {
		t.Errorf("idempotency_keys_migrated: got %v, want 0", out["idempotency_keys_migrated"])
	}
}

// TestRun_V2Database_MigratesV2ToV3_ReportsMigrated verifies that a v2
// database causes Run to apply only the v2→v3 migration step and report the
// idempotency-key counters in the JSON output.
func TestRun_V2Database_MigratesV2ToV3_ReportsMigrated(t *testing.T) {
	t.Parallel()

	// Given — a service stub that simulates a v2 database with four idempotency
	// keys to migrate.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	stub := newV2Stub(driving.MigrationResult{
		IdempotencyKeysMigrated:   4,
		IdempotencyKeysSkipped:    1,
		InvalidLabelValuesSkipped: 2,
	})
	runFn := func(ctx context.Context, input upgrade.RunInput) error {
		input.Svc = stub
		return upgrade.Run(ctx, input)
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — status "migrated"; v1→v2 counters are zero; v2→v3 counters are
	// populated, including invalid_label_values_skipped so operators can detect
	// dedupe metadata that was dropped because it failed label validation.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "migrated" {
		t.Errorf("status: got %v, want %q", out["status"], "migrated")
	}
	// v1→v2 step must be a no-op on a v2 database.
	if out["claimed_issues_converted"] != float64(0) {
		t.Errorf("claimed_issues_converted: got %v, want 0 (v1→v2 step must be a no-op on v2 database)", out["claimed_issues_converted"])
	}
	if out["idempotency_keys_migrated"] != float64(4) {
		t.Errorf("idempotency_keys_migrated: got %v, want 4", out["idempotency_keys_migrated"])
	}
	if out["idempotency_keys_skipped"] != float64(1) {
		t.Errorf("idempotency_keys_skipped: got %v, want 1", out["idempotency_keys_skipped"])
	}
	if out["invalid_label_values_skipped"] != float64(2) {
		t.Errorf("invalid_label_values_skipped: got %v, want 2", out["invalid_label_values_skipped"])
	}
}

// TestRun_V1Database_MigratesV1ToV3_ReportsBothCounters verifies that a v1
// database causes Run to apply v1→v2 followed by v2→v3 in a single invocation
// and that the JSON output includes counters from both steps.
func TestRun_V1Database_MigratesV1ToV3_ReportsBothCounters(t *testing.T) {
	t.Parallel()

	// Given — a service stub that simulates a v1 database with claimed issues,
	// history rows, legacy relationships, and idempotency keys.
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	stub := newV1Stub(
		driving.MigrationResult{
			ClaimedIssuesConverted:        2,
			HistoryRowsRemoved:            5,
			LegacyRelationshipsTranslated: 3,
		},
		driving.MigrationResult{
			IdempotencyKeysMigrated: 6,
			IdempotencyKeysSkipped:  0,
		},
	)
	runFn := func(ctx context.Context, input upgrade.RunInput) error {
		input.Svc = stub
		return upgrade.Run(ctx, input)
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — status "migrated" with both sets of counters present.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &out); decodeErr != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", decodeErr, stdout.String())
	}
	if out["status"] != "migrated" {
		t.Errorf("status: got %v, want %q", out["status"], "migrated")
	}
	if out["claimed_issues_converted"] != float64(2) {
		t.Errorf("claimed_issues_converted: got %v, want 2", out["claimed_issues_converted"])
	}
	if out["history_rows_removed"] != float64(5) {
		t.Errorf("history_rows_removed: got %v, want 5", out["history_rows_removed"])
	}
	if out["legacy_relationships_translated"] != float64(3) {
		t.Errorf("legacy_relationships_translated: got %v, want 3", out["legacy_relationships_translated"])
	}
	if out["idempotency_keys_migrated"] != float64(6) {
		t.Errorf("idempotency_keys_migrated: got %v, want 6", out["idempotency_keys_migrated"])
	}
	if out["idempotency_keys_skipped"] != float64(0) {
		t.Errorf("idempotency_keys_skipped: got %v, want 0", out["idempotency_keys_skipped"])
	}
}

// TestRun_V2Database_JSONOutputContainsNewCounters verifies that the JSON
// output of a v2→v3 migration includes the idempotency_keys_migrated and
// idempotency_keys_skipped fields, so callers can parse the full result
// programmatically.
func TestRun_V2Database_JSONOutputContainsNewCounters(t *testing.T) {
	t.Parallel()

	// Given
	ios, _, stdout, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	stub := newV2Stub(driving.MigrationResult{
		IdempotencyKeysMigrated: 2,
		IdempotencyKeysSkipped:  0,
	})
	runFn := func(ctx context.Context, input upgrade.RunInput) error {
		input.Svc = stub
		return upgrade.Run(ctx, input)
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade", "--json"})
	// Then — the JSON object includes both new counter keys, even when zero.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]interface{}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &raw); decodeErr != nil {
		t.Fatalf("invalid JSON: %v — raw: %s", decodeErr, stdout.String())
	}
	if _, ok := raw["idempotency_keys_migrated"]; !ok {
		t.Error("JSON output missing field: idempotency_keys_migrated")
	}
	if _, ok := raw["idempotency_keys_skipped"]; !ok {
		t.Error("JSON output missing field: idempotency_keys_skipped")
	}
	if _, ok := raw["invalid_label_values_skipped"]; !ok {
		t.Error("JSON output missing field: invalid_label_values_skipped")
	}
}

// TestRun_CheckSchemaVersionError_ReturnsError verifies that a non-migration
// error from CheckSchemaVersion (e.g., I/O failure) is propagated to the
// caller rather than treated as a migration trigger.
func TestRun_CheckSchemaVersionError_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a service stub that returns an I/O error (not wrapped in
	// ErrSchemaMigrationRequired) from CheckSchemaVersion.
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios}

	sentinelErr := errors.New("simulated I/O failure")
	stub := &upgradeServiceStub{
		checkSchemaVersionFn: func(_ context.Context) error { return sentinelErr },
		migrateV1ToV2Fn: func(_ context.Context) (driving.MigrationResult, error) {
			panic("upgradeServiceStub: MigrateV1ToV2 must not be called when CheckSchemaVersion returns a non-migration error")
		},
		migrateV2ToV3Fn: func(_ context.Context) (driving.MigrationResult, error) {
			panic("upgradeServiceStub: MigrateV2ToV3 must not be called when CheckSchemaVersion returns a non-migration error")
		},
	}
	runFn := func(ctx context.Context, input upgrade.RunInput) error {
		input.Svc = stub
		return upgrade.Run(ctx, input)
	}

	// When
	cmd := upgrade.NewCmd(f, runFn)
	err := cmd.Run(t.Context(), []string{"upgrade"})

	// Then — the error wraps the sentinel; no migration was attempted.
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected error chain to contain sentinel error, got: %v", err)
	}
}
