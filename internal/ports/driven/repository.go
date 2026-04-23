package driven

import (
	"context"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
)

// DefaultLimit is the default maximum number of items for list operations
// when the caller does not specify a limit.
const DefaultLimit = 20

// NormalizeLimit applies the default limit when the caller passes zero.
// A zero limit is treated as "use the default", not "unlimited". To request
// all results without truncation, pass a negative limit.
func NormalizeLimit(limit int) int {
	if limit == 0 {
		return DefaultLimit
	}
	return limit
}

// IssueListItem is a lightweight projection of an issue for list views.
type IssueListItem struct {
	ID              domain.ID
	Role            domain.Role
	State           domain.State
	Priority        domain.Priority
	Title           string
	ParentID        domain.ID
	ParentCreatedAt time.Time
	CreatedAt       time.Time
	IsDeleted       bool
	// IsBlocked is true when the issue has at least one unresolved
	// blocked_by relationship or a blocked/deferred ancestor. This is a
	// computed display concern — the underlying state machine does not change.
	IsBlocked bool
	// BlockerIDs contains the IDs of non-closed issues that directly block
	// this issue via blocked_by relationships. Empty when IsBlocked is false.
	BlockerIDs []domain.ID
	// SecondaryState is the computed list-view secondary state for this item.
	// Populated by the service layer after retrieval from the repository.
	SecondaryState domain.SecondaryState
}

// DisplayStatus returns the human-readable status for display purposes in the
// format "primary (secondary)" — e.g., "open (ready)", "open (blocked)",
// "deferred (blocked)". When no secondary state applies (e.g., closed), it
// returns just the primary state string.
func (item IssueListItem) DisplayStatus() string {
	if item.SecondaryState == domain.SecondaryNone {
		return item.State.String()
	}
	return item.State.String() + " (" + item.SecondaryState.String() + ")"
}

// IssueFilter defines filtering criteria for issue list and search.
type IssueFilter struct {
	// Roles filters by one or more issue roles (empty means no filter).
	Roles []domain.Role
	// States filters by one or more states (empty means no filter).
	States []domain.State
	// Ready filters to only ready issues when true.
	Ready bool
	// ParentIDs filters to children of one or more parent epics.
	// When multiple IDs are provided, issues matching any parent are included.
	ParentIDs []domain.ID
	// DescendantsOf recursively filters to all descendants of an domain.
	DescendantsOf domain.ID
	// AncestorsOf filters to the parent chain of an issue (up to the root).
	AncestorsOf domain.ID
	// LabelFilters specifies label-based filters.
	LabelFilters []LabelFilter
	// Orphan filters to issues that have no parent epic.
	Orphan bool
	// Blocked filters to issues that have at least one unresolved blocked_by
	// relationship (target is neither closed nor deleted).
	Blocked bool
	// ExcludeClosed hides closed issues from results when true. Ignored when
	// States explicitly includes StateClosed — an explicit state filter
	// represents intentional user selection and takes precedence.
	ExcludeClosed bool
	// IncludeDeleted includes soft-deleted issues when true.
	IncludeDeleted bool
}

// LabelFilter specifies a single label-based filter criterion.
type LabelFilter struct {
	// Key is the label key to match.
	Key string
	// Value is the label value to match. Empty for wildcard ("key:*").
	Value string
	// Negate inverts the filter — exclude issues matching this label.
	Negate bool
}

// SortDirection indicates ascending or descending sort order. The zero value
// (SortAscending) is the standard ascending direction for every IssueOrderBy
// value: lowest numeric value first, oldest timestamp first, alphabetic A–Z.
type SortDirection int

const (
	// SortAscending is the default direction — lowest first, oldest first,
	// alphabetic A–Z. For timestamp-based IssueOrderBy values
	// (OrderByCreatedAt, OrderByUpdatedAt) this means oldest issues appear
	// first.
	SortAscending SortDirection = iota

	// SortDescending reverses the primary sort axis of an IssueOrderBy
	// variant. For timestamp-based values (OrderByCreatedAt,
	// OrderByUpdatedAt) this means newest issues appear first. Tiebreaker
	// columns (typically issue ID) remain ascending for deterministic output.
	SortDescending
)

