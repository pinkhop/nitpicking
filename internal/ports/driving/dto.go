package driving

import (
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// --- Label DTOs ---

// LabelInput is a service-layer DTO for specifying a label key-value pair.
// CLI adapters construct this type from raw strings; the service implementation
// validates and converts to the domain domain.Label type internally.
type LabelInput struct {
	// Key is the label key (e.g., "kind", "area").
	Key string
	// Value is the label value (e.g., "bug", "auth").
	Value string
}

// LabelOutput is a service-layer DTO for returning a label key-value pair.
// The service converts domain domain.Label values into this type so driving
// adapters do not depend on domain types.
type LabelOutput struct {
	// Key is the label key.
	Key string
	// Value is the label value.
	Value string
}

// --- Issue List Item DTO ---

// IssueListItemDTO is a flat projection of a driven.IssueListItem with all
// domain types converted to strings. Driving adapters consume this type
// instead of the driven-port IssueListItem so they do not depend on domain
// packages. The service layer performs the conversion after retrieving data
// from the repository.
type IssueListItemDTO struct {
	// ID is the string representation of the issue ID (e.g., "NP-a3bxr").
	ID string
	// Role is the issue role (RoleTask or RoleEpic).
	Role domain.Role
	// State is the primary state (StateOpen, StateClosed, StateDeferred).
	// Claimed is a secondary state of open, not a primary state.
	State domain.State
	// Priority is the priority level (P0–P3).
	Priority domain.Priority
	// Title is the issue title.
	Title string
	// ParentID is the string representation of the parent issue ID, or empty
	// if the issue has no parent.
	ParentID string
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// IsDeleted is true when the issue has been soft-deleted.
	IsDeleted bool
	// IsBlocked is true when the issue has at least one unresolved blocked_by
	// relationship or a blocked/deferred ancestor.
	IsBlocked bool
	// BlockerIDs contains the string representations of non-closed issue IDs
	// that directly block this issue via blocked_by relationships. Empty when
	// IsBlocked is false or blocking is inherited only.
	BlockerIDs []string
	// SecondaryState is the computed list-view secondary state (e.g.,
	// SecondaryReady, SecondaryBlocked, SecondaryActive). SecondaryNone
	// indicates no secondary state.
	SecondaryState domain.SecondaryState
	// DisplayStatus is the human-readable status (e.g., "open (ready)",
	// "open (blocked)"). Precomputed from State and SecondaryState.
	DisplayStatus string
}

// --- Issue DTOs ---

// CreateIssueInput holds the parameters for creating an domain.
type CreateIssueInput struct {
	Role               domain.Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           domain.Priority
	ParentID           string
	Labels             []LabelInput
	Relationships      []RelationshipInput
	Author             string
	Claim              bool
	IdempotencyKey     string
}

// RelationshipInput describes a relationship to add during issue creation
// or update.
type RelationshipInput struct {
	Type     domain.RelationType
	TargetID string
}

// CreateIssueOutput holds the result of creating an domain.
type CreateIssueOutput struct {
	Issue   domain.Issue
	ClaimID string // Non-empty if the issue was created as claimed.
}

// ClaimInput holds the parameters for claiming an issue.
type ClaimInput struct {
	IssueID        string
	Author         string
	StaleThreshold time.Duration
	// StaleAt is an optional absolute timestamp at which the claim becomes
	// stale. When non-zero, it takes precedence over StaleThreshold. The
	// caller is responsible for validating that StaleAt is in the future
	// and within 24 hours; the service passes it through to the domain.
	StaleAt time.Time
	// LabelFilters specifies label guard-rail assertions when claiming by ID.
	// If non-empty, the issue must match all filters or the claim fails with a
	// descriptive error naming the issue and unmet condition.
	LabelFilters []LabelFilterInput
	// Role specifies a role guard-rail assertion when claiming by ID. If
	// non-zero, the issue must have this role or the claim fails with a
	// descriptive error naming the issue and unmet condition.
	Role domain.Role
}

// ClaimOutput holds the result of claiming an issue.
type ClaimOutput struct {
	ClaimID   string
	IssueID   string
	Author    string
	CreatedAt time.Time
	StaleAt   time.Time
}

// ClaimNextReadyInput holds the parameters for claiming the next ready issue.
// The service selects the highest-priority open issue that has no active
// (non-stale) claim held by another author, no unresolved blockers, and
// matches all provided label and role filters.
type ClaimNextReadyInput struct {
	Author         string
	Role           domain.Role
	LabelFilters   []LabelFilterInput
	StaleThreshold time.Duration
	// StaleAt is an optional absolute timestamp at which the claim becomes
	// stale. When non-zero, it takes precedence over StaleThreshold.
	StaleAt time.Time
}

// UpdateIssueInput holds the parameters for updating a claimed domain.
type UpdateIssueInput struct {
	IssueID            string
	ClaimID            string
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *domain.Priority
	ParentID           *string
	LabelSet           []LabelInput
	LabelRemove        []string
	RelationshipAdd    []RelationshipInput
	RelationshipRemove []RelationshipInput
	CommentBody        string
}

// OneShotUpdateInput holds the parameters for an atomic claim→update→release.
type OneShotUpdateInput struct {
	IssueID            string
	Author             string
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *domain.Priority
	ParentID           *string
	LabelSet           []LabelInput
	LabelRemove        []string
}

// TransitionInput holds the parameters for a state transition.
type TransitionInput struct {
	IssueID string
	ClaimID string
	Action  TransitionAction
}

// TransitionAction identifies the kind of state transition.
type TransitionAction int

const (
	// ActionRelease returns the issue to its default unclaimed state.
	ActionRelease TransitionAction = iota + 1

	// ActionClose marks a task as complete. Terminal.
	ActionClose

	// ActionDefer shelves the domain.
	ActionDefer
)

// CloseWithReasonInput holds the parameters for the atomic close-with-reason
// workflow. The service derives the author from the claim record, adds the
// reason as a comment, and closes the issue — all within a single transaction.
// If any step fails, neither the comment nor the state change persists.
type CloseWithReasonInput struct {
	IssueID string
	ClaimID string
	Reason  string
}

// DeferIssueInput holds the parameters for atomically deferring an issue with
// an optional "defer-until" label. When Until is non-empty, a "defer-until"
// label is set on the issue before the state transition — both within a single
// transaction so the label mutation and state change are atomic.
type DeferIssueInput struct {
	IssueID string
	ClaimID string
	Until   string
}

// ReopenInput holds the parameters for reopening a closed or deferred domain.
// The service validates that the issue is in a reopenable state (closed or
// deferred), claims it internally, transitions it back to open, and records
// the appropriate history event (EventReopened or EventUndeferred).
type ReopenInput struct {
	IssueID string
	Author  string
}

// DeleteInput holds the parameters for soft-deleting an domain.
type DeleteInput struct {
	IssueID string
	ClaimID string
}

// InheritedBlocking describes why an issue is not ready due to a blocked
// ancestor. The AncestorID identifies the first blocked ancestor, and
// BlockerIDs lists the unresolved blockers on that ancestor.
type InheritedBlocking struct {
	AncestorID string
	BlockerIDs []string
}

// RelationshipDTO is a flat projection of a domain relationship, using string
// fields so that driving adapters do not depend on domain types. The service
// layer converts domain.Relationship values into this type.
type RelationshipDTO struct {
	// SourceID is the string representation of the relationship's source issue ID.
	SourceID string
	// TargetID is the string representation of the relationship's target issue ID.
	TargetID string
	// Type is the canonical string name of the relationship type (e.g.,
	// "blocked_by", "blocks", "refs", "parent_of", "child_of").
	Type string
}

// ShowIssueOutput holds the full detail view of an domain. All fields are
// primitive types or service-layer DTOs — CLI adapters do not need to import
// the domain package to read this struct.
type ShowIssueOutput struct {
	// Flat primitive-typed fields populated from the domain Issue by the
	// service implementation.
	ID                 string
	Role               domain.Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           domain.Priority
	State              domain.State
	ParentID           string
	Labels             map[string]string
	CreatedAt          time.Time

	Revision      int
	Author        string
	Relationships []RelationshipDTO
	IsReady       bool
	// SecondaryState is the single list-view secondary state (e.g.,
	// SecondaryReady, SecondaryBlocked). SecondaryNone indicates no secondary
	// state.
	SecondaryState domain.SecondaryState
	// DetailStates is the ordered set of secondary conditions for detail views
	// (e.g., [SecondaryBlocked, SecondaryActive]). May contain multiple
	// entries for epics. Empty or nil when there is no secondary state.
	DetailStates      []domain.SecondaryState
	InheritedBlocking *InheritedBlocking
	CommentCount      int
	ChildCount        int
	Children          []IssueListItemDTO
	Comments          []CommentDTO
	ParentTitle       string
	BlockerDetails    []BlockerDetail
	ClaimID           string
	ClaimAuthor       string
	ClaimedAt         time.Time
	ClaimStaleAt      time.Time
}

// BlockerDetail holds enriched information about an issue that blocks this
// one via a blocked_by relationship. The command layer uses this to render
// blockers ordered by state and annotated with claim/deferral metadata.
type BlockerDetail struct {
	ID          string
	Title       string
	State       domain.State
	ClaimAuthor string
}

// IssueSummaryOutput holds aggregate issue counts for the status dashboard.
// Mirrors driven.IssueSummary with an additional Total convenience field.
// The three primary states are open, closed, and deferred; claimed is a
// transient secondary state of open and is not counted separately.
type IssueSummaryOutput struct {
	Open     int
	Deferred int
	Closed   int
	Ready    int
	Blocked  int
	Total    int
}

// --- Filter & Ordering Types ---
//
// These service-layer types mirror the driven-port filter and ordering
// vocabulary so that driving adapters (CLI commands) depend only on the
// driving port package — not on internal/ports/driven. The core implementation
// translates these to driven-port types before calling the repository.

// LabelFilterInput specifies a single label-based filter criterion.
type LabelFilterInput struct {
	// Key is the label key to match.
	Key string
	// Value is the label value to match. Empty for wildcard ("key:*").
	Value string
	// Negate inverts the filter — exclude issues matching this label.
	Negate bool
}

// IssueFilterInput defines filtering criteria for issue list and search
// operations. CLI commands construct this type; the service translates it
// to the repository's filter type internally.
type IssueFilterInput struct {
	// Roles filters by one or more issue roles (empty means no filter).
	Roles []domain.Role
	// States filters by one or more states (empty means no filter).
	States []domain.State
	// Ready filters to only ready issues when true.
	Ready bool
	// ParentIDs filters to children of one or more parent epics.
	// When multiple IDs are provided, issues matching any parent are included.
	ParentIDs []string
	// DescendantsOf recursively filters to all descendants of an domain.
	DescendantsOf string
	// AncestorsOf filters to the parent chain of an issue (up to the root).
	AncestorsOf string
	// LabelFilters specifies label-based filters.
	LabelFilters []LabelFilterInput
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

// OrderBy specifies the sort order for issue listings. CLI commands use these
// constants; the service translates them to the repository's ordering type.
type OrderBy int

const (
	// OrderByPriority sorts by priority (highest urgency first), then by
	// family-anchored creation time, then by issue ID as a tiebreaker.
	OrderByPriority OrderBy = iota

	// OrderByCreatedAt sorts by family-anchored creation time (oldest
	// family first), then by issue created_at within a family.
	OrderByCreatedAt

	// OrderByUpdatedAt sorts by family-anchored creation time (most
	// recent family first), then by issue created_at descending.
	OrderByUpdatedAt
)

// ListIssuesInput holds the parameters for listing issues.
type ListIssuesInput struct {
	Filter  IssueFilterInput
	OrderBy OrderBy
	Limit   int
}

// ListIssuesOutput holds the result of listing issues. Items are flat
// service-layer DTOs with string fields — driving adapters do not need to
// import domain or driven-port packages to read this struct.
type ListIssuesOutput struct {
	Items   []IssueListItemDTO
	HasMore bool
}

// SearchIssuesInput holds the parameters for searching issues.
type SearchIssuesInput struct {
	Query        string
	Filter       IssueFilterInput
	OrderBy      OrderBy
	Limit        int
	IncludeNotes bool
}

// --- Comment DTOs ---

// CommentDTO is a flat projection of a domain comment with primitive-typed
// fields. CLI adapters do not need to import domain/comment to read this.
type CommentDTO struct {
	// CommentID is the sequential integer ID (displayed as "comment-N").
	CommentID int64
	// DisplayID is the human-readable ID (e.g., "comment-368").
	DisplayID string
	// IssueID is the string representation of the parent issue ID.
	IssueID string
	// Author is the comment author's name.
	Author string
	// Body is the comment text.
	Body string
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
}

// CommentFilterInput defines filtering criteria for comment list and search
// operations. CLI commands construct this type; the service translates it
// to the repository's filter type internally.
type CommentFilterInput struct {
	// Authors filters comments by any of the listed authors (OR'd).
	Authors []string
	// CreatedAfter filters to comments created after this timestamp.
	CreatedAfter time.Time
	// AfterCommentID filters to comments with ID greater than this.
	AfterCommentID int64
	// IssueIDs scopes the search to specific issues (OR'd).
	IssueIDs []string
	// ParentIDs scopes to comments on issues that are direct children of
	// the specified parents.
	ParentIDs []string
	// TreeIDs scopes to comments on all issues in the tree rooted at the
	// specified IDs.
	TreeIDs []string
	// LabelFilters scopes to comments on issues matching these labels.
	LabelFilters []LabelFilterInput
	// FollowRefs expands the scope to include all issues referenced by any
	// issue already in scope.
	FollowRefs bool
}

// AddCommentInput holds the parameters for adding a comment.
type AddCommentInput struct {
	IssueID string
	Author  string
	Body    string
}

// AddCommentOutput holds the result of adding a comment.
type AddCommentOutput struct {
	Comment CommentDTO
}

// ListCommentsInput holds the parameters for listing comments.
type ListCommentsInput struct {
	IssueID string
	Filter  CommentFilterInput
	Limit   int
}

// ListCommentsOutput holds the result of listing comments.
type ListCommentsOutput struct {
	Comments []CommentDTO
	HasMore  bool
}

// SearchCommentsInput holds the parameters for searching comments.
type SearchCommentsInput struct {
	Query   string
	IssueID string // Empty for global search.
	Filter  CommentFilterInput
	Limit   int
}

// --- History DTOs ---

// HistoryEntryDTO is a flat projection of a domain history entry with
// primitive-typed fields. CLI adapters do not need to import domain/history.
type HistoryEntryDTO struct {
	// IssueID is the string representation of the issue this entry belongs to.
	IssueID string
	// Revision is the sequential revision number within the domain.
	Revision int
	// Author is the name of the agent that performed the action.
	Author string
	// Timestamp is when the action occurred.
	Timestamp time.Time
	// EventType is the canonical string name of the event (e.g., "created",
	// "claimed", "closed").
	EventType string
	// Changes lists the field-level changes recorded in this entry.
	Changes []FieldChangeDTO
}

// FieldChangeDTO describes a single field change within a history entry.
type FieldChangeDTO struct {
	// Field is the name of the changed field.
	Field string
	// Before is the previous value (empty for additions).
	Before string
	// After is the new value (empty for removals).
	After string
}

// HistoryFilterInput defines filtering criteria for history queries. CLI
// commands construct this type; the service translates it to the repository's
// filter type internally.
type HistoryFilterInput struct {
	// Author filters entries by author name.
	Author string
	// After filters entries created after this timestamp.
	After time.Time
	// Before filters entries created before this timestamp.
	Before time.Time
}

// ListHistoryInput holds the parameters for listing history.
type ListHistoryInput struct {
	IssueID string
	Filter  HistoryFilterInput
	Limit   int
}

// ListHistoryOutput holds the result of listing history.
type ListHistoryOutput struct {
	Entries []HistoryEntryDTO
	HasMore bool
}

// --- Epic DTOs ---

// EpicProgressInput holds the parameters for retrieving epic completion data.
type EpicProgressInput struct {
	// Filter restricts which epics are included. When empty, all open epics
	// are returned. Callers may set Roles, ExcludeClosed, or other filter
	// fields; the service always forces the role to epic.
	Filter IssueFilterInput
	// EpicID, when non-empty, restricts the result to a single epic.
	EpicID string
}

// EpicProgressItem holds completion data for a single epic.
type EpicProgressItem struct {
	ID             string
	Title          string
	State          domain.State
	Priority       domain.Priority
	SecondaryState domain.SecondaryState
	// Total is the number of direct children.
	Total int
	// Closed is the number of children in the closed state.
	Closed int
	// Open is the number of non-blocked children in the open state.
	// This includes children with active claims (open/claimed secondary state).
	Open int
	// Blocked is the number of children with unresolved blocked_by
	// relationships (regardless of primary state).
	Blocked int
	// Deferred is the number of non-blocked children in the deferred state.
	Deferred int
	// Percent is the completion percentage (0–100).
	Percent int
	// Completed is true when all children are closed.
	Completed bool
}

// EpicProgressOutput holds the result of the epic progress query.
type EpicProgressOutput struct {
	Items []EpicProgressItem
}

// CloseCompletedEpicsInput holds the parameters for batch-closing completed
// epics.
type CloseCompletedEpicsInput struct {
	// Author is the agent closing the epics. Used for claiming and commenting.
	Author string
	// DryRun, when true, returns the list of completed epics without closing
	// them.
	DryRun bool
	// IncludeTasks, when true, extends the close-completed logic to tasks that
	// have children, all of which are closed. Tasks without children are not
	// affected by this flag.
	IncludeTasks bool
}

// CloseCompletedEpicResult records the outcome of attempting to close a single
// epic.
type CloseCompletedEpicResult struct {
	ID      string
	Title   string
	Closed  bool
	Message string
}

// CloseCompletedEpicsOutput holds the result of batch-closing completed epics.
type CloseCompletedEpicsOutput struct {
	Results     []CloseCompletedEpicResult
	ClosedCount int
}

// --- Label Propagation DTOs ---

// PropagateLabelInput holds the parameters for propagating a label from a
// parent issue to all its descendants.
type PropagateLabelInput struct {
	// IssueID is the parent issue whose label value is propagated.
	IssueID string
	// Key is the label key to propagate.
	Key string
	// Author is the agent performing the propagation, used for claiming
	// each descendant during one-shot updates.
	Author string
}

// PropagateLabelOutput holds the result of a label propagation operation.
type PropagateLabelOutput struct {
	// Value is the label value that was propagated from the parent.
	Value string
	// Propagated is the number of descendants that were updated.
	Propagated int
	// Total is the total number of descendants examined.
	Total int
}

// --- Diagnostics DTOs ---

// DoctorSeverity represents the severity level of a diagnostic check.
// Higher numeric values indicate more severe checks.
type DoctorSeverity int

const (
	// SeverityInfo is the lowest severity — informational checks.
	SeverityInfo DoctorSeverity = iota
	// SeverityWarning is the middle severity — potential problems.
	SeverityWarning
	// SeverityError is the highest severity — integrity or correctness issues.
	SeverityError
)

// String returns the human-readable label for a severity level.
func (s DoctorSeverity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// ParseDoctorSeverity converts a string to a DoctorSeverity. Returns an error
// for unrecognized values.
func ParseDoctorSeverity(s string) (DoctorSeverity, error) {
	switch s {
	case "error":
		return SeverityError, nil
	case "warning":
		return SeverityWarning, nil
	case "info":
		return SeverityInfo, nil
	default:
		return 0, fmt.Errorf("invalid severity %q: must be error, warning, or info", s)
	}
}

// ActionKind identifies the type of action recommended by a DoctorFinding.
// The CLI adapter maps each kind to a specific np command string; other
// adapters (web, TUI) can map the same kind to buttons, keybindings, etc.
type ActionKind string

const (
	// ActionKindRunGC suggests running garbage collection.
	ActionKindRunGC ActionKind = "run_gc"
	// ActionKindUndefer suggests restoring the deferred issue identified by IssueID.
	ActionKindUndefer ActionKind = "undefer"
	// ActionKindUnblockRelationship suggests removing the stale blocked_by
	// relationship between SourceID and TargetID.
	ActionKindUnblockRelationship ActionKind = "unblock_relationship"
	// ActionKindCloseCompleted suggests closing all epics whose children are
	// fully closed.
	ActionKindCloseCompleted ActionKind = "close_completed"
	// ActionKindInvestigateCorruption suggests backing up and investigating
	// data corruption; no parameters are needed.
	ActionKindInvestigateCorruption ActionKind = "investigate_corruption"
	// ActionKindExecSQL suggests running the SQL statement in SQL directly
	// against the database.
	ActionKindExecSQL ActionKind = "exec_sql"
	// ActionKindAddToGitignore suggests adding a path to .gitignore.
	ActionKindAddToGitignore ActionKind = "add_to_gitignore"
)

// ActionHint describes a structured action a caller should take to address
// a DoctorFinding. The Kind field names the action; the remaining fields
// supply the parameters that action requires. Fields irrelevant to a given
// Kind will be zero.
type ActionHint struct {
	// Kind names the action to take.
	Kind ActionKind `json:"kind"`
	// IssueID is the issue to act on (Undefer).
	IssueID string `json:"issue_id,omitzero"`
	// SourceID and TargetID identify the two ends of the relationship to
	// remove (UnblockRelationship).
	SourceID string `json:"source_id,omitzero"`
	// TargetID is the second issue in the relationship to remove.
	TargetID string `json:"target_id,omitzero"`
	// SQL is the statement to execute against the database (ExecSQL).
	SQL string `json:"sql,omitzero"`
}

// DoctorFinding represents a single diagnostic finding.
type DoctorFinding struct {
	// Category identifies the kind of finding.
	Category string `json:"category"`
	// Severity is "warning" or "error".
	Severity string `json:"severity"`
	// Message describes the finding.
	Message string `json:"message"`
	// IssueIDs lists affected issues.
	IssueIDs []string `json:"issue_ids,omitzero"`
	// Action describes a structured remediation action the caller should take.
	// Nil when no specific action is recommended.
	Action *ActionHint `json:"action,omitzero"`
}

// DoctorInput holds the parameters for running diagnostics.
type DoctorInput struct {
	// MinSeverity is the minimum severity threshold. Checks below this
	// threshold are skipped and their findings are excluded.
	MinSeverity DoctorSeverity
	// AdditionalFindings are findings from checks that run outside the
	// service layer (e.g., filesystem checks). They are merged with
	// service-generated findings before classification.
	AdditionalFindings []DoctorFinding
}

// DoctorCheckResult records the pass/fail/skipped status of a single
// diagnostic check.
type DoctorCheckResult struct {
	// Name is the check's identifier shown in output.
	Name string
	// Status is "pass", "fail", or "skipped".
	Status string
	// Detail is a human-readable description of the check result.
	Detail string
}

// DoctorOutput holds the results of the doctor diagnostic.
type DoctorOutput struct {
	// Findings contains findings from active (non-skipped) checks only.
	Findings []DoctorFinding
	// Checks contains the pass/fail/skipped status of every registered
	// diagnostic check.
	Checks []DoctorCheckResult
	// Healthy is true when no active findings exist.
	Healthy bool
}

// GraphDataOutput holds the data needed to render an issue graph.
type GraphDataOutput struct {
	// Nodes contains all non-deleted issues as lightweight projections.
	Nodes []IssueListItemDTO
	// Relationships contains all relationships for the included issues,
	// projected as flat DTOs with string fields.
	Relationships []RelationshipDTO
}

// GCInput holds the parameters for garbage collection.
type GCInput struct {
	IncludeClosed bool
}

// GCOutput holds the result of garbage collection.
type GCOutput struct {
	DeletedIssuesRemoved int
	ClosedIssuesRemoved  int
	// ExpiredClaimsDeleted is the number of stale claim rows that were
	// removed. This count is always populated regardless of IncludeClosed.
	ExpiredClaimsDeleted int
}

// --- Backup / Restore DTOs ---

// BackupInput holds the parameters for creating a database backup.
type BackupInput struct {
	// Writer is the driven-port writer that receives the serialised
	// backup data. The caller is responsible for constructing and
	// closing the writer.
	Writer driven.BackupWriter
}

// BackupOutput holds the result of a backup operation.
type BackupOutput struct {
	// IssueCount is the number of issue records written.
	IssueCount int
}

// RestoreInput holds the parameters for restoring a database from backup.
type RestoreInput struct {
	// Reader is the driven-port reader that provides the serialised
	// backup data. The caller is responsible for constructing and
	// closing the reader.
	Reader driven.BackupReader
}

// --- Import DTOs ---

// ImportInput holds the parameters for importing validated JSONL records.
type ImportInput struct {
	Records       []domain.ValidatedRecord
	DefaultAuthor string
	ForceAuthor   bool
}

// ImportLineResult describes the outcome of importing a single record.
type ImportLineResult struct {
	IdempotencyKey string
	IssueID        domain.ID
	Skipped        bool // True if the idempotency key was already imported.
	Err            error
}

// ImportOutput holds the aggregate result of an import operation.
type ImportOutput struct {
	Created int
	Skipped int
	Failed  int
	Results []ImportLineResult
}

// --- Schema Migration DTOs ---

// MigrationResult describes the outcome of a v1→v2 schema migration. It is
// returned by Service.MigrateV1ToV2 and consumed by the upgrade command to
// produce human-readable or JSON output.
type MigrationResult struct {
	// ClaimedIssuesConverted is the number of issues whose primary state was
	// changed from "claimed" to "open" during the migration.
	ClaimedIssuesConverted int

	// HistoryRowsRemoved is the number of history rows deleted because their
	// event_type was "claimed" or "released", which are no longer valid in v2.
	HistoryRowsRemoved int
}
