package port

import (
	"context"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
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
	ID        issue.ID
	Role      issue.Role
	State     issue.State
	Priority  issue.Priority
	Title     string
	ParentID  issue.ID
	CreatedAt time.Time
	UpdatedAt time.Time
	IsDeleted bool
	// IsBlocked is true when the issue has at least one unresolved
	// blocked_by relationship. This is a computed display concern —
	// the underlying state machine does not change.
	IsBlocked bool
}

// DisplayStatus returns the human-readable status for display purposes.
// When the issue is blocked, it returns "blocked" instead of the underlying
// state. This is a presentation concern — the domain state machine is unchanged.
func (item IssueListItem) DisplayStatus() string {
	if item.IsBlocked {
		return "blocked"
	}
	return item.State.String()
}

// IssueFilter defines filtering criteria for issue list and search.
type IssueFilter struct {
	// Role filters by issue role (zero value means no filter).
	Role issue.Role
	// States filters by one or more states (empty means no filter).
	States []issue.State
	// Ready filters to only ready issues when true.
	Ready bool
	// ParentID filters to children of a specific epic.
	ParentID issue.ID
	// DescendantsOf recursively filters to all descendants of an issue.
	DescendantsOf issue.ID
	// AncestorsOf filters to the parent chain of an issue (up to the root).
	AncestorsOf issue.ID
	// LabelFilters specifies dimension-based filters.
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

// LabelFilter specifies a single dimension-based filter criterion.
type LabelFilter struct {
	// Key is the dimension key to match.
	Key string
	// Value is the dimension value to match. Empty for wildcard ("key:*").
	Value string
	// Negate inverts the filter — exclude issues matching this dimension.
	Negate bool
}

// IssueOrderBy specifies the sort order for issue listings.
type IssueOrderBy int

const (
	// OrderByPriority sorts by priority (highest urgency first), then
	// creation time as tiebreaker.
	OrderByPriority IssueOrderBy = iota

	// OrderByCreatedAt sorts by creation time (oldest first).
	OrderByCreatedAt

	// OrderByUpdatedAt sorts by last modification time (most recent first).
	OrderByUpdatedAt
)

// CommentFilter defines filtering criteria for comment listings.
type CommentFilter struct {
	// Author filters comments by author.
	Author identity.Author
	// CreatedAfter filters to comments created after this timestamp.
	CreatedAfter time.Time
	// AfterCommentID filters to comments with ID greater than this.
	AfterCommentID int64
	// IssueID scopes the search to a specific issue (zero = global).
	IssueID issue.ID
}

// HistoryFilter defines filtering criteria for history listings.
type HistoryFilter struct {
	// Author filters entries by author.
	Author identity.Author
	// After filters entries created after this timestamp.
	After time.Time
	// Before filters entries created before this timestamp.
	Before time.Time
}

// IssueRepository defines the persistence interface for issues.
type IssueRepository interface {
	// CreateIssue persists a new issue. Returns the created issue.
	CreateIssue(ctx context.Context, t issue.Issue) error

	// GetIssue retrieves an issue by ID. Returns domain.ErrNotFound if
	// not found or if soft-deleted (unless includeDeleted is true).
	GetIssue(ctx context.Context, id issue.ID, includeDeleted bool) (issue.Issue, error)

	// UpdateIssue persists changes to an existing issue.
	UpdateIssue(ctx context.Context, t issue.Issue) error

	// ListIssues returns a filtered, ordered list of issues. A positive limit
	// caps the result size; a negative limit returns all matching results.
	// The hasMore return value indicates whether additional results exist
	// beyond the limit.
	ListIssues(ctx context.Context, filter IssueFilter, orderBy IssueOrderBy, limit int) (items []IssueListItem, hasMore bool, err error)

	// SearchIssues performs full-text search on title, description, and
	// acceptance criteria. Limit semantics match ListIssues.
	SearchIssues(ctx context.Context, query string, filter IssueFilter, orderBy IssueOrderBy, limit int) (items []IssueListItem, hasMore bool, err error)

	// GetChildStatuses returns the completion-relevant status of all direct
	// children of an epic, for deriving epic completion.
	GetChildStatuses(ctx context.Context, epicID issue.ID) ([]issue.ChildStatus, error)

	// GetDescendants returns all descendants of an epic (recursively),
	// with claim status, for recursive deletion checks.
	GetDescendants(ctx context.Context, epicID issue.ID) ([]issue.DescendantInfo, error)

	// HasChildren reports whether an epic has any children.
	HasChildren(ctx context.Context, epicID issue.ID) (bool, error)

	// GetAncestorStatuses returns the states of all ancestor epics of a
	// issue, walking up the parent chain, for readiness propagation.
	GetAncestorStatuses(ctx context.Context, id issue.ID) ([]issue.AncestorStatus, error)

	// GetParentID returns the parent ID of an issue (for cycle detection).
	GetParentID(ctx context.Context, id issue.ID) (issue.ID, error)

	// IssueIDExists reports whether an issue ID already exists (for
	// collision detection during ID generation).
	IssueIDExists(ctx context.Context, id issue.ID) (bool, error)

	// ListDistinctLabels returns all unique dimension key-value pairs
	// across non-deleted issues.
	ListDistinctLabels(ctx context.Context) ([]issue.Label, error)

	// GetIssueByIdempotencyKey retrieves an issue by its idempotency key.
	// Returns domain.ErrNotFound if no issue exists with that key.
	GetIssueByIdempotencyKey(ctx context.Context, key string) (issue.Issue, error)
}

// CommentRepository defines the persistence interface for comments.
type CommentRepository interface {
	// CreateComment persists a new comment and returns the assigned ID.
	CreateComment(ctx context.Context, c comment.Comment) (int64, error)

	// GetComment retrieves a comment by ID. Returns domain.ErrNotFound if not found.
	GetComment(ctx context.Context, id int64) (comment.Comment, error)

	// ListComments returns comments for an issue with optional filters.
	// Limit semantics match IssueRepository.ListIssues.
	ListComments(ctx context.Context, issueID issue.ID, filter CommentFilter, limit int) (items []comment.Comment, hasMore bool, err error)

	// SearchComments performs full-text search on comment bodies.
	// Limit semantics match IssueRepository.ListIssues.
	SearchComments(ctx context.Context, query string, filter CommentFilter, limit int) (items []comment.Comment, hasMore bool, err error)
}

// ClaimRepository defines the persistence interface for claims.
type ClaimRepository interface {
	// CreateClaim persists a new claim.
	CreateClaim(ctx context.Context, c claim.Claim) error

	// GetClaimByIssue retrieves the active claim for an issue.
	// Returns domain.ErrNotFound if no active claim exists.
	GetClaimByIssue(ctx context.Context, issueID issue.ID) (claim.Claim, error)

	// GetClaimByID retrieves a claim by its claim ID.
	// Returns domain.ErrNotFound if not found.
	GetClaimByID(ctx context.Context, claimID string) (claim.Claim, error)

	// InvalidateClaim removes the active claim from an issue.
	InvalidateClaim(ctx context.Context, claimID string) error

	// UpdateClaimLastActivity updates the last activity timestamp on a claim.
	UpdateClaimLastActivity(ctx context.Context, claimID string, lastActivity time.Time) error

	// UpdateClaimThreshold updates the stale threshold on a claim.
	UpdateClaimThreshold(ctx context.Context, claimID string, threshold time.Duration) error

	// ListStaleClaims returns all claims that are stale as of the given time.
	ListStaleClaims(ctx context.Context, now time.Time) ([]claim.Claim, error)

	// ListActiveClaims returns all claims that are not stale as of the given
	// time.
	ListActiveClaims(ctx context.Context, now time.Time) ([]claim.Claim, error)
}

// RelationshipRepository defines the persistence interface for relationships.
type RelationshipRepository interface {
	// CreateRelationship creates a relationship if it does not already exist.
	// Returns true if created, false if it already existed (idempotent).
	CreateRelationship(ctx context.Context, rel issue.Relationship) (bool, error)

	// DeleteRelationship removes a relationship if it exists.
	// Returns true if deleted, false if it did not exist (idempotent).
	DeleteRelationship(ctx context.Context, sourceID, targetID issue.ID, relType issue.RelationType) (bool, error)

	// ListRelationships returns all relationships for an issue (both
	// directions).
	ListRelationships(ctx context.Context, issueID issue.ID) ([]issue.Relationship, error)

	// GetBlockerStatuses returns the blocker statuses for readiness checks.
	GetBlockerStatuses(ctx context.Context, issueID issue.ID) ([]issue.BlockerStatus, error)
}

// HistoryRepository defines the persistence interface for history entries.
type HistoryRepository interface {
	// AppendHistory adds a history entry for an issue and returns the
	// assigned entry ID.
	AppendHistory(ctx context.Context, entry history.Entry) (int64, error)

	// ListHistory returns history entries for an issue with optional filters.
	// Limit semantics match IssueRepository.ListIssues.
	ListHistory(ctx context.Context, issueID issue.ID, filter HistoryFilter, limit int) (items []history.Entry, hasMore bool, err error)

	// CountHistory returns the number of history entries for an issue
	// (used to compute revision).
	CountHistory(ctx context.Context, issueID issue.ID) (int, error)

	// GetLatestHistory returns the most recent history entry for an issue
	// (used to derive the issue's current author).
	GetLatestHistory(ctx context.Context, issueID issue.ID) (history.Entry, error)
}

// DatabaseRepository defines database-level operations.
type DatabaseRepository interface {
	// InitDatabase creates the database schema and stores the prefix.
	InitDatabase(ctx context.Context, prefix string) error

	// GetPrefix retrieves the stored prefix.
	GetPrefix(ctx context.Context) (string, error)

	// GC physically removes deleted (and optionally closed) issue data.
	GC(ctx context.Context, includeClosedIssues bool) error

	// IntegrityCheck runs database-level integrity validation (e.g. SQLite
	// PRAGMA integrity_check). Returns nil if the database is healthy.
	IntegrityCheck(ctx context.Context) error

	// CountDeletedRatio returns the total number of issues and the number of
	// soft-deleted issues, for GC threshold calculations.
	CountDeletedRatio(ctx context.Context) (total, deleted int, err error)
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
}
