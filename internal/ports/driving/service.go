package driving

import (
	"context"
	"time"
)

// Service defines the driving port — the use-case boundary that CLI and
// other adapters invoke. Each method corresponds to a command from §8 of
// the specification.
type Service interface {
	// --- Global Operations ---

	// Init creates a new database with the given prefix.
	Init(ctx context.Context, prefix string) error

	// AgentName generates a random agent name.
	AgentName(ctx context.Context) (string, error)

	// GetPrefix returns the database's configured issue ID prefix.
	GetPrefix(ctx context.Context) (string, error)

	// --- Issue Operations ---

	// CreateIssue creates a new issue.
	CreateIssue(ctx context.Context, input CreateIssueInput) (CreateIssueOutput, error)

	// ClaimByID claims a specific issue.
	ClaimByID(ctx context.Context, input ClaimInput) (ClaimOutput, error)

	// ClaimNextReady claims the highest-priority ready issue.
	ClaimNextReady(ctx context.Context, input ClaimNextReadyInput) (ClaimOutput, error)

	// LookupClaimIssueID returns the string representation of the issue ID
	// associated with the given claim ID. Returns domain.ErrNotFound if the
	// claim does not exist.
	LookupClaimIssueID(ctx context.Context, claimID string) (string, error)

	// LookupClaimAuthor returns the author who holds the given claim.
	// Returns domain.ErrNotFound if the claim does not exist.
	LookupClaimAuthor(ctx context.Context, claimID string) (string, error)

	// OneShotUpdate performs an atomic claim→update→release.
	OneShotUpdate(ctx context.Context, input OneShotUpdateInput) error

	// UpdateIssue updates a claimed issue's fields.
	UpdateIssue(ctx context.Context, input UpdateIssueInput) error

	// ExtendStaleThreshold extends the stale threshold on an active claim.
	ExtendStaleThreshold(ctx context.Context, issueID string, claimID string, threshold time.Duration) error

	// TransitionState changes the state of a claimed issue.
	TransitionState(ctx context.Context, input TransitionInput) error

	// CloseWithReason atomically adds a closing reason as a comment and
	// transitions the issue to closed. The author is derived from the claim
	// record. If any step fails, neither the comment nor the state change
	// persists. Returns an error if the reason is empty, the claim is invalid,
	// or the state transition is not allowed.
	CloseWithReason(ctx context.Context, input CloseWithReasonInput) error

	// DeferIssue transitions a claimed issue to the deferred state and
	// releases the claim — all within a single transaction.
	DeferIssue(ctx context.Context, input DeferIssueInput) error

	// ReopenIssue transitions a closed or deferred issue back to the open
	// state. The operation is atomic: the issue is claimed internally,
	// transitioned to open, and released — all within a single transaction.
	// Records EventReopened for closed issues or EventUndeferred for
	// deferred issues.
	ReopenIssue(ctx context.Context, input ReopenInput) error

	// DeleteIssue soft-deletes a claimed issue.
	DeleteIssue(ctx context.Context, input DeleteInput) error

	// ShowIssue returns the full detail view of an issue.
	ShowIssue(ctx context.Context, id string) (ShowIssueOutput, error)

	// ListIssues returns a filtered, ordered, paginated list of issues.
	ListIssues(ctx context.Context, input ListIssuesInput) (ListIssuesOutput, error)

	// SearchIssues performs full-text search on issues.
	SearchIssues(ctx context.Context, input SearchIssuesInput) (ListIssuesOutput, error)

	// GetIssueSummary returns aggregate issue counts by primary state and
	// computed readiness/blocked status. More efficient than calling
	// ListIssues multiple times — uses a single query internally.
	GetIssueSummary(ctx context.Context) (IssueSummaryOutput, error)

	// --- Epic Operations ---

	// EpicProgress returns completion data for open epics. It uses
	// GetChildStatuses to compute progress in a single query per epic,
	// avoiding the N+1 pattern of listing children via ListIssues.
	EpicProgress(ctx context.Context, input EpicProgressInput) (EpicProgressOutput, error)

	// CloseCompletedEpics finds all open epics in the completed secondary
	// state and batch-closes them. Each epic is claimed, a closing comment
	// is added, and the epic is transitioned to closed. Per-epic failures
	// are captured in the result without aborting the batch.
	CloseCompletedEpics(ctx context.Context, input CloseCompletedEpicsInput) (CloseCompletedEpicsOutput, error)

	// --- Label Operations ---

	// ListDistinctLabels returns all unique label key-value pairs
	// across non-deleted issues, projected as service-layer DTOs.
	ListDistinctLabels(ctx context.Context) ([]LabelOutput, error)

	// --- Relationship Operations ---

	// AddRelationship adds a relationship between two issues. The sourceID
	// is the string representation of the source issue ID.
	AddRelationship(ctx context.Context, sourceID string, rel RelationshipInput, author string) error

	// RemoveRelationship removes a relationship between two issues. The
	// sourceID is the string representation of the source issue ID.
	RemoveRelationship(ctx context.Context, sourceID string, rel RelationshipInput, author string) error

	// RemoveBidirectionalBlock removes any blocked_by relationship between
	// issueA and issueB regardless of which direction was stored. It tries
	// both "A blocked_by B" and "B blocked_by A". The operation is
	// idempotent — if neither relationship exists, no error is returned.
	// Both issue IDs are string representations.
	RemoveBidirectionalBlock(ctx context.Context, issueA, issueB string, author string) error

	// --- Label Propagation ---

	// PropagateLabel copies a label from a parent issue to all its descendants
	// that lack it or have a different value. Each descendant is updated via
	// OneShotUpdate (atomic claim→update→release). Per-descendant failures
	// are skipped without aborting the batch.
	PropagateLabel(ctx context.Context, input PropagateLabelInput) (PropagateLabelOutput, error)

	// --- Comment Operations ---

	// AddComment adds a comment to an issue.
	AddComment(ctx context.Context, input AddCommentInput) (AddCommentOutput, error)

	// ShowComment retrieves a single comment by ID.
	ShowComment(ctx context.Context, commentID int64) (CommentDTO, error)

	// ListComments lists comments for an issue.
	ListComments(ctx context.Context, input ListCommentsInput) (ListCommentsOutput, error)

	// SearchComments searches comments by text.
	SearchComments(ctx context.Context, input SearchCommentsInput) (ListCommentsOutput, error)

	// --- History Operations ---

	// ShowHistory lists history entries for an issue.
	ShowHistory(ctx context.Context, input ListHistoryInput) (ListHistoryOutput, error)

	// --- Graph ---

	// GetGraphData returns all non-deleted issues and their relationships
	// in a single read-only transaction, for rendering as a graph.
	GetGraphData(ctx context.Context) (GraphDataOutput, error)

	// --- Diagnostics ---

	// Doctor runs diagnostics and returns classified findings. The input
	// controls the minimum severity threshold and allows the caller to inject
	// additional findings from checks that run outside the service layer
	// (e.g., filesystem checks). The output includes per-check pass/fail
	// status and a healthy flag.
	Doctor(ctx context.Context, input DoctorInput) (DoctorOutput, error)

	// GC performs garbage collection.
	GC(ctx context.Context, input GCInput) (GCOutput, error)

	// --- Backup / Restore ---

	// Backup writes a complete snapshot of all non-deleted issues — with
	// their comments, labels, relationships, claims, and history — to the
	// provided BackupWriter. The operation runs in a single read
	// transaction for snapshot consistency.
	Backup(ctx context.Context, input BackupInput) (BackupOutput, error)

	// Restore replaces the entire database contents with data read from
	// the provided BackupReader. This is a destructive operation: all
	// existing data is removed before the backup is applied. The restore
	// is atomic — either the full backup is applied or the database is
	// left unchanged.
	Restore(ctx context.Context, input RestoreInput) error

	// --- Import ---

	// ImportIssues creates issues from validated JSONL import records.
	ImportIssues(ctx context.Context, input ImportInput) (ImportOutput, error)

	// --- Reset ---

	// CountAllIssues returns the total number of issues in the database,
	// including closed and deferred issues but excluding soft-deleted ones.
	CountAllIssues(ctx context.Context) (int, error)

	// ResetDatabase removes all data from every table and reclaims disk
	// space. This is a destructive operation: all issues, comments, claims,
	// relationships, history, and labels are permanently deleted. The
	// caller is responsible for creating a backup beforehand.
	ResetDatabase(ctx context.Context) error

	// --- Schema Migration ---

	// CheckSchemaVersion returns nil when the database is at the current
	// schema version (v3). It returns a wrapped domain.ErrSchemaMigrationRequired
	// when the schema is at an older version. Commands that gate on schema
	// version call this method rather than accessing the storage adapter directly.
	CheckSchemaVersion(ctx context.Context) error

	// MigrateV1ToV2 upgrades a v1 database to v2 schema in a single atomic
	// transaction. The migration converts claimed-state issues to open, removes
	// obsolete history event types, and records schema_version=2. Callers should
	// call CheckSchemaVersion first to distinguish the "already current" case
	// from a migration that made changes. Returns a MigrationResult with row
	// counts for each migration step.
	MigrateV1ToV2(ctx context.Context) (MigrationResult, error)

	// MigrateV2ToV3 upgrades a v2 database to v3 schema in a single atomic
	// transaction. The migration carries each non-NULL idempotency_key column
	// value forward as an idempotency:<value> label row (skip-on-conflict policy),
	// drops the idx_issues_idempotency unique index and the idempotency_key column,
	// and records schema_version=3. Callers should call CheckSchemaVersion first to
	// distinguish the "already current" case. Returns a MigrationResult with counts
	// for migrated, skipped, and invalid rows.
	MigrateV2ToV3(ctx context.Context) (MigrationResult, error)
}