// IssueOrderBy specifies the sort order for issue listings.
type IssueOrderBy int

const (
	// OrderByPriority sorts by priority (highest urgency first), then by
	// family-anchored creation time (COALESCE of parent's and issue's
	// created_at), then by issue created_at within a family, then by
	// issue ID as a deterministic tiebreaker.
	OrderByPriority IssueOrderBy = iota

	// OrderByCreatedAt sorts by family-anchored creation time (oldest
	// family first), then by issue created_at within a family, then by
	// issue ID as a deterministic tiebreaker.
	OrderByCreatedAt

	// OrderByUpdatedAt sorts by family-anchored creation time, then by issue
	// created_at within a family, then by issue ID as a deterministic
	// tiebreaker. SortAscending yields oldest-first; SortDescending yields
	// newest-first — consistent with every other IssueOrderBy value.
	OrderByUpdatedAt

	// OrderByPriorityCreated sorts by priority (highest urgency first),
	// then by the issue's own created_at (ascending), then by issue ID
	// as a deterministic tiebreaker. Unlike OrderByPriority, this variant
	// does not use family-anchored sorting — it treats each issue's
	// creation timestamp independently. Designed for flat listing commands
	// (ready, blocked) where parent grouping is not meaningful.
	OrderByPriorityCreated

	// OrderByID sorts by issue ID ascending (lexicographic on the string
	// representation). Because IDs contain a random component, this
	// produces a stable but effectively arbitrary order — useful as a
	// neutral default when no semantic ordering (priority, time) is
	// explicitly requested.
	OrderByID

	// OrderByRole sorts by role name ascending (alphabetic), then by
	// issue ID as a deterministic tiebreaker.
	OrderByRole

	// OrderByState sorts by state name ascending (alphabetic), then by
	// issue ID as a deterministic tiebreaker.
	OrderByState

	// OrderByTitle sorts by title ascending (case-insensitive alphabetic),
	// then by issue ID as a deterministic tiebreaker.
	OrderByTitle

	// OrderByParentID sorts by parent issue ID (lexicographic), then by issue
	// ID as a deterministic tiebreaker. Parentless issues use an empty-string
	// sentinel, so they cluster at the start under SortAscending and at the
	// end under SortDescending — descending is the exact reverse of ascending.
	OrderByParentID

	// OrderByParentCreated sorts by parent creation time, then by issue ID as
	// a deterministic tiebreaker. Parentless issues use an empty-string
	// sentinel, so they cluster at the start under SortAscending and at the
	// end under SortDescending — descending is the exact reverse of ascending.
	OrderByParentCreated
)

// CommentFilter defines filtering criteria for comment listings.
type CommentFilter struct {
	// Author filters comments by author.
	Author domain.Author
	// Authors filters comments by any of the listed authors (OR'd).
	Authors []domain.Author
	// CreatedAfter filters to comments created after this timestamp.
	CreatedAfter time.Time
	// AfterCommentID filters to comments with ID greater than this.
	AfterCommentID int64
	// IssueID scopes the search to a specific issue (zero = global).
	IssueID domain.ID
	// IssueIDs scopes the search to specific issues (OR'd with IssueID).
	IssueIDs []domain.ID
	// ParentIDs scopes to comments on issues that are direct children of
	// the specified parents (OR'd with other issue scopes).
	ParentIDs []domain.ID
	// TreeIDs scopes to comments on all issues in the tree rooted at the
	// specified IDs — ancestors through descendants (OR'd with other scopes).
	TreeIDs []domain.ID
	// LabelFilters scopes to comments on issues matching these labels.
	LabelFilters []LabelFilter
	// FollowRefs expands the scope to include all issues referenced (via
	// relationships) by any issue already in scope.
	FollowRefs bool
}

// HistoryFilter defines filtering criteria for history listings.
type HistoryFilter struct {
	// Author filters entries by author.
	Author domain.Author
	// After filters entries created after this timestamp.
	After time.Time
	// Before filters entries created before this timestamp.
	Before time.Time
}

