package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/comment"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
	"github.com/pinkhop/nitpicking/internal/domain/port"
)

// serviceImpl implements the Service interface.
type serviceImpl struct {
	tx port.Transactor
}

// New creates a new Service backed by the given Transactor.
func New(tx port.Transactor) Service {
	return &serviceImpl{tx: tx}
}

// --- Global Operations ---

func (s *serviceImpl) Init(ctx context.Context, prefix string) error {
	if err := issue.ValidatePrefix(prefix); err != nil {
		return err
	}
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		return uow.Database().InitDatabase(ctx, prefix)
	})
}

func (s *serviceImpl) AgentName(_ context.Context) (string, error) {
	return identity.GenerateAgentName(), nil
}

func (s *serviceImpl) AgentInstructions(_ context.Context) (string, error) {
	return identity.AgentInstructions(), nil
}

func (s *serviceImpl) GetPrefix(ctx context.Context) (string, error) {
	var prefix string
	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		p, err := uow.Database().GetPrefix(ctx)
		if err != nil {
			return err
		}
		prefix = p
		return nil
	})
	return prefix, err
}

// --- Issue Operations ---

func (s *serviceImpl) CreateIssue(ctx context.Context, input CreateIssueInput) (CreateIssueOutput, error) {
	var output CreateIssueOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		// Check idempotency key.
		if input.IdempotencyKey != "" {
			existing, err := uow.Issues().GetIssueByIdempotencyKey(ctx, input.IdempotencyKey)
			if err == nil {
				output.Issue = existing
				return nil
			}
			if !errors.Is(err, domain.ErrNotFound) {
				return err
			}
		}

		// Get prefix and generate ID.
		prefix, err := uow.Database().GetPrefix(ctx)
		if err != nil {
			return fmt.Errorf("getting prefix: %w", err)
		}

		id, err := issue.GenerateID(prefix, func(id issue.ID) (bool, error) {
			return uow.Issues().IssueIDExists(ctx, id)
		})
		if err != nil {
			return err
		}

		now := time.Now()

		// Create issue.
		var t issue.Issue
		switch input.Role {
		case issue.RoleTask:
			t, err = issue.NewTask(issue.NewTaskParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           input.Priority,
				ParentID:           input.ParentID,
				Dimensions:         issue.DimensionSetFrom(input.Dimensions),
				CreatedAt:          now,
				IdempotencyKey:     input.IdempotencyKey,
			})
		case issue.RoleEpic:
			t, err = issue.NewEpic(issue.NewEpicParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           input.Priority,
				ParentID:           input.ParentID,
				Dimensions:         issue.DimensionSetFrom(input.Dimensions),
				CreatedAt:          now,
				IdempotencyKey:     input.IdempotencyKey,
			})
		default:
			return domain.NewValidationError("role", "must be task or epic")
		}
		if err != nil {
			return err
		}

		// Validate parent if set.
		if !input.ParentID.IsZero() {
			if err := s.validateParent(ctx, uow, t.ID(), input.ParentID); err != nil {
				return err
			}
		}

		if err := uow.Issues().CreateIssue(ctx, t); err != nil {
			return err
		}

		// Record creation history.
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   id,
			Revision:  0,
			Author:    input.Author,
			Timestamp: now,
			EventType: history.EventCreated,
			Changes: []history.FieldChange{
				{Field: "title", After: input.Title},
				{Field: "role", After: input.Role.String()},
				{Field: "priority", After: t.Priority().String()},
				{Field: "state", After: t.State().String()},
			},
		}))
		if err != nil {
			return err
		}

		// Add relationships.
		for _, ri := range input.Relationships {
			rel, err := issue.NewRelationship(id, ri.TargetID, ri.Type)
			if err != nil {
				return err
			}
			// Validate target exists and is not deleted.
			target, err := uow.Issues().GetIssue(ctx, ri.TargetID, false)
			if err != nil {
				return fmt.Errorf("relationship target %s: %w", ri.TargetID, err)
			}
			_ = target

			if _, err := uow.Relationships().CreateRelationship(ctx, rel); err != nil {
				return err
			}
		}

		// Optionally claim.
		if input.Claim {
			c, err := claim.NewClaim(claim.NewClaimParams{
				IssueID: id,
				Author:  input.Author,
				Now:     now,
			})
			if err != nil {
				return err
			}

			t = t.WithState(issue.StateClaimed)
			if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
				return err
			}
			if err := uow.Claims().CreateClaim(ctx, c); err != nil {
				return err
			}

			revision, _ := uow.History().CountHistory(ctx, id)
			_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
				IssueID:   id,
				Revision:  revision,
				Author:    input.Author,
				Timestamp: now,
				EventType: history.EventClaimed,
			}))
			if err != nil {
				return err
			}

			output.ClaimID = c.ID()
		}

		output.Issue = t
		return nil
	})

	return output, err
}

