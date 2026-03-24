package service

import (
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// --- Issue DTOs ---

// CreateIssueInput holds the parameters for creating an issue.
type CreateIssueInput struct {
	Role               issue.Role
	Title              string
	Description        string
	AcceptanceCriteria string
	Priority           issue.Priority
	ParentID           issue.ID
	Dimensions         []issue.Dimension
	Relationships      []RelationshipInput
	Author             identity.Author
	Claim              bool
	IdempotencyKey     string
}

// RelationshipInput describes a relationship to add during issue creation
// or update.
type RelationshipInput struct {
	Type     issue.RelationType
	TargetID issue.ID
}

// CreateIssueOutput holds the result of creating an issue.
type CreateIssueOutput struct {
	Issue   issue.Issue
	ClaimID string // Non-empty if the issue was created as claimed.
}

// ClaimInput holds the parameters for claiming an issue.
type ClaimInput struct {
	IssueID        issue.ID
	Author         identity.Author
	AllowSteal     bool
	StaleThreshold time.Duration
}

// ClaimOutput holds the result of claiming an issue.
type ClaimOutput struct {
	ClaimID string
	IssueID issue.ID
	Stolen  bool
}

// ClaimNextReadyInput holds the parameters for claiming the next ready issue.
type ClaimNextReadyInput struct {
	Author           identity.Author
	Role             issue.Role
	DimensionFilters []port.DimensionFilter
	StealIfNeeded    bool
	StaleThreshold   time.Duration
}

// UpdateIssueInput holds the parameters for updating a claimed issue.
type UpdateIssueInput struct {
	IssueID            issue.ID
	ClaimID            string
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *issue.Priority
	ParentID           *issue.ID
	DimensionSet       []issue.Dimension
	DimensionRemove    []string
	RelationshipAdd    []RelationshipInput
	RelationshipRemove []RelationshipInput
	NoteBody           string
}

// OneShotUpdateInput holds the parameters for an atomic claim→update→release.
type OneShotUpdateInput struct {
	IssueID            issue.ID
	Author             identity.Author
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *issue.Priority
	ParentID           *issue.ID
	DimensionSet       []issue.Dimension
	DimensionRemove    []string
}

// TransitionInput holds the parameters for a state transition.
type TransitionInput struct {
	IssueID issue.ID
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

	// ActionDefer shelves the issue.
	ActionDefer

	// ActionWait marks the issue as externally blocked.
	ActionWait
)

// DeleteInput holds the parameters for soft-deleting an issue.
type DeleteInput struct {
	IssueID issue.ID
	ClaimID string
}

// ShowIssueOutput holds the full detail view of an issue.
type ShowIssueOutput struct {
	Issue         issue.Issue
	Revision      int
	Author        identity.Author
	Relationships []issue.Relationship
	IsReady       bool
	IsComplete    bool // Only meaningful for epics.
	NoteCount     int
	ClaimID       string
	ClaimAuthor   string
	ClaimStaleAt  time.Time
}

// ListIssuesInput holds the parameters for listing issues.
type ListIssuesInput struct {
	Filter  port.IssueFilter
	OrderBy port.IssueOrderBy
	Page    port.PageRequest
}

// ListIssuesOutput holds the result of listing issues.
type ListIssuesOutput struct {
	Items      []port.IssueListItem
	TotalCount int
}

// SearchIssuesInput holds the parameters for searching issues.
type SearchIssuesInput struct {
	Query        string
	Filter       port.IssueFilter
	OrderBy      port.IssueOrderBy
	Page         port.PageRequest
	IncludeNotes bool
}

// --- Note DTOs ---

// AddNoteInput holds the parameters for adding a note.
type AddNoteInput struct {
	IssueID issue.ID
	Author  identity.Author
	Body    string
}

// AddNoteOutput holds the result of adding a note.
type AddNoteOutput struct {
	Note note.Note
}

// ListNotesInput holds the parameters for listing notes.
type ListNotesInput struct {
	IssueID issue.ID
	Filter  port.NoteFilter
	Page    port.PageRequest
}

// ListNotesOutput holds the result of listing notes.
type ListNotesOutput struct {
	Notes      []note.Note
	TotalCount int
}

// SearchNotesInput holds the parameters for searching notes.
type SearchNotesInput struct {
	Query   string
	IssueID issue.ID // Zero for global search.
	Filter  port.NoteFilter
	Page    port.PageRequest
}

// --- History DTOs ---

// ListHistoryInput holds the parameters for listing history.
type ListHistoryInput struct {
	IssueID issue.ID
	Filter  port.HistoryFilter
	Page    port.PageRequest
}

// ListHistoryOutput holds the result of listing history.
type ListHistoryOutput struct {
	Entries    []history.Entry
	TotalCount int
}

// --- Diagnostics DTOs ---

// DoctorFinding represents a single diagnostic finding.
type DoctorFinding struct {
	// Category identifies the kind of finding.
	Category string
	// Severity is "warning" or "error".
	Severity string
	// Message describes the finding.
	Message string
	// IssueIDs lists affected issues.
	IssueIDs []string
}

// DoctorOutput holds the results of the doctor diagnostic.
type DoctorOutput struct {
	Findings []DoctorFinding
}

// GraphDataOutput holds the data needed to render an issue graph.
type GraphDataOutput struct {
	// Nodes contains all non-deleted issues as lightweight projections.
	Nodes []port.IssueListItem
	// Relationships contains all relationships for the included issues.
	Relationships []issue.Relationship
}

// GCInput holds the parameters for garbage collection.
type GCInput struct {
	IncludeClosed bool
}

// GCOutput holds the result of garbage collection.
type GCOutput struct {
	DeletedIssuesRemoved int
	ClosedIssuesRemoved  int
}