// IssueSummary holds aggregate counts of issues grouped by primary state and
// computed readiness/blocked status. Designed for dashboard display — avoids
// loading individual issues into memory.
//
// The three primary states are open, closed, and deferred. Claimed is not a
// primary state; it is a transient secondary state of open (see SecondaryActive).
type IssueSummary struct {
	Open     int
	Deferred int
	Closed   int
	Ready    int
	Blocked  int
}

// Total returns the total number of issues across all primary states.
func (s IssueSummary) Total() int {
	return s.Open + s.Deferred + s.Closed
}

// IssueRepository defines the persistence interface for issues.
type IssueRepository interface {
	// CreateIssue persists a new domain. Returns the created domain.
	CreateIssue(ctx context.Context, t domain.Issue) error

	// GetIssue retrieves an issue by ID. Returns domain.ErrNotFound if
	// not found or if soft-deleted (unless includeDeleted is true).
	GetIssue(ctx context.Context, id domain.ID, includeDeleted bool) (domain.Issue, error)

	// UpdateIssue persists changes to an existing domain.
	UpdateIssue(ctx context.Context, t domain.Issue) error

	// ListIssues returns a filtered, ordered list of issues. A positive limit
	// caps the result size; a negative limit returns all matching results.
	// The hasMore return value indicates whether additional results exist
	// beyond the limit. The direction parameter controls whether the primary
	// sort axis runs ascending (SortAscending, the default) or descending
	// (SortDescending); tiebreaker columns always remain ascending.
	ListIssues(ctx context.Context, filter IssueFilter, orderBy IssueOrderBy, direction SortDirection, limit int) (items []IssueListItem, hasMore bool, err error)

	// SearchIssues performs full-text search on title, description, and
	// acceptance criteria. Limit and direction semantics match ListIssues.
	SearchIssues(ctx context.Context, query string, filter IssueFilter, orderBy IssueOrderBy, direction SortDirection, limit int) (items []IssueListItem, hasMore bool, err error)

	// GetChildStatuses returns the completion-relevant status of all direct
	// children of an epic, for deriving epic completion.
	GetChildStatuses(ctx context.Context, epicID domain.ID) ([]domain.ChildStatus, error)

	// GetDescendants returns all descendants of an epic (recursively),
	// with claim status, for recursive deletion checks.
	GetDescendants(ctx context.Context, epicID domain.ID) ([]domain.DescendantInfo, error)

	// HasChildren reports whether an epic has any children.
	HasChildren(ctx context.Context, epicID domain.ID) (bool, error)

	// GetAncestorStatuses returns the states of all ancestor epics of a
	// issue, walking up the parent chain, for readiness propagation.
	GetAncestorStatuses(ctx context.Context, id domain.ID) ([]domain.AncestorStatus, error)

	// GetParentID returns the parent ID of an issue (for cycle detection).
	GetParentID(ctx context.Context, id domain.ID) (domain.ID, error)

	// IssueIDExists reports whether an issue ID already exists (for
	// collision detection during ID generation).
	IssueIDExists(ctx context.Context, id domain.ID) (bool, error)

	// ListLabelCounts returns all unique label key-value pairs across
	// non-deleted issues (including closed and deferred), together with the
	// number of issues that carry each pair. The results are used by the
	// service layer to compute per-key popularity rankings. Hard-deleted
	// issues are excluded; soft state (closed, deferred) is included so that
	// the popularity signal reflects historical usage.
	ListLabelCounts(ctx context.Context) ([]domain.LabelCount, error)

	// GetIssueSummary returns aggregate issue counts by primary state and
	// computed readiness/blocked status. Excludes soft-deleted issues. Ready
	// and blocked counts follow the same rules as the Ready and Blocked
	// filters in ListIssues.
	GetIssueSummary(ctx context.Context) (IssueSummary, error)
}