func (s *serviceImpl) ClaimByID(ctx context.Context, input ClaimInput) (ClaimOutput, error) {
	var output ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		result, err := s.claimWithinTx(ctx, uow, input)
		if err != nil {
			return err
		}
		output = result
		return nil
	})

	return output, err
}

// claimWithinTx performs the claim logic using an already-open UnitOfWork.
// Both ClaimByID and ClaimNextReady delegate here so that ClaimNextReady
// can list issues and claim within a single transaction rather than nesting.
func (s *serviceImpl) claimWithinTx(ctx context.Context, uow port.UnitOfWork, input ClaimInput) (ClaimOutput, error) {
	var output ClaimOutput
	now := time.Now()

	t, err := uow.Issues().GetIssue(ctx, input.IssueID, true)
	if err != nil {
		return output, err
	}

	// Build claim status.
	activeClaim, err := uow.Claims().GetClaimByIssue(ctx, input.IssueID)
	if err != nil && err != domain.ErrNotFound {
		return output, err
	}

	status := claim.IssueClaimStatus{
		State:       t.State(),
		IsDeleted:   t.IsDeleted(),
		ActiveClaim: activeClaim,
	}

	if err := claim.ValidateClaim(status, input.AllowSteal, now); err != nil {
		return output, err
	}

	// Steal if needed.
	if activeClaim.ID() != "" {
		output.Stolen = true
		previousHolder := activeClaim.Author().String()

		// Invalidate old claim.
		if err := uow.Claims().InvalidateClaim(ctx, activeClaim.ID()); err != nil {
			return output, err
		}

		// Add steal comment.
		stealComment, err := comment.NewComment(comment.NewCommentParams{
			IssueID:   input.IssueID,
			Author:    input.Author,
			CreatedAt: now,
			Body:      claim.StealComment(previousHolder),
		})
		if err != nil {
			return output, err
		}
		if _, err := uow.Comments().CreateComment(ctx, stealComment); err != nil {
			return output, err
		}
	}

	// Create new claim.
	c, err := claim.NewClaim(claim.NewClaimParams{
		IssueID:        input.IssueID,
		Author:         input.Author,
		StaleThreshold: input.StaleThreshold,
		Now:            now,
	})
	if err != nil {
		return output, err
	}

	// Update issue state to claimed.
	t = t.WithState(issue.StateClaimed)
	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return output, err
	}
	if err := uow.Claims().CreateClaim(ctx, c); err != nil {
		return output, err
	}

	// Record claim history.
	revision, _ := uow.History().CountHistory(ctx, input.IssueID)
	_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   input.IssueID,
		Revision:  revision,
		Author:    input.Author,
		Timestamp: now,
		EventType: history.EventClaimed,
	}))
	if err != nil {
		return output, err
	}

	output.ClaimID = c.ID()
	output.IssueID = input.IssueID
	return output, nil
}

func (s *serviceImpl) ClaimNextReady(ctx context.Context, input ClaimNextReadyInput) (ClaimOutput, error) {
	var output ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		filter := port.IssueFilter{
			Ready:            true,
			Role:             input.Role,
			DimensionFilters: input.DimensionFilters,
		}

		items, _, err := uow.Issues().ListIssues(ctx, filter, port.OrderByPriority, 1)
		if err != nil {
			return err
		}

		if len(items) > 0 {
			result, err := s.claimWithinTx(ctx, uow, ClaimInput{
				IssueID:        items[0].ID,
				Author:         input.Author,
				StaleThreshold: input.StaleThreshold,
			})
			if err != nil {
				return err
			}
			output = result
			return nil
		}

		// No ready issues — try stealing a stale claim if requested.
		if !input.StealIfNeeded {
			return domain.ErrNotFound
		}

		staleClaims, err := uow.Claims().ListStaleClaims(ctx, time.Now())
		if err != nil {
			return err
		}

		if len(staleClaims) == 0 {
			return domain.ErrNotFound
		}

		// Steal the highest-priority stale issue.
		result, err := s.claimWithinTx(ctx, uow, ClaimInput{
			IssueID:        staleClaims[0].IssueID(),
			Author:         input.Author,
			AllowSteal:     true,
			StaleThreshold: input.StaleThreshold,
		})
		if err != nil {
			return err
		}
		output = result
		return nil
	})

	return output, err
}

