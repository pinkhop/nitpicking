package domain

import "time"

// BackupAlgorithmVersion identifies the backup format version. Restore
// implementations dispatch on this value to select the correct
// deserialisation path.
//
// Version history:
//
//	1 — initial format; included claim rows in the backup.
//	2 — claims are transient and excluded from backup.
const BackupAlgorithmVersion = 2

// BackupHeader is the first record in a backup file. It contains metadata
// about the backup itself — when it was taken, from which database
// prefix, and which algorithm version was used to produce it.
type BackupHeader struct {
	// Prefix is the issue-ID prefix of the source database (e.g. "NP").
	Prefix string `json:"prefix"`

	// Timestamp is the UTC moment the backup was initiated.
	Timestamp time.Time `json:"timestamp"`

	// Version is the backup algorithm version. Restore implementations
	// use this to select the correct deserialisation logic.
	Version int `json:"version"`
}

// BackupIssueRecord is a self-contained snapshot of a single issue and all
// of its associated data: labels, comments, relationships, claims,
// and history. Each non-deleted issue in the database produces exactly
// one BackupIssueRecord in the backup.
type BackupIssueRecord struct {
	// --- Core issue fields ---

	// IssueID is the unique identifier (e.g. "NP-a3bxr").
	IssueID string `json:"issue_id"`

	// Role is "task" or "epic".
	Role string `json:"role"`

	// Title is the issue title.
	Title string `json:"title"`

	// Description is the issue body text.
	Description string `json:"description"`

	// AcceptanceCriteria is the issue's acceptance criteria text.
	AcceptanceCriteria string `json:"acceptance_criteria"`

	// Priority is the priority string (e.g. "P1", "P2").
	Priority string `json:"priority"`

	// State is the lifecycle state (e.g. "open", "closed", "deferred").
	State string `json:"state"`

	// ParentID is the parent epic's ID, or empty when unparented.
	ParentID string `json:"parent_id"`

	// CreatedAt is the issue creation timestamp in RFC 3339 with
	// nanosecond precision.
	CreatedAt time.Time `json:"created_at"`

	// IdempotencyKey is the optional deduplication key set at creation.
	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// --- Associated data ---

	// Labels is the set of key–value label pairs attached to the issue.
	Labels []BackupLabelRecord `json:"labels"`

	// Comments is the ordered list of comments on the issue.
	Comments []BackupCommentRecord `json:"comments"`

	// Relationships is the list of relationships where this issue is
	// the source.
	Relationships []BackupRelationshipRecord `json:"relationships"`

	// Claims is the list of active claims on the issue. Typically zero
	// or one, but modelled as a slice for forward-compatibility.
	Claims []BackupClaimRecord `json:"claims"`

	// History is the ordered list of history entries for the issue.
	History []BackupHistoryRecord `json:"history"`
}

// BackupLabelRecord is a single key–value label on an issue.
type BackupLabelRecord struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BackupCommentRecord is a snapshot of a single comment.
type BackupCommentRecord struct {
	// CommentID is the auto-increment integer ID.
	CommentID int64 `json:"comment_id"`

	// Author is the comment author's name.
	Author string `json:"author"`

	// CreatedAt is the comment creation timestamp.
	CreatedAt time.Time `json:"created_at"`

	// Body is the comment text.
	Body string `json:"body"`
}

// BackupRelationshipRecord is a snapshot of a single directed relationship.
type BackupRelationshipRecord struct {
	// TargetID is the ID of the related issue.
	TargetID string `json:"target_id"`

	// RelType is the relationship type string (e.g. "blocked_by").
	RelType string `json:"rel_type"`
}

// BackupClaimRecord is a snapshot of an active claim on an issue.
type BackupClaimRecord struct {
	// ClaimSHA512 is the hex-encoded SHA-512 hash of the original claim
	// token. The plaintext token is not recoverable from the backup.
	ClaimSHA512 string `json:"claim_sha512"`

	// Author is the claim holder's name.
	Author string `json:"author"`

	// StaleThreshold is the claim duration in nanoseconds, derived from
	// StaleAt minus ClaimedAt. The JSON field name is retained for backup
	// format compatibility.
	StaleThreshold int64 `json:"stale_threshold"`

	// LastActivity is the timestamp when the claim was created (claimedAt).
	// The JSON field name is retained for backup format compatibility.
	LastActivity time.Time `json:"last_activity"`
}

// BackupHistoryRecord is a snapshot of a single history entry.
type BackupHistoryRecord struct {
	// EntryID is the auto-increment integer ID.
	EntryID int64 `json:"entry_id"`

	// Revision is the zero-based revision index within the issue's
	// history.
	Revision int `json:"revision"`

	// Author is the actor who performed the mutation.
	Author string `json:"author"`

	// Timestamp is when the mutation occurred.
	Timestamp time.Time `json:"timestamp"`

	// EventType is the event type string (e.g. "created", "updated").
	EventType string `json:"event_type"`

	// Changes is the list of field-level changes recorded by this entry.
	Changes []BackupFieldChangeRecord `json:"changes"`
}

// BackupFieldChangeRecord is a single field-level before/after pair within a
// history entry.
type BackupFieldChangeRecord struct {
	// Field is the name of the changed field.
	Field string `json:"field"`

	// Before is the value before the change.
	Before string `json:"before"`

	// After is the value after the change.
	After string `json:"after"`
}