// CommentRepository defines the persistence interface for comments.
type CommentRepository interface {
	// CreateComment persists a new comment and returns the assigned ID.
	CreateComment(ctx context.Context, c domain.Comment) (int64, error)

	// GetComment retrieves a comment by ID. Returns domain.ErrNotFound if not found.
	GetComment(ctx context.Context, id int64) (domain.Comment, error)

	// ListComments returns comments for an issue with optional filters.
	// Limit semantics match IssueRepository.ListIssues.
	ListComments(ctx context.Context, issueID domain.ID, filter CommentFilter, limit int) (items []domain.Comment, hasMore bool, err error)

	// SearchComments performs full-text search on comment bodies.
	// Limit semantics match IssueRepository.ListIssues.
	SearchComments(ctx context.Context, query string, filter CommentFilter, limit int) (items []domain.Comment, hasMore bool, err error)
}

// ClaimRepository defines the persistence interface for claims.
type ClaimRepository interface {
	// CreateClaim persists a new claim.
	CreateClaim(ctx context.Context, c domain.Claim) error

	// GetClaimByIssue retrieves the active claim for an domain.
	// Returns domain.ErrNotFound if no active claim exists.
	GetClaimByIssue(ctx context.Context, issueID domain.ID) (domain.Claim, error)

	// GetClaimByID retrieves a claim by its claim ID.
	// Returns domain.ErrNotFound if not found.
	GetClaimByID(ctx context.Context, claimID string) (domain.Claim, error)

	// InvalidateClaim removes the active claim from an domain.
	InvalidateClaim(ctx context.Context, claimID string) error

	// UpdateClaimStaleAt updates the stale-at timestamp on a claim,
	// effectively extending the claim's lifetime. Replaces the former
	// UpdateClaimLastActivity and UpdateClaimThreshold methods.
	UpdateClaimStaleAt(ctx context.Context, claimID string, staleAt time.Time) error

	// ListStaleClaims returns all claims that are stale as of the given time.
	ListStaleClaims(ctx context.Context, now time.Time) ([]domain.Claim, error)

	// ListActiveClaims returns all claims that are not stale as of the given
	// time.
	ListActiveClaims(ctx context.Context, now time.Time) ([]domain.Claim, error)

	// DeleteExpiredClaims removes all claim rows whose stale-at timestamp is
	// on or before now. Returns the number of rows deleted. Active claims
	// (stale-at is in the future) are not touched.
	DeleteExpiredClaims(ctx context.Context, now time.Time) (int, error)
}

// RelationshipRepository defines the persistence interface for relationships.
type RelationshipRepository interface {
	// CreateRelationship creates a relationship if it does not already exist.
	// Returns true if created, false if it already existed (idempotent).
	CreateRelationship(ctx context.Context, rel domain.Relationship) (bool, error)

	// DeleteRelationship removes a relationship if it exists.
	// Returns true if deleted, false if it did not exist (idempotent).
	DeleteRelationship(ctx context.Context, sourceID, targetID domain.ID, relType domain.RelationType) (bool, error)

	// ListRelationships returns all relationships for an issue (both
	// directions).
	ListRelationships(ctx context.Context, issueID domain.ID) ([]domain.Relationship, error)

	// GetBlockerStatuses returns the blocker statuses for readiness checks.
	GetBlockerStatuses(ctx context.Context, issueID domain.ID) ([]domain.BlockerStatus, error)
}

// HistoryRepository defines the persistence interface for history entries.
type HistoryRepository interface {
	// AppendHistory adds a history entry for an issue and returns the
	// assigned entry ID.
	AppendHistory(ctx context.Context, entry history.Entry) (int64, error)

	// ListHistory returns history entries for an issue with optional filters.
	// Limit semantics match IssueRepository.ListIssues.
	ListHistory(ctx context.Context, issueID domain.ID, filter HistoryFilter, limit int) (items []history.Entry, hasMore bool, err error)

	// CountHistory returns the number of history entries for an issue
	// (used to compute revision).
	CountHistory(ctx context.Context, issueID domain.ID) (int, error)

	// GetLatestHistory returns the most recent history entry for an issue
	// (used to derive the issue's current author).
	GetLatestHistory(ctx context.Context, issueID domain.ID) (history.Entry, error)
}