func (s *serviceImpl) OneShotUpdate(ctx context.Context, input OneShotUpdateInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Claim within the existing transaction.
		claimResult, err := s.claimWithinTx(ctx, uow, ClaimInput{
			IssueID: input.IssueID,
			Author:  input.Author,
		})
		if err != nil {
			return err
		}

		// Update.
		if err := s.applyIssueUpdates(ctx, uow, input.IssueID, claimResult.ClaimID, input.Author, now, oneShotToUpdateFields(input)); err != nil {
			return err
		}

		// Release.
		return s.releaseIssue(ctx, uow, input.IssueID, claimResult.ClaimID, input.Author, now)
	})
}

func (s *serviceImpl) UpdateIssue(ctx context.Context, input UpdateIssueInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Verify claim.
		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != input.IssueID {
			return fmt.Errorf("claim %s does not match issue %s", input.ClaimID, input.IssueID)
		}

		if err := s.applyIssueUpdates(ctx, uow, input.IssueID, input.ClaimID, c.Author(), now, updateFieldsFromInput(input)); err != nil {
			return err
		}

		// Add comment if provided.
		if input.NoteBody != "" {
			n, err := comment.NewComment(comment.NewCommentParams{
				IssueID:   input.IssueID,
				Author:    c.Author(),
				CreatedAt: now,
				Body:      input.NoteBody,
			})
			if err != nil {
				return err
			}
			if _, err := uow.Comments().CreateComment(ctx, n); err != nil {
				return err
			}
		}

		// Update claim last activity.
		return uow.Claims().UpdateClaimLastActivity(ctx, input.ClaimID, now)
	})
}

func (s *serviceImpl) ExtendStaleThreshold(ctx context.Context, issueID issue.ID, claimID string, threshold time.Duration) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		c, err := uow.Claims().GetClaimByID(ctx, claimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != issueID {
			return fmt.Errorf("claim %s does not match issue %s", claimID, issueID)
		}
		return uow.Claims().UpdateClaimThreshold(ctx, claimID, threshold)
	})
}

func (s *serviceImpl) TransitionState(ctx context.Context, input TransitionInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != input.IssueID {
			return fmt.Errorf("claim %s does not match issue %s", input.ClaimID, input.IssueID)
		}

		t, err := uow.Issues().GetIssue(ctx, input.IssueID, false)
		if err != nil {
			return err
		}

		switch input.Action {
		case ActionRelease:
			return s.releaseIssue(ctx, uow, input.IssueID, input.ClaimID, c.Author(), now)
		case ActionClose:
			return s.closeIssue(ctx, uow, t, input.ClaimID, c.Author(), now)
		case ActionDefer:
			return s.transitionIssue(ctx, uow, t, input.ClaimID, c.Author(), now, issue.StateDeferred)
		default:
			return fmt.Errorf("invalid transition action")
		}
	})
}

