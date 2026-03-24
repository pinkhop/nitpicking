package service

import (
	"context"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
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

	// AgentInstructions returns Markdown instructions for agents.
	AgentInstructions(ctx context.Context) (string, error)

	// GetPrefix returns the database's configured issue ID prefix.
	GetPrefix(ctx context.Context) (string, error)

	// --- Issue Operations ---

	// CreateIssue creates a new issue.
	CreateIssue(ctx context.Context, input CreateIssueInput) (CreateIssueOutput, error)

	// ClaimByID claims a specific issue.
	ClaimByID(ctx context.Context, input ClaimInput) (ClaimOutput, error)

	// ClaimNextReady claims the highest-priority ready issue.
	ClaimNextReady(ctx context.Context, input ClaimNextReadyInput) (ClaimOutput, error)

	// OneShotUpdate performs an atomic claim→update→release.
	OneShotUpdate(ctx context.Context, input OneShotUpdateInput) error

	// UpdateIssue updates a claimed issue's fields.
	UpdateIssue(ctx context.Context, input UpdateIssueInput) error

	// ExtendStaleThreshold extends the stale threshold on an active claim.
	ExtendStaleThreshold(ctx context.Context, issueID issue.ID, claimID string, threshold time.Duration) error

	// TransitionState changes the state of a claimed issue.
	TransitionState(ctx context.Context, input TransitionInput) error

	// DeleteIssue soft-deletes a claimed issue.
	DeleteIssue(ctx context.Context, input DeleteInput) error

	// ShowIssue returns the full detail view of an issue.
	ShowIssue(ctx context.Context, id issue.ID) (ShowIssueOutput, error)

	// ListIssues returns a filtered, ordered, paginated list of issues.
	ListIssues(ctx context.Context, input ListIssuesInput) (ListIssuesOutput, error)

	// SearchIssues performs full-text search on issues.
	SearchIssues(ctx context.Context, input SearchIssuesInput) (ListIssuesOutput, error)

	// --- Relationship Operations ---

	// AddRelationship adds a relationship between two issues.
	AddRelationship(ctx context.Context, sourceID issue.ID, rel RelationshipInput, author identity.Author) error

	// RemoveRelationship removes a relationship between two issues.
	RemoveRelationship(ctx context.Context, sourceID issue.ID, rel RelationshipInput, author identity.Author) error

	// --- Comment Operations ---

	// AddComment adds a comment to an issue.
	AddComment(ctx context.Context, input AddCommentInput) (AddCommentOutput, error)

	// ShowComment retrieves a single comment by ID.
	ShowComment(ctx context.Context, commentID int64) (comment.Comment, error)

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

	// Doctor runs diagnostics and returns findings.
	Doctor(ctx context.Context) (DoctorOutput, error)

	// GC performs garbage collection.
	GC(ctx context.Context, input GCInput) (GCOutput, error)
}