// DatabaseRepository defines database-level operations.
type DatabaseRepository interface {
	// InitDatabase creates the database schema and stores the prefix.
	InitDatabase(ctx context.Context, prefix string) error

	// GetPrefix retrieves the stored prefix.
	GetPrefix(ctx context.Context) (string, error)

	// GC physically removes deleted (and optionally closed) issue data.
	// Returns the number of deleted issues removed and, when
	// includeClosedIssues is true, the number of closed issues removed.
	GC(ctx context.Context, includeClosedIssues bool) (deletedCount int, closedCount int, err error)

	// IntegrityCheck runs database-level integrity validation (e.g. SQLite
	// PRAGMA integrity_check). Returns nil if the database is healthy.
	IntegrityCheck(ctx context.Context) error

	// CountDeletedRatio returns the total number of issues and the number of
	// soft-deleted issues, for GC threshold calculations.
	CountDeletedRatio(ctx context.Context) (total, deleted int, err error)

	// GetSchemaVersion returns the schema version stored in the metadata table.
	// Returns 0 when the database has no schema_version key (v1 schema), and
	// 2 when the database has been migrated to v2. The doctor command uses this
	// to report schema_migration_required when the database is v1.
	GetSchemaVersion(ctx context.Context) (int, error)

	// SetSchemaVersion writes the given version to the metadata table, inserting
	// the key if absent or updating it when already present. Used by the upgrade
	// command to record a successful v1→v2 migration within the migration
	// transaction.
	SetSchemaVersion(ctx context.Context, version int) error

	// ClearAllData removes all data from every table (issues, comments,
	// claims, relationships, history, labels, FTS, and metadata).
	// Used by restore to prepare a clean slate before inserting backup
	// data. Foreign-key constraints are temporarily disabled.
	ClearAllData(ctx context.Context) error

	// RestoreIssueRaw inserts a raw issue row without going through
	// domain constructors. Used by restore to faithfully recreate
	// arbitrary states (closed, deferred, deleted, etc.).
	RestoreIssueRaw(ctx context.Context, rec domain.BackupIssueRecord) error

	// RestoreCommentRaw inserts a comment row with an explicit ID,
	// bypassing the auto-increment assignment. Used by restore to
	// preserve original comment IDs.
	RestoreCommentRaw(ctx context.Context, issueID string, rec domain.BackupCommentRecord) error

	// RestoreClaimRaw inserts a claim row directly. Used by restore.
	RestoreClaimRaw(ctx context.Context, issueID string, rec domain.BackupClaimRecord) error

	// RestoreRelationshipRaw inserts a relationship row directly.
	// Used by restore.
	RestoreRelationshipRaw(ctx context.Context, sourceID string, rec domain.BackupRelationshipRecord) error

	// RestoreHistoryRaw inserts a history entry row with an explicit
	// ID. Used by restore to preserve original entry IDs and
	// revisions.
	RestoreHistoryRaw(ctx context.Context, issueID string, rec domain.BackupHistoryRecord) error

	// RestoreLabelRaw inserts a label row directly.
	// Used by restore.
	RestoreLabelRaw(ctx context.Context, issueID string, rec domain.BackupLabelRecord) error

	// RebuildFTS repopulates the full-text search tables from the
	// canonical data tables. Must be called after all issue and comment
	// data has been restored.
	RebuildFTS(ctx context.Context) error
}

// UnitOfWork represents a transactional scope. All repository operations
// within a unit of work are atomic — they either all succeed or all fail.
type UnitOfWork interface {
	// Issues returns the issue repository within this transaction.
	Issues() IssueRepository

	// Comments returns the comment repository within this transaction.
	Comments() CommentRepository

	// Claims returns the claim repository within this transaction.
	Claims() ClaimRepository

	// Relationships returns the relationship repository within this transaction.
	Relationships() RelationshipRepository

	// History returns the history repository within this transaction.
	History() HistoryRepository

	// Database returns the database-level repository within this transaction.
	Database() DatabaseRepository
}