func (s *serviceImpl) DeleteIssue(ctx context.Context, input DeleteInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != input.IssueID {
			return fmt.Errorf("claim %s does not match issue %s", input.ClaimID, input.IssueID)
		}

		t, err := uow.Issues().GetIssue(ctx, input.IssueID, false)
		if err != nil {
			return err
		}

		if err := issue.ValidateDeletion(t.IsDeleted()); err != nil {
			return err
		}

		// For epics, check descendants.
		if t.IsEpic() {
			descendants, err := uow.Issues().GetDescendants(ctx, input.IssueID)
			if err != nil {
				return err
			}

			result := issue.PlanEpicDeletion(input.IssueID, descendants)
			if len(result.Conflicts) > 0 {
				return &domain.ClaimConflictError{
					IssueID:       input.IssueID.String(),
					CurrentHolder: result.Conflicts[0].ClaimedBy,
					StaleAt:       time.Now(),
				}
			}

			// Delete all descendants.
			for _, id := range result.ToDelete {
				if id == input.IssueID {
					continue
				}
				descendant, err := uow.Issues().GetIssue(ctx, id, false)
				if err != nil {
					continue
				}
				deleted := descendant.WithDeleted()
				if err := uow.Issues().UpdateIssue(ctx, deleted); err != nil {
					return err
				}

				revision, _ := uow.History().CountHistory(ctx, id)
				_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
					IssueID:   id,
					Revision:  revision,
					Author:    c.Author(),
					Timestamp: now,
					EventType: history.EventDeleted,
				}))
				if err != nil {
					return err
				}
			}
		}

		// Delete the issue itself.
		deleted := t.WithDeleted()
		if err := uow.Issues().UpdateIssue(ctx, deleted); err != nil {
			return err
		}
		if err := uow.Claims().InvalidateClaim(ctx, input.ClaimID); err != nil {
			return err
		}

		revision, _ := uow.History().CountHistory(ctx, input.IssueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   input.IssueID,
			Revision:  revision,
			Author:    c.Author(),
			Timestamp: now,
			EventType: history.EventDeleted,
		}))
		return err
	})
}

func (s *serviceImpl) ShowIssue(ctx context.Context, id issue.ID) (ShowIssueOutput, error) {
	var output ShowIssueOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		t, err := uow.Issues().GetIssue(ctx, id, false)
		if err != nil {
			return err
		}

		output.Issue = t

		// Revision and author from history.
		histCount, _ := uow.History().CountHistory(ctx, id)
		output.Revision = max(0, histCount-1)

		latest, err := uow.History().GetLatestHistory(ctx, id)
		if err == nil {
			output.Author = latest.Author()
		}

		// Relationships.
		rels, err := uow.Relationships().ListRelationships(ctx, id)
		if err == nil {
			output.Relationships = rels
		}

		// Readiness.
		blockers, _ := uow.Relationships().GetBlockerStatuses(ctx, id)
		ancestors, _ := uow.Issues().GetAncestorStatuses(ctx, id)

		if t.IsTask() {
			output.IsReady = issue.IsTaskReady(t.State(), blockers, ancestors)
		} else {
			hasChildren, _ := uow.Issues().HasChildren(ctx, id)
			output.IsReady = issue.IsEpicReady(t.State(), hasChildren, blockers, ancestors)

			// Completion.
		}

		// Comment count — use unlimited listing to count all comments.
		allComments, _, commentErr := uow.Comments().ListComments(ctx, id, port.CommentFilter{}, -1)
		if commentErr == nil {
			output.CommentCount = len(allComments)
		}

		// Child count.
		children, _, childErr := uow.Issues().ListIssues(ctx,
			port.IssueFilter{ParentID: id}, port.OrderByPriority, -1)
		if childErr == nil {
			output.ChildCount = len(children)
		}

		// Claim info.
		activeClaim, err := uow.Claims().GetClaimByIssue(ctx, id)
		if err == nil {
			output.ClaimID = activeClaim.ID()
			output.ClaimAuthor = activeClaim.Author().String()
			output.ClaimStaleAt = activeClaim.StaleAt()
		}

		return nil
	})

	return output, err
}

func (s *serviceImpl) ListIssues(ctx context.Context, input ListIssuesInput) (ListIssuesOutput, error) {
	var output ListIssuesOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		items, hasMore, err := uow.Issues().ListIssues(ctx, input.Filter, input.OrderBy, input.Limit)
		if err != nil {
			return err
		}
		output.Items = items
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchIssues(ctx context.Context, input SearchIssuesInput) (ListIssuesOutput, error) {
	var output ListIssuesOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		items, hasMore, err := uow.Issues().SearchIssues(ctx, input.Query, input.Filter, input.OrderBy, input.Limit)
		if err != nil {
			return err
		}
		output.Items = items
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// --- Dimension Operations ---

func (s *serviceImpl) ListDistinctDimensions(ctx context.Context) ([]issue.Dimension, error) {
	var dims []issue.Dimension
	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		var queryErr error
		dims, queryErr = uow.Issues().ListDistinctDimensions(ctx)
		return queryErr
	})
	return dims, err
}

// --- Relationship Operations ---

func (s *serviceImpl) AddRelationship(ctx context.Context, sourceID issue.ID, ri RelationshipInput, author identity.Author) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Validate target exists and is not deleted.
		if _, err := uow.Issues().GetIssue(ctx, ri.TargetID, false); err != nil {
			return fmt.Errorf("relationship target %s: %w", ri.TargetID, err)
		}

		rel, err := issue.NewRelationship(sourceID, ri.TargetID, ri.Type)
		if err != nil {
			return err
		}

		created, err := uow.Relationships().CreateRelationship(ctx, rel)
		if err != nil {
			return err
		}

		if created {
			revision, _ := uow.History().CountHistory(ctx, sourceID)
			_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
				IssueID:   sourceID,
				Revision:  revision,
				Author:    author,
				Timestamp: now,
				EventType: history.EventRelationshipAdded,
				Changes: []history.FieldChange{
					{Field: "relationship", After: fmt.Sprintf("%s:%s", ri.Type, ri.TargetID)},
				},
			}))
			return err
		}

		return nil
	})
}

func (s *serviceImpl) RemoveRelationship(ctx context.Context, sourceID issue.ID, ri RelationshipInput, author identity.Author) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		deleted, err := uow.Relationships().DeleteRelationship(ctx, sourceID, ri.TargetID, ri.Type)
		if err != nil {
			return err
		}

		if deleted {
			revision, _ := uow.History().CountHistory(ctx, sourceID)
			_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
				IssueID:   sourceID,
				Revision:  revision,
				Author:    author,
				Timestamp: now,
				EventType: history.EventRelationshipRemoved,
				Changes: []history.FieldChange{
					{Field: "relationship", Before: fmt.Sprintf("%s:%s", ri.Type, ri.TargetID)},
				},
			}))
			return err
		}

		return nil
	})
}

// --- Comment Operations ---

func (s *serviceImpl) AddComment(ctx context.Context, input AddCommentInput) (AddCommentOutput, error) {
	var output AddCommentOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Verify issue exists and is not deleted.
		t, err := uow.Issues().GetIssue(ctx, input.IssueID, true)
		if err != nil {
			return err
		}
		if t.IsDeleted() {
			return fmt.Errorf("cannot add comment to deleted issue: %w", domain.ErrDeletedIssue)
		}

		n, err := comment.NewComment(comment.NewCommentParams{
			IssueID:   input.IssueID,
			Author:    input.Author,
			CreatedAt: now,
			Body:      input.Body,
		})
		if err != nil {
			return err
		}

		id, err := uow.Comments().CreateComment(ctx, n)
		if err != nil {
			return err
		}

		// Update claim last activity if claimed.
		activeClaim, err := uow.Claims().GetClaimByIssue(ctx, input.IssueID)
		if err == nil {
			_ = uow.Claims().UpdateClaimLastActivity(ctx, activeClaim.ID(), now)
		}

		output.Comment, err = uow.Comments().GetComment(ctx, id)
		return err
	})

	return output, err
}

func (s *serviceImpl) ShowComment(ctx context.Context, commentID int64) (comment.Comment, error) {
	var result comment.Comment

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		n, err := uow.Comments().GetComment(ctx, commentID)
		if err != nil {
			return err
		}
		result = n
		return nil
	})

	return result, err
}