// UnitOfWorkFactory creates new units of work.
type UnitOfWorkFactory interface {
	// Begin starts a new unit of work (transaction). The caller must call
	// Commit or Rollback on the returned UnitOfWork.
	Begin(ctx context.Context) (UnitOfWork, error)

	// ReadOnly starts a read-only unit of work.
	ReadOnly(ctx context.Context) (UnitOfWork, error)
}

// Transactor provides a higher-level API for executing work within a
// transaction. It handles commit/rollback automatically.
type Transactor interface {
	// WithTransaction executes fn within a transaction. If fn returns nil,
	// the transaction is committed. If fn returns an error, the transaction
	// is rolled back and the error is returned.
	WithTransaction(ctx context.Context, fn func(uow UnitOfWork) error) error

	// WithReadTransaction executes fn within a read-only transaction.
	WithReadTransaction(ctx context.Context, fn func(uow UnitOfWork) error) error

	// Vacuum reclaims disk space and defragments the database file. Must be
	// called outside any transaction.
	Vacuum(ctx context.Context) error
}

// MigrationResult carries the counts of changes made by a schema migration.
type MigrationResult struct {
	// ClaimedIssuesConverted is the number of issues whose state was changed
	// from "claimed" to "open" during the v1→v2 migration.
	ClaimedIssuesConverted int

	// HistoryRowsRemoved is the number of history rows deleted because their
	// event_type was "claimed" or "released" — event types removed in v2.
	HistoryRowsRemoved int

	// LegacyRelationshipsTranslated is the number of relationship rows whose
	// rel_type was translated from a v0.2.0 value ("cites" → "refs") or
	// dropped ("cited_by") during the v1→v2 migration.
	LegacyRelationshipsTranslated int

	// IdempotencyKeysMigrated is the number of non-NULL idempotency_key column
	// values successfully written as idempotency:<value> label rows during the
	// v2→v3 migration. Zero for v1→v2 results.
	IdempotencyKeysMigrated int

	// IdempotencyKeysSkipped is the number of idempotency_key column values not
	// written because the issue already carried an idempotency label (skip-on-conflict
	// policy). Zero for v1→v2 results.
	IdempotencyKeysSkipped int

	// InvalidLabelValuesSkipped is the number of idempotency_key column values
	// not written because domain.NewLabel rejected the stored value as invalid.
	// Zero for v1→v2 results.
	InvalidLabelValuesSkipped int
}

// Migrator exposes schema migration operations. It is implemented by the
// SQLite storage adapter and is a separate interface from Transactor because
// migration is a storage-specific concern that runs outside the normal
// unit-of-work lifecycle. The core service delegates to this interface so
// that driving adapters (CLI commands) never need to import the concrete
// storage adapter package.
type Migrator interface {
	// CheckSchemaVersion returns nil when the database schema is at the
	// current version (v3). It returns a wrapped domain.ErrSchemaMigrationRequired
	// when the schema is at an older version. Callers use this to determine
	// whether a migration is needed before issuing regular database commands.
	CheckSchemaVersion(ctx context.Context) error

	// MigrateV1ToV2 upgrades a v1 database to v2 schema in a single atomic
	// transaction. It is safe to call on a v2 database but CheckSchemaVersion
	// should be used first so the caller can distinguish the "already current"
	// case from a migration that made changes. Returns a MigrationResult
	// describing the number of rows affected by each migration step.
	MigrateV1ToV2(ctx context.Context) (MigrationResult, error)

	// MigrateV2ToV3 upgrades a v2 database to v3 schema in a single atomic
	// transaction. It carries each non-NULL idempotency_key column value forward
	// as an idempotency:<value> label row, drops the idx_issues_idempotency unique
	// index, drops the idempotency_key column, and records schema_version=3. It is
	// safe to call on a v3 database but CheckSchemaVersion should be used first to
	// distinguish the "already current" case. Returns a MigrationResult describing
	// the number of rows affected by each migration step.
	MigrateV2ToV3(ctx context.Context) (MigrationResult, error)
}