func (s *serviceImpl) ListComments(ctx context.Context, input ListCommentsInput) (ListCommentsOutput, error) {
	var output ListCommentsOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		comments, hasMore, err := uow.Comments().ListComments(ctx, input.IssueID, input.Filter, input.Limit)
		if err != nil {
			return err
		}
		output.Comments = comments
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchComments(ctx context.Context, input SearchCommentsInput) (ListCommentsOutput, error) {
	var output ListCommentsOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		filter := input.Filter
		filter.IssueID = input.IssueID

		comments, hasMore, err := uow.Comments().SearchComments(ctx, input.Query, filter, input.Limit)
		if err != nil {
			return err
		}
		output.Comments = comments
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// --- History Operations ---

func (s *serviceImpl) ShowHistory(ctx context.Context, input ListHistoryInput) (ListHistoryOutput, error) {
	var output ListHistoryOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		entries, hasMore, err := uow.History().ListHistory(ctx, input.IssueID, input.Filter, input.Limit)
		if err != nil {
			return err
		}
		output.Entries = entries
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// --- Graph ---

func (s *serviceImpl) GetGraphData(ctx context.Context) (GraphDataOutput, error) {
	var output GraphDataOutput
	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		// Fetch all non-deleted issues (unlimited).
		items, _, err := uow.Issues().ListIssues(ctx, port.IssueFilter{}, port.OrderByPriority, -1)
		if err != nil {
			return err
		}
		output.Nodes = items

		// Collect relationships for all issues. Use a set to deduplicate
		// since ListRelationships returns both directions.
		seen := make(map[string]bool)
		for _, item := range items {
			rels, err := uow.Relationships().ListRelationships(ctx, item.ID)
			if err != nil {
				return err
			}
			for _, rel := range rels {
				key := rel.SourceID().String() + "-" + rel.Type().String() + "-" + rel.TargetID().String()
				if !seen[key] {
					seen[key] = true
					output.Relationships = append(output.Relationships, rel)
				}
			}
		}

		return nil
	})
	return output, err
}

// --- Diagnostics ---

func (s *serviceImpl) Doctor(ctx context.Context) (DoctorOutput, error) {
	var output DoctorOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Check stale claims.
		staleClaims, err := uow.Claims().ListStaleClaims(ctx, now)
		if err != nil {
			return err
		}
		for _, c := range staleClaims {
			output.Findings = append(output.Findings, DoctorFinding{
				Category: "stale_claim",
				Severity: "warning",
				Message:  fmt.Sprintf("Issue %s has been claimed by %q since %s (stale)", c.IssueID(), c.Author(), c.LastActivity().Format(time.RFC3339)),
				IssueIDs: []string{c.IssueID().String()},
			})
		}

		// Check for no-ready-issues condition.
		readinessFindings, readErr := s.checkReadiness(ctx, uow)
		if readErr != nil {
			return readErr
		}
		output.Findings = append(output.Findings, readinessFindings...)

		return nil
	})

	return output, err
}

// checkReadiness detects when no issues are ready for work and analyzes the
// causes. It produces actionable findings when blocked or deferred issues
// prevent any work from being picked up.
func (s *serviceImpl) checkReadiness(ctx context.Context, uow port.UnitOfWork) ([]DoctorFinding, error) {
	// Check whether any ready issues exist.
	readyItems, _, err := uow.Issues().ListIssues(ctx, port.IssueFilter{
		Ready:         true,
		ExcludeClosed: true,
	}, port.OrderByPriority, 1)
	if err != nil {
		return nil, fmt.Errorf("listing ready issues: %w", err)
	}
	if len(readyItems) > 0 {
		return nil, nil
	}

	// Check whether any non-closed issues exist at all.
	allItems, _, err := uow.Issues().ListIssues(ctx, port.IssueFilter{
		ExcludeClosed: true,
	}, port.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing all issues: %w", err)
	}
	if len(allItems) == 0 {
		return nil, nil
	}

	var findings []DoctorFinding

	// Identify blocked issues and analyze their blockers.
	blockedItems, _, err := uow.Issues().ListIssues(ctx, port.IssueFilter{
		Blocked:       true,
		ExcludeClosed: true,
	}, port.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing blocked issues: %w", err)
	}

	// Track close-eligible epics and deferred blockers to deduplicate findings.
	closeEligibleEpics := make(map[string]bool)
	deferredBlockers := make(map[string]bool)

	for _, blocked := range blockedItems {
		rels, relErr := uow.Relationships().ListRelationships(ctx, blocked.ID)
		if relErr != nil {
			return nil, fmt.Errorf("listing relationships for %s: %w", blocked.ID, relErr)
		}

		for _, rel := range rels {
			if rel.Type() != issue.RelBlockedBy || rel.SourceID() != blocked.ID {
				continue
			}
			targetID := rel.TargetID()

			blocker, getErr := uow.Issues().GetIssue(ctx, targetID, false)
			if getErr != nil {
				continue // blocker deleted or not found — already resolved
			}

			if blocker.State() == issue.StateClosed {
				continue
			}

			// Check if the blocker is deferred.
			if blocker.State() == issue.StateDeferred && !deferredBlockers[targetID.String()] {
				deferredBlockers[targetID.String()] = true
			}

			// Check if the blocker is a close-eligible epic.
			if blocker.IsEpic() && !closeEligibleEpics[targetID.String()] {
				children, childErr := uow.Issues().GetChildStatuses(ctx, targetID)
				if childErr != nil {
					continue
				}
				if isCloseEligible(children) {
					closeEligibleEpics[targetID.String()] = true
				}
			}
		}
	}

	// Emit findings for close-eligible epic blockers.
	for epicID := range closeEligibleEpics {
		findings = append(findings, DoctorFinding{
			Category:   "close_eligible_blocker",
			Severity:   "warning",
			Message:    fmt.Sprintf("Epic %s has all children closed but is still open, blocking other issues.", epicID),
			IssueIDs:   []string{epicID},
			Suggestion: "Run 'np epic close-eligible --author <name>' to batch-close fully resolved epics.",
		})
	}

	// Emit findings for deferred blockers.
	for blockerID := range deferredBlockers {
		findings = append(findings, DoctorFinding{
			Category:   "deferred_blocker",
			Severity:   "warning",
			Message:    fmt.Sprintf("Issue %s is deferred and blocking other issues.", blockerID),
			IssueIDs:   []string{blockerID},
			Suggestion: fmt.Sprintf("Run 'np issue undefer %s --author <name>' to restore it.", blockerID),
		})
	}

	// Emit the summary finding if there are blocked issues.
	if len(blockedItems) > 0 {
		blockedIDs := make([]string, 0, len(blockedItems))
		for _, item := range blockedItems {
			blockedIDs = append(blockedIDs, item.ID.String())
		}
		findings = append(findings, DoctorFinding{
			Category: "no_ready_issues",
			Severity: "warning",
			Message:  fmt.Sprintf("No issues are ready for work. %d issues are blocked.", len(blockedItems)),
			IssueIDs: blockedIDs,
		})
	}

	return findings, nil
}

// isCloseEligible reports whether an epic's children are all closed, making
// the epic eligible for closure. An epic with no children is not eligible.
func isCloseEligible(children []issue.ChildStatus) bool {
	if len(children) == 0 {
		return false
	}
	for _, c := range children {
		if c.State != issue.StateClosed {
			return false
		}
	}
	return true
}

func (s *serviceImpl) GC(ctx context.Context, input GCInput) (GCOutput, error) {
	var output GCOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		return uow.Database().GC(ctx, input.IncludeClosed)
	})

	return output, err
}

// --- Internal helpers ---

func (s *serviceImpl) validateParent(ctx context.Context, uow port.UnitOfWork, childID, parentID issue.ID) error {
	parent, err := uow.Issues().GetIssue(ctx, parentID, true)
	if err != nil {
		return fmt.Errorf("parent %s: %w", parentID, err)
	}
	if err := issue.ValidateParent(childID, parentID, parent.IsDeleted()); err != nil {
		return err
	}
	ancestorLookup := func(id issue.ID) (issue.ID, error) {
		return uow.Issues().GetParentID(ctx, id)
	}
	if err := issue.ValidateNoCycle(childID, parentID, ancestorLookup); err != nil {
		return err
	}
	return issue.ValidateDepth(parentID, ancestorLookup)
}

func (s *serviceImpl) releaseIssue(ctx context.Context, uow port.UnitOfWork, issueID issue.ID, claimID string, author identity.Author, now time.Time) error {
	t, err := uow.Issues().GetIssue(ctx, issueID, false)
	if err != nil {
		return err
	}

	releaseState := issue.ReleaseState()

	t = t.WithState(releaseState)
	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, issueID)
	_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   issueID,
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventReleased,
		Changes: []history.FieldChange{
			{Field: "state", Before: issue.StateClaimed.String(), After: releaseState.String()},
		},
	}))
	return err
}

func (s *serviceImpl) closeIssue(ctx context.Context, uow port.UnitOfWork, t issue.Issue, claimID string, author identity.Author, now time.Time) error {
	if err := issue.Transition(t.State(), issue.StateClosed); err != nil {
		return err
	}

	// Ensure all children are closed before allowing close.
	children, err := uow.Issues().GetChildStatuses(ctx, t.ID())
	if err != nil {
		return fmt.Errorf("checking children: %w", err)
	}
	for _, child := range children {
		if child.State != issue.StateClosed {
			return fmt.Errorf("cannot close issue with unclosed children: %w", domain.ErrIllegalTransition)
		}
	}

	t = t.WithState(issue.StateClosed)
	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, t.ID())
	_, histErr := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   t.ID(),
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventStateChanged,
		Changes: []history.FieldChange{
			{Field: "state", Before: issue.StateClaimed.String(), After: issue.StateClosed.String()},
		},
	}))
	return histErr
}

func (s *serviceImpl) transitionIssue(ctx context.Context, uow port.UnitOfWork, t issue.Issue, claimID string, author identity.Author, now time.Time, targetState issue.State) error {
	if err := issue.Transition(t.State(), targetState); err != nil {
		return err
	}

	t = t.WithState(targetState)
	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, t.ID())
	_, err := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   t.ID(),
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventStateChanged,
		Changes: []history.FieldChange{
			{Field: "state", Before: issue.StateClaimed.String(), After: targetState.String()},
		},
	}))
	return err
}

// updateFields groups optional field updates for an issue.
type updateFields struct {
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *issue.Priority
	ParentID           *issue.ID
	DimensionSet       []issue.Dimension
	DimensionRemove    []string
}

func oneShotToUpdateFields(input OneShotUpdateInput) updateFields {
	return updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		ParentID:           input.ParentID,
		DimensionSet:       input.DimensionSet,
		DimensionRemove:    input.DimensionRemove,
	}
}

func updateFieldsFromInput(input UpdateIssueInput) updateFields {
	return updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		ParentID:           input.ParentID,
		DimensionSet:       input.DimensionSet,
		DimensionRemove:    input.DimensionRemove,
	}
}

func (s *serviceImpl) applyIssueUpdates(ctx context.Context, uow port.UnitOfWork, issueID issue.ID, claimID string, author identity.Author, now time.Time, fields updateFields) error {
	t, err := uow.Issues().GetIssue(ctx, issueID, false)
	if err != nil {
		return err
	}

	var changes []history.FieldChange

	if fields.Title != nil {
		old := t.Title()
		t, err = t.WithTitle(*fields.Title)
		if err != nil {
			return err
		}
		changes = append(changes, history.FieldChange{Field: "title", Before: old, After: *fields.Title})
	}

	if fields.Description != nil {
		old := t.Description()
		t = t.WithDescription(*fields.Description)
		changes = append(changes, history.FieldChange{Field: "description", Before: old, After: *fields.Description})
	}

	if fields.AcceptanceCriteria != nil {
		old := t.AcceptanceCriteria()
		t = t.WithAcceptanceCriteria(*fields.AcceptanceCriteria)
		changes = append(changes, history.FieldChange{Field: "acceptance_criteria", Before: old, After: *fields.AcceptanceCriteria})
	}

	if fields.Priority != nil {
		old := t.Priority()
		t = t.WithPriority(*fields.Priority)
		changes = append(changes, history.FieldChange{Field: "priority", Before: old.String(), After: fields.Priority.String()})
	}

	if fields.ParentID != nil {
		oldParentID := t.ParentID()
		newParentID := *fields.ParentID

		if !newParentID.IsZero() {
			if err := s.validateParent(ctx, uow, issueID, newParentID); err != nil {
				return err
			}
		}

		t = t.WithParentID(newParentID)
		changes = append(changes, history.FieldChange{
			Field:  "parent",
			Before: oldParentID.String(),
			After:  newParentID.String(),
		})
	}

	// Apply dimension changes.
	dimensions := t.Dimensions()
	for _, f := range fields.DimensionSet {
		dimensions = dimensions.Set(f)
	}
	for _, key := range fields.DimensionRemove {
		dimensions = dimensions.Remove(key)
	}
	t = t.WithDimensions(dimensions)

	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return err
	}

	if len(changes) > 0 {
		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventUpdated,
			Changes:   changes,
		}))
		if err != nil {
			return err
		}
	}

	// Update claim last activity.
	return uow.Claims().UpdateClaimLastActivity(ctx, claimID, now)
}
