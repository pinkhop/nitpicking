package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// gcThresholdRatio is the fraction of deleted issues at which the doctor
// recommends running GC. When deleted issues exceed this ratio of total
// issues, a gc_recommended finding is emitted.
const gcThresholdRatio = 0.20

// serviceImpl implements the driving.Service interface.
type serviceImpl struct {
	tx driven.Transactor
}

// New creates a new driving.Service backed by the given Transactor.
func New(tx driven.Transactor) driving.Service {
	return &serviceImpl{tx: tx}
}

// parseAuthor converts a raw author string into a validated domain Author.
// Called at the service boundary to validate author names before they reach
// domain functions.
func parseAuthor(raw string) (domain.Author, error) {
	a, err := domain.NewAuthor(raw)
	if err != nil {
		return domain.Author{}, fmt.Errorf("invalid author: %w", err)
	}
	return a, nil
}

// --- Global Operations ---

func (s *serviceImpl) Init(ctx context.Context, prefix string) error {
	if err := domain.ValidatePrefix(prefix); err != nil {
		return err
	}
	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		return uow.Database().InitDatabase(ctx, prefix)
	})
}

func (s *serviceImpl) AgentName(_ context.Context) (string, error) {
	return domain.GenerateAgentName(), nil
}

func (s *serviceImpl) GetPrefix(ctx context.Context) (string, error) {
	var prefix string
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
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

func (s *serviceImpl) CreateIssue(ctx context.Context, input driving.CreateIssueInput) (driving.CreateIssueOutput, error) {
	author, err := parseAuthor(input.Author)
	if err != nil {
		return driving.CreateIssueOutput{}, err
	}

	var output driving.CreateIssueOutput

	err = s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
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

		id, err := domain.GenerateID(prefix, func(id domain.ID) (bool, error) {
			return uow.Issues().IssueIDExists(ctx, id)
		})
		if err != nil {
			return err
		}

		now := time.Now()

		// Convert label inputs to domain labels.
		domainLabels, err := toLabelSlice(input.Labels)
		if err != nil {
			return err
		}

		role := input.Role

		priority := input.Priority

		// Parse ParentID if provided.
		var parentID domain.ID
		if input.ParentID != "" {
			parentID, err = domain.ParseID(input.ParentID)
			if err != nil {
				return fmt.Errorf("invalid parent ID: %w", err)
			}
		}

		// Create domain.
		var t domain.Issue
		switch role {
		case domain.RoleTask:
			t, err = domain.NewTask(domain.NewTaskParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           priority,
				ParentID:           parentID,
				Labels:             domain.LabelSetFrom(domainLabels),
				CreatedAt:          now,
				IdempotencyKey:     input.IdempotencyKey,
			})
		case domain.RoleEpic:
			t, err = domain.NewEpic(domain.NewEpicParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           priority,
				ParentID:           parentID,
				Labels:             domain.LabelSetFrom(domainLabels),
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
		if input.ParentID != "" {
			if err := s.validateParent(ctx, uow, t.ID(), parentID, role); err != nil {
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
			Author:    author,
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
			targetID, err := domain.ParseID(ri.TargetID)
			if err != nil {
				return fmt.Errorf("relationship target ID: %w", err)
			}
			rel, err := domain.NewRelationship(id, targetID, ri.Type)
			if err != nil {
				return err
			}
			// Validate target exists and is not deleted.
			target, err := uow.Issues().GetIssue(ctx, targetID, false)
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
			c, err := domain.NewClaim(domain.NewClaimParams{
				IssueID: id,
				Author:  author,
				Now:     now,
			})
			if err != nil {
				return err
			}

			t = t.WithState(domain.StateClaimed)
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
				Author:    author,
				Timestamp: now,
				EventType: history.EventClaimed,
			}))
			if err != nil {
				return err
			}

			output.ClaimID = c.Token()
		}

		output.Issue = t
		return nil
	})

	return output, err
}

func (s *serviceImpl) ClaimByID(ctx context.Context, input driving.ClaimInput) (driving.ClaimOutput, error) {
	var output driving.ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		result, err := s.claimWithinTx(ctx, uow, input)
		if err != nil {
			return err
		}
		output = result
		return nil
	})

	return output, err
}

func (s *serviceImpl) LookupClaimIssueID(ctx context.Context, claimID string) (string, error) {
	var issueIDStr string

	err := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		c, err := uow.Claims().GetClaimByID(ctx, claimID)
		if err != nil {
			return err
		}
		issueIDStr = c.IssueID().String()
		return nil
	})

	return issueIDStr, err
}

func (s *serviceImpl) LookupClaimAuthor(ctx context.Context, claimID string) (string, error) {
	var authorStr string

	err := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		c, err := uow.Claims().GetClaimByID(ctx, claimID)
		if err != nil {
			return err
		}
		authorStr = c.Author().String()
		return nil
	})

	return authorStr, err
}

// claimWithinTx performs the claim logic using an already-open UnitOfWork.
// Both ClaimByID and ClaimNextReady delegate here so that ClaimNextReady
// can list issues and claim within a single transaction rather than nesting.
func (s *serviceImpl) claimWithinTx(ctx context.Context, uow driven.UnitOfWork, input driving.ClaimInput) (driving.ClaimOutput, error) {
	author, err := parseAuthor(input.Author)
	if err != nil {
		return driving.ClaimOutput{}, err
	}

	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return driving.ClaimOutput{}, err
	}

	var output driving.ClaimOutput
	now := time.Now()

	t, err := uow.Issues().GetIssue(ctx, issueID, true)
	if err != nil {
		return output, err
	}

	// Guard-rail assertions: when the caller provides label or role
	// filters (used by the unified claim command when claiming by ID),
	// verify the issue matches before proceeding.
	if input.Role != 0 && t.Role() != input.Role {
		return output, fmt.Errorf("issue %s does not have role %s", input.IssueID, input.Role)
	}
	for _, lf := range input.LabelFilters {
		val, ok := t.Labels().Get(lf.Key)
		if !ok {
			return output, fmt.Errorf("issue %s does not have label %s", input.IssueID, lf.Key+":*")
		}
		if lf.Value != "" && val != lf.Value {
			return output, fmt.Errorf("issue %s does not have label %s:%s", input.IssueID, lf.Key, lf.Value)
		}
	}

	// Build claim status.
	activeClaim, err := uow.Claims().GetClaimByIssue(ctx, issueID)
	if err != nil && err != domain.ErrNotFound {
		return output, err
	}

	status := IssueClaimStatus{
		State:       t.State(),
		IsDeleted:   t.IsDeleted(),
		ActiveClaim: activeClaim,
	}

	if err := ValidateClaim(status, input.AllowSteal, now); err != nil {
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
		stealComment, err := domain.NewComment(domain.NewCommentParams{
			IssueID:   issueID,
			Author:    author,
			CreatedAt: now,
			Body:      StealComment(previousHolder),
		})
		if err != nil {
			return output, err
		}
		if _, err := uow.Comments().CreateComment(ctx, stealComment); err != nil {
			return output, err
		}
	}

	// Create new claim.
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       issueID,
		Author:        author,
		StaleDuration: input.StaleThreshold,
		StaleAt:       input.StaleAt,
		Now:           now,
	})
	if err != nil {
		return output, err
	}

	// Update issue state to claimed.
	t = t.WithState(domain.StateClaimed)
	if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
		return output, err
	}
	if err := uow.Claims().CreateClaim(ctx, c); err != nil {
		return output, err
	}

	// Record claim history.
	revision, _ := uow.History().CountHistory(ctx, issueID)
	_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   issueID,
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventClaimed,
	}))
	if err != nil {
		return output, err
	}

	output.ClaimID = c.Token()
	output.IssueID = input.IssueID
	output.Author = input.Author
	output.CreatedAt = c.ClaimedAt()
	output.StaleAt = c.StaleAt()
	return output, nil
}

func (s *serviceImpl) ClaimNextReady(ctx context.Context, input driving.ClaimNextReadyInput) (driving.ClaimOutput, error) {
	var output driving.ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		filter := driven.IssueFilter{
			Ready:        true,
			LabelFilters: toPortLabelFilters(input.LabelFilters),
		}
		if input.Role != 0 {
			filter.Roles = []domain.Role{input.Role}
		}

		items, _, err := uow.Issues().ListIssues(ctx, filter, driven.OrderByPriority, 1)
		if err != nil {
			return err
		}

		if len(items) > 0 {
			result, err := s.claimWithinTx(ctx, uow, driving.ClaimInput{
				IssueID:        items[0].ID.String(),
				Author:         input.Author,
				StaleThreshold: input.StaleThreshold,
				StaleAt:        input.StaleAt,
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

		// Build a set of issue IDs with stale claims for fast lookup.
		staleIssueIDs := make(map[domain.ID]struct{}, len(staleClaims))
		for _, c := range staleClaims {
			staleIssueIDs[c.IssueID()] = struct{}{}
		}

		// Query claimed issues with the same label/role filters, ordered by
		// priority. This reuses the repository's filter logic rather than
		// reimplementing label/role matching in the core.
		stealFilter := driven.IssueFilter{
			States:       []domain.State{domain.StateClaimed},
			LabelFilters: toPortLabelFilters(input.LabelFilters),
		}
		if input.Role != 0 {
			stealFilter.Roles = []domain.Role{input.Role}
		}

		candidates, _, err := uow.Issues().ListIssues(ctx, stealFilter, driven.OrderByPriority, 0)
		if err != nil {
			return err
		}

		// Find the highest-priority candidate that also has a stale claim.
		for _, candidate := range candidates {
			if _, ok := staleIssueIDs[candidate.ID]; !ok {
				continue
			}
			result, err := s.claimWithinTx(ctx, uow, driving.ClaimInput{
				IssueID:        candidate.ID.String(),
				Author:         input.Author,
				AllowSteal:     true,
				StaleThreshold: input.StaleThreshold,
				StaleAt:        input.StaleAt,
			})
			if err != nil {
				return err
			}
			output = result
			return nil
		}

		return domain.ErrNotFound
	})

	return output, err
}

func (s *serviceImpl) OneShotUpdate(ctx context.Context, input driving.OneShotUpdateInput) error {
	parsedID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return domain.NewValidationError("issue_id", fmt.Sprintf("invalid issue ID %q: %s", input.IssueID, err))
	}

	author, err := parseAuthor(input.Author)
	if err != nil {
		return err
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Claim within the existing transaction.
		claimResult, err := s.claimWithinTx(ctx, uow, driving.ClaimInput{
			IssueID: input.IssueID,
			Author:  input.Author,
		})
		if err != nil {
			return err
		}

		// Update.
		fields, err := oneShotToUpdateFields(input)
		if err != nil {
			return err
		}
		if err := s.applyIssueUpdates(ctx, uow, parsedID, claimResult.ClaimID, author, now, claimResult.StaleAt, fields); err != nil {
			return err
		}

		// Release.
		return s.releaseIssue(ctx, uow, parsedID, claimResult.ClaimID, author, now)
	})
}

func (s *serviceImpl) UpdateIssue(ctx context.Context, input driving.UpdateIssueInput) error {
	parsedID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return domain.NewValidationError("issue_id", fmt.Sprintf("invalid issue ID %q: %s", input.IssueID, err))
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Verify claim.
		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != parsedID {
			return fmt.Errorf("claim does not match issue %s", input.IssueID)
		}

		fields, err := updateFieldsFromInput(input)
		if err != nil {
			return err
		}
		newStaleAt := now.Add(c.StaleAt().Sub(c.ClaimedAt()))

		// Add comment if provided.
		if input.CommentBody != "" {
			n, err := domain.NewComment(domain.NewCommentParams{
				IssueID:   parsedID,
				Author:    c.Author(),
				CreatedAt: now,
				Body:      input.CommentBody,
			})
			if err != nil {
				return err
			}
			if _, err := uow.Comments().CreateComment(ctx, n); err != nil {
				return err
			}
		}

		// Apply updates and extend claim staleAt.
		return s.applyIssueUpdates(ctx, uow, parsedID, input.ClaimID, c.Author(), now, newStaleAt, fields)
	})
}

func (s *serviceImpl) ExtendStaleThreshold(ctx context.Context, issueID string, claimID string, threshold time.Duration) error {
	parsedID, err := domain.ParseID(issueID)
	if err != nil {
		return domain.NewValidationError("issue_id", fmt.Sprintf("invalid issue ID %q: %s", issueID, err))
	}
	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		c, err := uow.Claims().GetClaimByID(ctx, claimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != parsedID {
			return fmt.Errorf("claim does not match issue %s", issueID)
		}
		// Compute new staleAt from the claim's original claimedAt and the
		// requested threshold.
		newStaleAt := c.ClaimedAt().Add(threshold)
		return uow.Claims().UpdateClaimStaleAt(ctx, claimID, newStaleAt)
	})
}

// CloseWithReason atomically adds a closing reason as a comment and
// transitions the issue to closed within a single transaction. The author
// is derived from the claim record.
func (s *serviceImpl) CloseWithReason(ctx context.Context, input driving.CloseWithReasonInput) error {
	if input.Reason == "" {
		return fmt.Errorf("reason is required: explain why the issue is being closed")
	}

	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return fmt.Errorf("invalid issue ID: %w", err)
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Validate the claim and derive the author.
		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != issueID {
			return fmt.Errorf("claim does not match issue %s", issueID)
		}
		author := c.Author()

		// Fetch the issue and verify it is not deleted.
		t, err := uow.Issues().GetIssue(ctx, issueID, true)
		if err != nil {
			return err
		}
		if t.IsDeleted() {
			return fmt.Errorf("cannot close deleted issue: %w", domain.ErrDeletedIssue)
		}

		// Validate the state transition before making any mutations.
		if err := domain.Transition(t.State(), domain.StateClosed); err != nil {
			return err
		}

		// Ensure all children are closed before allowing close.
		children, err := uow.Issues().GetChildStatuses(ctx, t.ID())
		if err != nil {
			return fmt.Errorf("checking children: %w", err)
		}
		for _, child := range children {
			if child.State != domain.StateClosed {
				return fmt.Errorf("cannot close issue with unclosed children: %w", domain.ErrIllegalTransition)
			}
		}

		// Step 1: Add the reason as a comment.
		n, err := domain.NewComment(domain.NewCommentParams{
			IssueID:   issueID,
			Author:    author,
			CreatedAt: now,
			Body:      input.Reason,
		})
		if err != nil {
			return err
		}

		commentID, err := uow.Comments().CreateComment(ctx, n)
		if err != nil {
			return err
		}

		// Record the comment in the history log.
		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventCommentAdded,
			Changes: []history.FieldChange{
				{Field: "comment_id", After: fmt.Sprintf("%d", commentID)},
				{Field: "body", After: input.Reason},
			},
		}))
		if err != nil {
			return err
		}

		// Step 2: Close the domain.
		t = t.WithState(domain.StateClosed)
		if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
			return err
		}
		if err := uow.Claims().InvalidateClaim(ctx, input.ClaimID); err != nil {
			return err
		}

		// Record the state change in the history log.
		revision, _ = uow.History().CountHistory(ctx, t.ID())
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   t.ID(),
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventStateChanged,
			Changes: []history.FieldChange{
				{Field: "state", Before: domain.StateClaimed.String(), After: domain.StateClosed.String()},
			},
		}))
		return err
	})
}

func (s *serviceImpl) TransitionState(ctx context.Context, input driving.TransitionInput) error {
	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return fmt.Errorf("invalid issue ID: %w", err)
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != issueID {
			return fmt.Errorf("claim does not match issue %s", issueID)
		}

		t, err := uow.Issues().GetIssue(ctx, issueID, false)
		if err != nil {
			return err
		}

		switch input.Action {
		case driving.ActionRelease:
			return s.releaseIssue(ctx, uow, issueID, input.ClaimID, c.Author(), now)
		case driving.ActionClose:
			return s.closeIssue(ctx, uow, t, input.ClaimID, c.Author(), now)
		case driving.ActionDefer:
			return s.transitionIssue(ctx, uow, t, input.ClaimID, c.Author(), now, domain.StateDeferred)
		default:
			return fmt.Errorf("invalid transition action")
		}
	})
}

// DeferIssue atomically defers a claimed domain. When Until is non-empty, a
// "defer-until" label is set and recorded in history before the state
// transition — all within one transaction. This replaces the two-call pattern
// (UpdateIssue + TransitionState) that leaked an ordering invariant into the
// CLI adapter: the label must be set before the transition because the
// transition invalidates the claim.
func (s *serviceImpl) DeferIssue(ctx context.Context, input driving.DeferIssueInput) error {
	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return fmt.Errorf("invalid issue ID: %w", err)
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != issueID {
			return fmt.Errorf("claim does not match issue %s", issueID)
		}

		t, err := uow.Issues().GetIssue(ctx, issueID, false)
		if err != nil {
			return err
		}

		// Optionally set the defer-until label before transitioning, since
		// the transition will invalidate the claim.
		if input.Until != "" {
			lbl, lblErr := domain.NewLabel("defer-until", input.Until)
			if lblErr != nil {
				return fmt.Errorf("invalid --until value: %w", lblErr)
			}

			oldLabels := t.Labels()
			oldVal, existed := oldLabels.Get(lbl.Key())
			labels := oldLabels.Set(lbl)
			t = t.WithLabels(labels)

			if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
				return err
			}

			// Record label history.
			labelStr := lbl.Key() + ":" + lbl.Value()
			var lc history.FieldChange
			if !existed {
				lc = history.FieldChange{Field: "label", After: labelStr}
			} else if oldVal != lbl.Value() {
				lc = history.FieldChange{Field: "label", Before: lbl.Key() + ":" + oldVal, After: labelStr}
			}
			if lc.Field != "" {
				revision, _ := uow.History().CountHistory(ctx, t.ID())
				if _, histErr := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
					IssueID:   t.ID(),
					Revision:  revision,
					Author:    c.Author(),
					Timestamp: now,
					EventType: history.EventLabelAdded,
					Changes:   []history.FieldChange{lc},
				})); histErr != nil {
					return histErr
				}
			}

			newStaleAt := now.Add(c.StaleAt().Sub(c.ClaimedAt()))
			if err := uow.Claims().UpdateClaimStaleAt(ctx, input.ClaimID, newStaleAt); err != nil {
				return err
			}
		}

		return s.transitionIssue(ctx, uow, t, input.ClaimID, c.Author(), now, domain.StateDeferred)
	})
}

// ReopenIssue transitions a closed or deferred issue back to the open state.
// The operation is atomic: validate, claim, transition to open, release the
// claim, and record the appropriate semantic event — all in one transaction.
func (s *serviceImpl) ReopenIssue(ctx context.Context, input driving.ReopenInput) error {
	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return fmt.Errorf("invalid issue ID: %w", err)
	}

	author, err := parseAuthor(input.Author)
	if err != nil {
		return err
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		t, err := uow.Issues().GetIssue(ctx, issueID, true)
		if err != nil {
			return err
		}

		previousState := t.State()
		if previousState != domain.StateClosed && previousState != domain.StateDeferred {
			return fmt.Errorf("issue %s is %s: only closed or deferred issues can be reopened: %w",
				issueID, previousState, domain.ErrIllegalTransition)
		}

		// Create a transient claim for the reopen operation.
		c, claimErr := domain.NewClaim(domain.NewClaimParams{
			IssueID:       issueID,
			Author:        author,
			StaleDuration: 5 * time.Minute,
			Now:           now,
		})
		if claimErr != nil {
			return claimErr
		}
		if err := uow.Claims().CreateClaim(ctx, c); err != nil {
			return err
		}

		// Transition directly to open and invalidate the transient claim.
		openState := domain.ReleaseState()
		t = t.WithState(openState)
		if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
			return err
		}
		if err := uow.Claims().InvalidateClaim(ctx, c.Token()); err != nil {
			return err
		}

		// Record the semantic event — EventReopened for closed issues,
		// EventUndeferred for deferred issues.
		eventType := history.EventReopened
		if previousState == domain.StateDeferred {
			eventType = history.EventUndeferred
		}

		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: eventType,
			Changes: []history.FieldChange{
				{Field: "state", Before: previousState.String(), After: openState.String()},
			},
		}))
		return err
	})
}

func (s *serviceImpl) DeleteIssue(ctx context.Context, input driving.DeleteInput) error {
	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return fmt.Errorf("invalid issue ID: %w", err)
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.IssueID() != issueID {
			return fmt.Errorf("claim does not match issue %s", issueID)
		}

		t, err := uow.Issues().GetIssue(ctx, issueID, false)
		if err != nil {
			return err
		}

		if err := ValidateDeletion(t.IsDeleted()); err != nil {
			return err
		}

		// For epics, check descendants.
		if t.IsEpic() {
			descendants, err := uow.Issues().GetDescendants(ctx, issueID)
			if err != nil {
				return err
			}

			result := PlanEpicDeletion(issueID, descendants)
			if len(result.Conflicts) > 0 {
				return &domain.ClaimConflictError{
					IssueID:       issueID.String(),
					CurrentHolder: result.Conflicts[0].ClaimedBy,
					StaleAt:       time.Now(),
				}
			}

			// Delete all descendants.
			for _, id := range result.ToDelete {
				if id == issueID {
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

		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    c.Author(),
			Timestamp: now,
			EventType: history.EventDeleted,
		}))
		return err
	})
}

func (s *serviceImpl) ShowIssue(ctx context.Context, id string) (driving.ShowIssueOutput, error) {
	var output driving.ShowIssueOutput

	parsedID, err := domain.ParseID(id)
	if err != nil {
		return output, domain.NewValidationError("issue_id", fmt.Sprintf("invalid issue ID %q: %s", id, err))
	}

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		t, err := uow.Issues().GetIssue(ctx, parsedID, false)
		if err != nil {
			return err
		}

		// Flat primitive-typed fields — mirror the domain Issue's accessors
		// so adapters can read simple types without importing the domain.
		output.ID = t.ID().String()
		output.Role = t.Role()
		output.Title = t.Title()
		output.Description = t.Description()
		output.AcceptanceCriteria = t.AcceptanceCriteria()
		output.Priority = t.Priority()
		output.State = t.State()
		if !t.ParentID().IsZero() {
			output.ParentID = t.ParentID().String()
		}
		labels := make(map[string]string)
		for k, v := range t.Labels().All() {
			labels[k] = v
		}
		output.Labels = labels
		output.CreatedAt = t.CreatedAt()

		// Revision and author from history.
		histCount, _ := uow.History().CountHistory(ctx, parsedID)
		output.Revision = max(0, histCount-1)

		latest, err := uow.History().GetLatestHistory(ctx, parsedID)
		if err == nil {
			output.Author = latest.Author().String()
		}

		// Relationships — convert domain types to flat DTOs so driving
		// adapters do not depend on domain.Relationship.
		rels, err := uow.Relationships().ListRelationships(ctx, parsedID)
		if err == nil {
			output.Relationships = make([]driving.RelationshipDTO, 0, len(rels))
			for _, r := range rels {
				output.Relationships = append(output.Relationships, driving.RelationshipDTO{
					SourceID: r.SourceID().String(),
					TargetID: r.TargetID().String(),
					Type:     r.Type().String(),
				})
			}
		}

		// Readiness and secondary state.
		blockers, _ := uow.Relationships().GetBlockerStatuses(ctx, parsedID)
		ancestors, _ := uow.Issues().GetAncestorStatuses(ctx, parsedID)

		if t.IsTask() {
			output.IsReady = IsTaskReady(t.State(), blockers, ancestors)
			ssResult := TaskSecondaryState(t.State(), blockers, ancestors)
			output.SecondaryState = ssResult.ListState
			output.DetailStates = ssResult.DetailStates
		} else {
			hasChildren, _ := uow.Issues().HasChildren(ctx, parsedID)
			output.IsReady = IsEpicReady(t.State(), hasChildren, blockers, ancestors)
			allChildrenClosed := epicAllChildrenClosed(ctx, uow, parsedID)
			ssResult := EpicSecondaryState(t.State(), hasChildren, allChildrenClosed, blockers, ancestors)
			output.SecondaryState = ssResult.ListState
			output.DetailStates = ssResult.DetailStates
		}

		// Inherited blocking: find the first blocked ancestor and its blockers.
		for _, a := range ancestors {
			if !a.IsBlocked {
				continue
			}
			ib := &driving.InheritedBlocking{AncestorID: a.ID.String()}
			// Retrieve the ancestor's relationships to find its unresolved blockers.
			ancestorRels, relErr := uow.Relationships().ListRelationships(ctx, a.ID)
			if relErr == nil {
				for _, rel := range ancestorRels {
					if rel.Type() == domain.RelBlockedBy && rel.SourceID() == a.ID {
						// Check if the blocker is unresolved (not closed/deleted).
						blocker, getErr := uow.Issues().GetIssue(ctx, rel.TargetID(), false)
						if getErr != nil {
							continue
						}
						if blocker.State() != domain.StateClosed && !blocker.IsDeleted() {
							ib.BlockerIDs = append(ib.BlockerIDs, rel.TargetID().String())
						}
					}
				}
			}
			output.InheritedBlocking = ib
			break
		}

		// Comments — include all so adapters can decide how many to display.
		allComments, _, commentErr := uow.Comments().ListComments(ctx, parsedID, driven.CommentFilter{}, -1)
		if commentErr == nil {
			output.CommentCount = len(allComments)
			output.Comments = toCommentDTOs(allComments)
		}

		// Children — fetch full list items for display, retain count for
		// backward compatibility.
		children, _, childErr := uow.Issues().ListIssues(ctx,
			driven.IssueFilter{ParentIDs: []domain.ID{parsedID}}, driven.OrderByPriority, -1)
		if childErr == nil {
			output.ChildCount = len(children)
			if len(children) > 0 {
				enrichListItemSecondaryStates(ctx, uow, children)
				output.Children = toIssueListItemDTOs(children)
			}
		}

		// Synthetic parent/child relationships — augment stored relationships
		// so callers get a complete picture without extra queries.
		if !t.ParentID().IsZero() {
			output.Relationships = append(output.Relationships, driving.RelationshipDTO{
				SourceID: id,
				TargetID: t.ParentID().String(),
				Type:     domain.RelChildOf.String(),
			})
		}
		for _, child := range output.Children {
			output.Relationships = append(output.Relationships, driving.RelationshipDTO{
				SourceID: id,
				TargetID: child.ID,
				Type:     domain.RelParentOf.String(),
			})
		}

		// Parent title — resolve the parent issue's title if it has one.
		if !t.ParentID().IsZero() {
			parent, parentErr := uow.Issues().GetIssue(ctx, t.ParentID(), false)
			if parentErr == nil {
				output.ParentTitle = parent.Title()
			}
		}

		// Blocker details — enrich each blocked_by relationship target with
		// state and claim information for ordered, annotated display.
		// Only include relationships where this issue is the SOURCE (i.e.,
		// this issue is blocked by the target). When this issue is the
		// TARGET, the relationship means something else is blocked by us.
		for _, rel := range output.Relationships {
			if rel.Type != domain.RelBlockedBy.String() || rel.SourceID != id {
				continue
			}
			targetID, parseErr := domain.ParseID(rel.TargetID)
			if parseErr != nil {
				continue
			}
			detail := driving.BlockerDetail{ID: targetID.String()}
			blocker, getErr := uow.Issues().GetIssue(ctx, targetID, false)
			if getErr != nil {
				continue
			}
			detail.Title = blocker.Title()
			detail.State = blocker.State()
			blockerClaim, claimErr := uow.Claims().GetClaimByIssue(ctx, targetID)
			if claimErr == nil {
				detail.ClaimAuthor = blockerClaim.Author().String()
			}
			output.BlockerDetails = append(output.BlockerDetails, detail)
		}

		// Claim info.
		activeClaim, err := uow.Claims().GetClaimByIssue(ctx, parsedID)
		if err == nil {
			output.ClaimID = activeClaim.ID()
			output.ClaimAuthor = activeClaim.Author().String()
			output.ClaimStaleAt = activeClaim.StaleAt()

			// Derive claimed_at from the most recent EventClaimed history entry.
			allHistory, _, histErr := uow.History().ListHistory(ctx, parsedID, driven.HistoryFilter{}, -1)
			if histErr == nil {
				for i := len(allHistory) - 1; i >= 0; i-- {
					if allHistory[i].EventType() == history.EventClaimed {
						output.ClaimedAt = allHistory[i].Timestamp()
						break
					}
				}
			}
		}

		return nil
	})

	return output, err
}

func (s *serviceImpl) ListIssues(ctx context.Context, input driving.ListIssuesInput) (driving.ListIssuesOutput, error) {
	var output driving.ListIssuesOutput

	pf, err := toPortFilter(input.Filter)
	if err != nil {
		return output, err
	}
	po := toPortOrderBy(input.OrderBy)

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		items, hasMore, err := uow.Issues().ListIssues(ctx, pf, po, input.Limit)
		if err != nil {
			return err
		}
		enrichListItemSecondaryStates(ctx, uow, items)
		output.Items = toIssueListItemDTOs(items)
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchIssues(ctx context.Context, input driving.SearchIssuesInput) (driving.ListIssuesOutput, error) {
	var output driving.ListIssuesOutput

	pf, err := toPortFilter(input.Filter)
	if err != nil {
		return output, err
	}
	po := toPortOrderBy(input.OrderBy)

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		items, hasMore, err := uow.Issues().SearchIssues(ctx, input.Query, pf, po, input.Limit)
		if err != nil {
			return err
		}
		enrichListItemSecondaryStates(ctx, uow, items)
		output.Items = toIssueListItemDTOs(items)
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// GetIssueSummary returns aggregate issue counts by primary state and computed
// readiness/blocked status in a single read transaction.
func (s *serviceImpl) GetIssueSummary(ctx context.Context) (driving.IssueSummaryOutput, error) {
	var output driving.IssueSummaryOutput

	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		summary, err := uow.Issues().GetIssueSummary(ctx)
		if err != nil {
			return err
		}
		output = driving.IssueSummaryOutput{
			Open:     summary.Open,
			Claimed:  summary.Claimed,
			Deferred: summary.Deferred,
			Closed:   summary.Closed,
			Ready:    summary.Ready,
			Blocked:  summary.Blocked,
			Total:    summary.Total(),
		}
		return nil
	})

	return output, err
}

// --- Label Operations ---

func (s *serviceImpl) ListDistinctLabels(ctx context.Context) ([]driving.LabelOutput, error) {
	var out []driving.LabelOutput
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		domainLabels, queryErr := uow.Issues().ListDistinctLabels(ctx)
		if queryErr != nil {
			return queryErr
		}
		out = make([]driving.LabelOutput, len(domainLabels))
		for i, l := range domainLabels {
			out[i] = driving.LabelOutput{Key: l.Key(), Value: l.Value()}
		}
		return nil
	})
	return out, err
}

// --- Label Propagation ---

// propagateTarget holds a descendant that needs a label update along with its
// claim context, determined during the validation phase.
type propagateTarget struct {
	issueID domain.ID
	// hasClaim is true when the descendant is already claimed by the
	// propagation author — the update uses the existing claim without
	// claiming or releasing.
	hasClaim bool
}

// PropagateLabel copies a label from a parent issue to all its descendants
// that lack it or have a different value. The operation is atomic with respect
// to claim conflicts: if any descendant is claimed by a different author, no
// labels are changed and a ClaimConflictError is returned.
func (s *serviceImpl) PropagateLabel(ctx context.Context, input driving.PropagateLabelInput) (driving.PropagateLabelOutput, error) {
	var output driving.PropagateLabelOutput

	author, err := parseAuthor(input.Author)
	if err != nil {
		return output, err
	}

	// Look up the parent's label value.
	parent, err := s.ShowIssue(ctx, input.IssueID)
	if err != nil {
		return output, fmt.Errorf("looking up parent issue: %w", err)
	}
	value, exists := parent.Labels[input.Key]
	if !exists {
		return output, fmt.Errorf("issue %s does not have label %q", input.IssueID, input.Key)
	}
	output.Value = value

	// List all descendants.
	descendants, err := s.ListIssues(ctx, driving.ListIssuesInput{
		Filter:  driving.IssueFilterInput{DescendantsOf: input.IssueID},
		OrderBy: driving.OrderByPriority,
		Limit:   -1,
	})
	if err != nil {
		return output, fmt.Errorf("listing descendants: %w", err)
	}
	output.Total = len(descendants.Items)

	// Phase 1: Validate — identify which descendants need updating and check
	// claim ownership. If any descendant is claimed by a different author,
	// bail out before mutating anything.
	var targets []propagateTarget
	for _, item := range descendants.Items {
		child, showErr := s.ShowIssue(ctx, item.ID)
		if showErr != nil {
			return output, fmt.Errorf("looking up descendant %s: %w", item.ID, showErr)
		}

		existingVal, hasLabel := child.Labels[input.Key]
		if hasLabel && existingVal == value {
			continue
		}

		parsedID, parseErr := domain.ParseID(item.ID)
		if parseErr != nil {
			return output, fmt.Errorf("parsing descendant ID %s: %w", item.ID, parseErr)
		}

		switch {
		case child.ClaimAuthor == "":
			// Unclaimed — will use OneShotUpdate.
			targets = append(targets, propagateTarget{issueID: parsedID})
		case child.ClaimAuthor == input.Author:
			// Claimed by the same author — will update via the existing claim
			// within a transaction.
			targets = append(targets, propagateTarget{issueID: parsedID, hasClaim: true})
		default:
			// Claimed by a different author — fail atomically.
			return output, fmt.Errorf(
				"descendant %s is claimed by %q: %w",
				item.ID, child.ClaimAuthor,
				&domain.ClaimConflictError{
					IssueID:       item.ID,
					CurrentHolder: child.ClaimAuthor,
					StaleAt:       child.ClaimStaleAt,
				},
			)
		}
	}

	// Phase 2: Mutate — apply the label to each target. All claim conflicts
	// were caught in phase 1, so errors here are unexpected and propagated.
	labelInput := driving.LabelInput{Key: input.Key, Value: value}
	for _, target := range targets {
		if target.hasClaim {
			// Descendant is claimed by the propagation author — look up the
			// claim by issue ID within a transaction and apply the label
			// update directly. This bypasses the bearer-token requirement
			// of UpdateIssue, since propagation is an internal operation
			// that has already verified author ownership.
			updateErr := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
				c, claimErr := uow.Claims().GetClaimByIssue(ctx, target.issueID)
				if claimErr != nil {
					return fmt.Errorf("looking up claim for %s: %w", target.issueID, claimErr)
				}
				fields := updateFields{LabelSet: []driving.LabelInput{labelInput}}
				propagateNow := time.Now()
				newStaleAt := propagateNow.Add(c.StaleAt().Sub(c.ClaimedAt()))
				return s.applyIssueUpdates(ctx, uow, target.issueID, c.ID(), author, propagateNow, newStaleAt, fields)
			})
			if updateErr != nil {
				return output, fmt.Errorf("updating claimed descendant %s: %w", target.issueID, updateErr)
			}
		} else {
			// Unclaimed — atomic claim→update→release.
			editErr := s.OneShotUpdate(ctx, driving.OneShotUpdateInput{
				IssueID:  target.issueID.String(),
				Author:   input.Author,
				LabelSet: []driving.LabelInput{labelInput},
			})
			if editErr != nil {
				return output, fmt.Errorf("one-shot update on descendant %s: %w", target.issueID, editErr)
			}
		}
		output.Propagated++
	}

	return output, nil
}

// --- Epic Operations ---

// EpicProgress returns completion data for open epics using GetChildStatuses
// for efficient child lookups.
func (s *serviceImpl) EpicProgress(ctx context.Context, input driving.EpicProgressInput) (driving.EpicProgressOutput, error) {
	var output driving.EpicProgressOutput
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		var epics []driven.IssueListItem

		if input.EpicID != "" {
			// Single epic mode.
			epicID, parseErr := domain.ParseID(input.EpicID)
			if parseErr != nil {
				return fmt.Errorf("parsing epic ID: %w", parseErr)
			}
			t, getErr := uow.Issues().GetIssue(ctx, epicID, false)
			if getErr != nil {
				return fmt.Errorf("looking up epic: %w", getErr)
			}
			if !t.IsEpic() {
				return fmt.Errorf("issue %s is not an epic", input.EpicID)
			}
			epics = []driven.IssueListItem{{
				ID:       t.ID(),
				Role:     t.Role(),
				State:    t.State(),
				Priority: t.Priority(),
				Title:    t.Title(),
			}}
		} else {
			// All open epics.
			pf, filterErr := toPortFilter(input.Filter)
			if filterErr != nil {
				return filterErr
			}
			pf.Roles = []domain.Role{domain.RoleEpic}
			pf.ExcludeClosed = true
			items, _, listErr := uow.Issues().ListIssues(ctx, pf, driven.OrderByPriority, -1)
			if listErr != nil {
				return fmt.Errorf("listing epics: %w", listErr)
			}
			epics = items
		}

		for _, epic := range epics {
			children, childErr := uow.Issues().GetChildStatuses(ctx, epic.ID)
			if childErr != nil {
				continue
			}

			progress := ComputeEpicProgress(children)

			// Compute secondary state for the epic.
			ss := computeListSecondaryState(ctx, uow, epic)

			output.Items = append(output.Items, driving.EpicProgressItem{
				ID:             epic.ID.String(),
				Title:          epic.Title,
				State:          epic.State,
				Priority:       epic.Priority,
				SecondaryState: ss,
				Total:          progress.Total,
				Closed:         progress.Closed,
				Claimed:        progress.Claimed,
				Open:           progress.Open,
				Blocked:        progress.Blocked,
				Deferred:       progress.Deferred,
				Percent:        progress.Percent,
				Completed:      progress.Completed,
			})
		}

		return nil
	})
	return output, err
}

// CloseCompletedEpics finds all open epics where all children are closed and
// batch-closes them. Each epic is claimed, a closing comment is added, and the
// epic is transitioned to closed. Per-epic failures are captured in the result.
func (s *serviceImpl) CloseCompletedEpics(ctx context.Context, input driving.CloseCompletedEpicsInput) (driving.CloseCompletedEpicsOutput, error) {
	var output driving.CloseCompletedEpicsOutput

	// Phase 1: find completed issues within a read transaction.
	// By default only epics are considered. When IncludeTasks is set,
	// parent tasks (tasks that have children, all closed) are included too.
	var completed []driven.IssueListItem
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		roles := []domain.Role{domain.RoleEpic}
		if input.IncludeTasks {
			roles = append(roles, domain.RoleTask)
		}

		items, _, listErr := uow.Issues().ListIssues(ctx, driven.IssueFilter{
			Roles:         roles,
			ExcludeClosed: true,
		}, driven.OrderByPriority, -1)
		if listErr != nil {
			return fmt.Errorf("listing issues: %w", listErr)
		}

		for _, item := range items {
			children, childErr := uow.Issues().GetChildStatuses(ctx, item.ID)
			if childErr != nil {
				continue
			}
			// Skip childless tasks — only parent tasks with all children
			// closed should be auto-closed.
			if len(children) == 0 {
				continue
			}
			progress := ComputeEpicProgress(children)
			if progress.Completed {
				completed = append(completed, item)
			}
		}
		return nil
	})
	if err != nil {
		return output, err
	}

	if len(completed) == 0 || input.DryRun {
		for _, epic := range completed {
			output.Results = append(output.Results, driving.CloseCompletedEpicResult{
				ID:      epic.ID.String(),
				Title:   epic.Title,
				Closed:  false,
				Message: "dry run",
			})
		}
		return output, nil
	}

	// Phase 2: close each completed epic in its own transaction.
	for _, epic := range completed {
		result := s.closeCompletedEpic(ctx, epic, input.Author)
		output.Results = append(output.Results, result)
		if result.Closed {
			output.ClosedCount++
		}
	}

	return output, nil
}

// closeCompletedEpic claims an epic, adds a closing comment, and transitions it
// to closed. Returns a result indicating success or failure.
func (s *serviceImpl) closeCompletedEpic(ctx context.Context, epic driven.IssueListItem, author string) driving.CloseCompletedEpicResult {
	// Claim the epic.
	claimOut, err := s.ClaimByID(ctx, driving.ClaimInput{
		IssueID: epic.ID.String(),
		Author:  author,
	})
	if err != nil {
		return driving.CloseCompletedEpicResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("claim failed: %v", err),
		}
	}

	// Add a closing comment.
	_, err = s.AddComment(ctx, driving.AddCommentInput{
		IssueID: epic.ID.String(),
		Author:  author,
		Body:    "All children are closed. Closing epic via batch close-completed.",
	})
	if err != nil {
		// Release the claim on comment failure.
		_ = s.TransitionState(ctx, driving.TransitionInput{
			IssueID: epic.ID.String(),
			ClaimID: claimOut.ClaimID,
			Action:  driving.ActionRelease,
		})
		return driving.CloseCompletedEpicResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("comment failed: %v", err),
		}
	}

	// Close the epic.
	err = s.TransitionState(ctx, driving.TransitionInput{
		IssueID: epic.ID.String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionClose,
	})
	if err != nil {
		return driving.CloseCompletedEpicResult{
			ID:      epic.ID.String(),
			Title:   epic.Title,
			Closed:  false,
			Message: fmt.Sprintf("close failed: %v", err),
		}
	}

	return driving.CloseCompletedEpicResult{
		ID:     epic.ID.String(),
		Title:  epic.Title,
		Closed: true,
	}
}

// --- Relationship Operations ---

func (s *serviceImpl) AddRelationship(ctx context.Context, sourceIDStr string, ri driving.RelationshipInput, authorStr string) error {
	author, err := parseAuthor(authorStr)
	if err != nil {
		return err
	}
	sourceID, err := domain.ParseID(sourceIDStr)
	if err != nil {
		return err
	}
	targetID, err := domain.ParseID(ri.TargetID)
	if err != nil {
		return err
	}
	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Validate target exists and is not deleted.
		if _, err := uow.Issues().GetIssue(ctx, targetID, false); err != nil {
			return fmt.Errorf("relationship target %s: %w", ri.TargetID, err)
		}

		rel, err := domain.NewRelationship(sourceID, targetID, ri.Type)
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

func (s *serviceImpl) RemoveRelationship(ctx context.Context, sourceIDStr string, ri driving.RelationshipInput, authorStr string) error {
	author, err := parseAuthor(authorStr)
	if err != nil {
		return err
	}
	sourceID, err := domain.ParseID(sourceIDStr)
	if err != nil {
		return err
	}
	targetID, err := domain.ParseID(ri.TargetID)
	if err != nil {
		return err
	}
	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		deleted, err := uow.Relationships().DeleteRelationship(ctx, sourceID, targetID, ri.Type)
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

// RemoveBidirectionalBlock removes any blocked_by relationship between issueA
// and issueB regardless of which direction was stored. Both removals are
// idempotent — the operation succeeds even if neither relationship exists.
func (s *serviceImpl) RemoveBidirectionalBlock(ctx context.Context, issueA, issueB string, author string) error {
	err := s.RemoveRelationship(ctx, issueA, driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: issueB,
	}, author)
	if err != nil {
		return fmt.Errorf("removing %s blocked_by %s: %w", issueA, issueB, err)
	}

	err = s.RemoveRelationship(ctx, issueB, driving.RelationshipInput{
		Type:     domain.RelBlockedBy,
		TargetID: issueA,
	}, author)
	if err != nil {
		return fmt.Errorf("removing %s blocked_by %s: %w", issueB, issueA, err)
	}

	return nil
}

// --- Comment Operations ---

func (s *serviceImpl) AddComment(ctx context.Context, input driving.AddCommentInput) (driving.AddCommentOutput, error) {
	author, err := parseAuthor(input.Author)
	if err != nil {
		return driving.AddCommentOutput{}, err
	}

	parsedIssueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return driving.AddCommentOutput{}, fmt.Errorf("invalid issue ID: %w", err)
	}

	var output driving.AddCommentOutput

	err = s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Verify issue exists and is not deleted.
		t, err := uow.Issues().GetIssue(ctx, parsedIssueID, true)
		if err != nil {
			return err
		}
		if t.IsDeleted() {
			return fmt.Errorf("cannot add comment to deleted issue: %w", domain.ErrDeletedIssue)
		}

		n, err := domain.NewComment(domain.NewCommentParams{
			IssueID:   parsedIssueID,
			Author:    author,
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

		// Record the comment in the history log.
		revision, _ := uow.History().CountHistory(ctx, parsedIssueID)
		_, histErr := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   parsedIssueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventCommentAdded,
			Changes: []history.FieldChange{
				{Field: "comment_id", After: fmt.Sprintf("%d", id)},
				{Field: "body", After: input.Body},
			},
		}))
		if histErr != nil {
			return histErr
		}

		// Extend claim staleAt if the issue is currently claimed.
		activeClaim, err := uow.Claims().GetClaimByIssue(ctx, parsedIssueID)
		if err == nil {
			newStaleAt := now.Add(activeClaim.StaleAt().Sub(activeClaim.ClaimedAt()))
			_ = uow.Claims().UpdateClaimStaleAt(ctx, activeClaim.ID(), newStaleAt)
		}

		c, err := uow.Comments().GetComment(ctx, id)
		if err != nil {
			return err
		}
		output.Comment = toCommentDTO(c)
		return nil
	})

	return output, err
}

func (s *serviceImpl) ShowComment(ctx context.Context, commentID int64) (driving.CommentDTO, error) {
	var result driving.CommentDTO

	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		c, err := uow.Comments().GetComment(ctx, commentID)
		if err != nil {
			return err
		}
		result = toCommentDTO(c)
		return nil
	})

	return result, err
}

func (s *serviceImpl) ListComments(ctx context.Context, input driving.ListCommentsInput) (driving.ListCommentsOutput, error) {
	parsedIssueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return driving.ListCommentsOutput{}, fmt.Errorf("invalid issue ID: %w", err)
	}

	portFilter, err := toPortCommentFilter(input.Filter)
	if err != nil {
		return driving.ListCommentsOutput{}, fmt.Errorf("invalid comment filter: %w", err)
	}

	var output driving.ListCommentsOutput

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		comments, hasMore, err := uow.Comments().ListComments(ctx, parsedIssueID, portFilter, input.Limit)
		if err != nil {
			return err
		}
		output.Comments = toCommentDTOs(comments)
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchComments(ctx context.Context, input driving.SearchCommentsInput) (driving.ListCommentsOutput, error) {
	portFilter, err := toPortCommentFilter(input.Filter)
	if err != nil {
		return driving.ListCommentsOutput{}, fmt.Errorf("invalid comment filter: %w", err)
	}

	// Scope to a specific issue when provided.
	if input.IssueID != "" {
		parsedIssueID, err := domain.ParseID(input.IssueID)
		if err != nil {
			return driving.ListCommentsOutput{}, fmt.Errorf("invalid issue ID: %w", err)
		}
		portFilter.IssueID = parsedIssueID
	}

	var output driving.ListCommentsOutput

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		comments, hasMore, err := uow.Comments().SearchComments(ctx, input.Query, portFilter, input.Limit)
		if err != nil {
			return err
		}
		output.Comments = toCommentDTOs(comments)
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// --- History Operations ---

func (s *serviceImpl) ShowHistory(ctx context.Context, input driving.ListHistoryInput) (driving.ListHistoryOutput, error) {
	issueID, err := domain.ParseID(input.IssueID)
	if err != nil {
		return driving.ListHistoryOutput{}, fmt.Errorf("invalid issue ID: %w", err)
	}

	var output driving.ListHistoryOutput

	pf := toPortHistoryFilter(input.Filter)

	err = s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		entries, hasMore, err := uow.History().ListHistory(ctx, issueID, pf, input.Limit)
		if err != nil {
			return err
		}
		output.Entries = toHistoryEntryDTOs(entries)
		output.HasMore = hasMore
		return nil
	})

	return output, err
}

// --- Graph ---

func (s *serviceImpl) GetGraphData(ctx context.Context) (driving.GraphDataOutput, error) {
	var output driving.GraphDataOutput
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		// Fetch all non-deleted issues (unlimited).
		items, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
		if err != nil {
			return err
		}
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
					output.Relationships = append(output.Relationships, driving.RelationshipDTO{
						SourceID: rel.SourceID().String(),
						TargetID: rel.TargetID().String(),
						Type:     rel.Type().String(),
					})
				}
			}
		}

		// Convert to DTOs after the relationship loop, which needs the
		// typed domain.ID from the raw driven.IssueListItem.
		output.Nodes = toIssueListItemDTOs(items)

		return nil
	})
	return output, err
}

// --- Diagnostics ---

func (s *serviceImpl) Doctor(ctx context.Context, input driving.DoctorInput) (driving.DoctorOutput, error) {
	var output driving.DoctorOutput

	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		now := time.Now()

		// Check stale claims (stealable).
		staleClaims, err := uow.Claims().ListStaleClaims(ctx, now)
		if err != nil {
			return err
		}
		if len(staleClaims) > 0 {
			for _, c := range staleClaims {
				output.Findings = append(output.Findings, driving.DoctorFinding{
					Category: "stale_claim",
					Severity: "warning",
					Message:  fmt.Sprintf("%s: claimed by %q since %s (stealable)", c.IssueID(), c.Author(), c.ClaimedAt().Format(time.RFC3339)),
					IssueIDs: []string{c.IssueID().String()},
					Action:   &driving.ActionHint{Kind: driving.ActionKindStealClaim, IssueID: c.IssueID().String()},
				})
			}
		}

		// Check long-held claims: active claims where idle time exceeds
		// 2× the default stale threshold but the claim is not yet stealable
		// (e.g. due to a custom longer threshold).
		activeClaims, err := uow.Claims().ListActiveClaims(ctx, now)
		if err != nil {
			return err
		}
		longThreshold := 2 * domain.DefaultStaleThreshold
		for _, c := range activeClaims {
			heldDuration := now.Sub(c.ClaimedAt())
			if heldDuration > longThreshold {
				claimDuration := c.StaleAt().Sub(c.ClaimedAt())
				output.Findings = append(output.Findings, driving.DoctorFinding{
					Category: "long_claim",
					Severity: "info",
					Message: fmt.Sprintf("%s has been claimed for %s (stale threshold: %s, not yet stealable)",
						c.IssueID(), heldDuration.Truncate(time.Minute), claimDuration),
					IssueIDs: []string{c.IssueID().String()},
				})
			}
		}

		// Check blocker graph health.
		blockerFindings, blockerErr := s.checkBlockerHealth(ctx, uow)
		if blockerErr != nil {
			return blockerErr
		}
		output.Findings = append(output.Findings, blockerFindings...)

		// Check for completed epics (standalone, not only when blocking).
		closeCompletedFindings, ceErr := s.checkCloseCompleted(ctx, uow)
		if ceErr != nil {
			return ceErr
		}
		output.Findings = append(output.Findings, closeCompletedFindings...)

		// Check for deleted and closed parent references.
		parentFindings, pErr := s.checkParentHealth(ctx, uow)
		if pErr != nil {
			return pErr
		}
		output.Findings = append(output.Findings, parentFindings...)

		// Check for priority inversions.
		inversionFindings, invErr := s.checkPriorityInversion(ctx, uow)
		if invErr != nil {
			return invErr
		}
		output.Findings = append(output.Findings, inversionFindings...)

		// Check for orphan tasks.
		orphanFindings, oErr := s.checkOrphanTasks(ctx, uow)
		if oErr != nil {
			return oErr
		}
		output.Findings = append(output.Findings, orphanFindings...)

		// Check storage integrity.
		integrityErr := uow.Database().IntegrityCheck(ctx)
		if integrityErr != nil {
			output.Findings = append(output.Findings, driving.DoctorFinding{
				Category: "storage_integrity",
				Severity: "error",
				Message:  fmt.Sprintf("Data corruption detected: %v", integrityErr),
				Action:   &driving.ActionHint{Kind: driving.ActionKindInvestigateCorruption},
			})
		}

		// Check GC recommendation.
		total, deleted, countErr := uow.Database().CountDeletedRatio(ctx)
		if countErr == nil && total > 0 {
			ratio := float64(deleted) / float64(total)
			if ratio > gcThresholdRatio {
				pct := int(ratio * 100)
				output.Findings = append(output.Findings, driving.DoctorFinding{
					Category: "gc_recommended",
					Severity: "info",
					Message:  fmt.Sprintf("%d of %d issues are deleted (%d%%)", deleted, total, pct),
					Action:   &driving.ActionHint{Kind: driving.ActionKindRunGC},
				})
			}
		}

		// Check for missing kind labels.
		labelFindings, lErr := s.checkMissingLabels(ctx, uow)
		if lErr != nil {
			return lErr
		}
		output.Findings = append(output.Findings, labelFindings...)

		// Check deferral health.
		deferralFindings, dErr := s.checkDeferrals(ctx, uow, now)
		if dErr != nil {
			return dErr
		}
		output.Findings = append(output.Findings, deferralFindings...)

		// Check for human-blocked issues.
		humanFindings, hErr := s.checkBlockedByHuman(ctx, uow)
		if hErr != nil {
			return hErr
		}
		output.Findings = append(output.Findings, humanFindings...)

		// Check for authors with multiple active claims.
		multiClaimFindings := s.checkMultiClaimAuthors(activeClaims)
		output.Findings = append(output.Findings, multiClaimFindings...)

		// Check for virtual labels stored in the labels table.
		virtualLabelCount, vlErr := uow.Database().CountVirtualLabelsInTable(ctx)
		if vlErr != nil {
			return vlErr
		}
		if virtualLabelCount > 0 {
			output.Findings = append(output.Findings, driving.DoctorFinding{
				Category: "virtual_label_in_table",
				Severity: "error",
				Message:  fmt.Sprintf("%d idempotency-key label(s) found in the labels table — they should be stored in the issues.idempotency_key column", virtualLabelCount),
				Action:   &driving.ActionHint{Kind: driving.ActionKindExecSQL, SQL: "DELETE FROM labels WHERE key = 'idempotency-key'"},
			})
		}

		return nil
	})
	if err != nil {
		return output, err
	}

	// Merge adapter-supplied findings (e.g., filesystem checks) with
	// service-generated findings before classification.
	allFindings := output.Findings
	allFindings = append(allFindings, input.AdditionalFindings...)

	// Classify all findings against the check registry.
	checks, filtered, healthy := classifyFindings(allFindings, input.MinSeverity)
	output.Findings = filtered
	output.Checks = checks
	output.Healthy = healthy

	return output, nil
}

// checkBlockerHealth walks the transitive blocked_by graph for all blocked
// issues and reports structural problems. Unlike the old readiness check, this
// runs unconditionally — it does not gate on "zero ready issues". It subsumes
// the old no_ready_issues, close_completed_blocker, and deferred_blocker
// categories under a unified blocker_health check.
func (s *serviceImpl) checkBlockerHealth(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	// Load all non-deleted, non-closed issues.
	allItems, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	if len(allItems) == 0 {
		return nil, nil
	}

	// Build issue state index from list items.
	type issueInfo struct {
		id        domain.ID
		state     domain.State
		isBlocked bool
		isEpic    bool
	}
	issueIndex := make(map[string]issueInfo, len(allItems))
	for _, item := range allItems {
		issueIndex[item.ID.String()] = issueInfo{
			id:        item.ID,
			state:     item.State,
			isBlocked: item.IsBlocked,
			isEpic:    item.Role == domain.RoleEpic,
		}
	}

	// Build blocked_by adjacency list: for each issue, its direct blockers.
	blockedByGraph := make(map[string][]string)
	for _, item := range allItems {
		if !item.IsBlocked {
			continue
		}
		rels, relErr := uow.Relationships().ListRelationships(ctx, item.ID)
		if relErr != nil {
			return nil, fmt.Errorf("listing relationships for %s: %w", item.ID, relErr)
		}
		for _, rel := range rels {
			if rel.Type() == domain.RelBlockedBy && rel.SourceID() == item.ID {
				blockedByGraph[item.ID.String()] = append(
					blockedByGraph[item.ID.String()], rel.TargetID().String(),
				)
			}
		}
	}

	var findings []driving.DoctorFinding

	// Deduplicate findings across chains.
	reportedCycles := make(map[string]bool)
	reportedDeferred := make(map[string]bool)
	reportedStale := make(map[string]bool)
	reportedDeleted := make(map[string]bool)
	reportedDeadEnd := make(map[string]bool)

	// Walk each blocked issue's transitive chain.
	for issueIDStr, blockers := range blockedByGraph {
		if len(blockers) == 0 {
			continue
		}

		// DFS to walk transitive blocked_by edges.
		visited := make(map[string]bool)
		inStack := make(map[string]bool)

		var walk func(node string) (reachesResolution bool)
		walk = func(node string) bool {
			if inStack[node] {
				// Cycle detected.
				if !reportedCycles[node] {
					reportedCycles[node] = true
					findings = append(findings, driving.DoctorFinding{
						Category: "blocker_cycle",
						Severity: "error",
						Message:  fmt.Sprintf("Cycle detected in blocked_by chain involving %s", node),
						IssueIDs: []string{node},
					})
				}
				return false
			}
			if visited[node] {
				return false
			}
			visited[node] = true
			inStack[node] = true
			defer func() { inStack[node] = false }()

			info, exists := issueIndex[node]
			if !exists {
				// Issue is deleted or closed — stale relationship.
				if !reportedDeleted[node] {
					reportedDeleted[node] = true
					findings = append(findings, driving.DoctorFinding{
						Category: "blocker_deleted",
						Severity: "error",
						Message:  fmt.Sprintf("Issue %s is blocked by %s which is closed or deleted (stale relationship)", issueIDStr, node),
						IssueIDs: []string{issueIDStr, node},
						Action:   &driving.ActionHint{Kind: driving.ActionKindUnblockRelationship, SourceID: issueIDStr, TargetID: node},
					})
				}
				return false
			}

			// Terminal conditions: issue is claimed or open+unblocked.
			if info.state == domain.StateClaimed {
				return true
			}

			// Dead end: deferred blocker.
			if info.state == domain.StateDeferred {
				if !reportedDeferred[node] {
					reportedDeferred[node] = true
					findings = append(findings, driving.DoctorFinding{
						Category: "blocker_deferred",
						Severity: "error",
						Message:  fmt.Sprintf("Issue %s is deferred and blocking other issues", node),
						IssueIDs: []string{node},
						Action:   &driving.ActionHint{Kind: driving.ActionKindUndefer, IssueID: node},
					})
				}
				return false
			}

			// Check completed epics before treating open+unblocked as resolved.
			if info.isEpic && info.state == domain.StateOpen && !info.isBlocked {
				children, childErr := uow.Issues().GetChildStatuses(ctx, info.id)
				if childErr == nil && isCloseCompleted(children) {
					if !reportedStale[node] {
						reportedStale[node] = true
						findings = append(findings, driving.DoctorFinding{
							Category: "blocker_close_completed",
							Severity: "error",
							Message:  fmt.Sprintf("Epic %s has all children closed but is still open, blocking other issues", node),
							IssueIDs: []string{node},
							Action:   &driving.ActionHint{Kind: driving.ActionKindCloseCompleted},
						})
					}
					return false
				}
			}

			// Open and unblocked (and not a completed epic) — resolution point.
			if info.state == domain.StateOpen && !info.isBlocked {
				return true
			}

			// Recurse into this node's blockers if it's also blocked.
			nodeBlockers := blockedByGraph[node]
			if len(nodeBlockers) == 0 {
				return true // Not blocked — this is a resolution point.
			}

			anyResolves := false
			for _, blocker := range nodeBlockers {
				if walk(blocker) {
					anyResolves = true
				}
			}

			if !anyResolves && !reportedDeadEnd[node] {
				reportedDeadEnd[node] = true
				findings = append(findings, driving.DoctorFinding{
					Category: "blocker_dead_end",
					Severity: "error",
					Message:  fmt.Sprintf("All transitive blockers for %s are unresolvable (nothing is claimed or ready)", node),
					IssueIDs: []string{node},
				})
			}

			return anyResolves
		}

		for _, blocker := range blockers {
			walk(blocker)
		}
	}

	return findings, nil
}

// isCloseCompleted reports whether an epic's children are all closed, making
// the epic completed. An epic with no children is not completed.
func isCloseCompleted(children []domain.ChildStatus) bool {
	if len(children) == 0 {
		return false
	}
	for _, c := range children {
		if c.State != domain.StateClosed {
			return false
		}
	}
	return true
}

// checkCloseCompleted detects open epics whose children are all closed, making
// them completed. This is a standalone check that runs regardless of blocking
// relationships — unlike blocker_close_completed which only fires when the
// epic is in a blocking chain.
func (s *serviceImpl) checkCloseCompleted(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	epics, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		Roles:         []domain.Role{domain.RoleEpic},
		ExcludeClosed: true,
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing epics: %w", err)
	}

	var findings []driving.DoctorFinding
	for _, epic := range epics {
		children, childErr := uow.Issues().GetChildStatuses(ctx, epic.ID)
		if childErr != nil {
			continue
		}
		if isCloseCompleted(children) {
			findings = append(findings, driving.DoctorFinding{
				Category: "close_completed",
				Severity: "warning",
				Message:  fmt.Sprintf("Epic %s has all children closed and can be closed", epic.ID),
				IssueIDs: []string{epic.ID.String()},
				Action:   &driving.ActionHint{Kind: driving.ActionKindCloseCompleted},
			})
		}
	}

	return findings, nil
}

// checkParentHealth detects issues with broken parent references — either
// referencing a deleted parent (data integrity issue) or open issues whose
// parent is closed (orphaned children).
func (s *serviceImpl) checkParentHealth(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	// Load all non-deleted issues (including closed for parent lookup).
	allItems, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	var findings []driving.DoctorFinding
	for _, item := range allItems {
		if item.ParentID.IsZero() {
			continue
		}

		parent, parentErr := uow.Issues().GetIssue(ctx, item.ParentID, true)
		if parentErr != nil {
			// Parent not found at all — this is an integrity error.
			findings = append(findings, driving.DoctorFinding{
				Category: "deleted_parent",
				Severity: "error",
				Message:  fmt.Sprintf("%s references deleted parent %s", item.ID, item.ParentID),
				IssueIDs: []string{item.ID.String(), item.ParentID.String()},
			})
			continue
		}

		if parent.IsDeleted() {
			findings = append(findings, driving.DoctorFinding{
				Category: "deleted_parent",
				Severity: "error",
				Message:  fmt.Sprintf("%s references deleted parent %s", item.ID, item.ParentID),
				IssueIDs: []string{item.ID.String(), item.ParentID.String()},
			})
			continue
		}

		// Only report closed parent for open (non-closed) issues.
		if item.State != domain.StateClosed && parent.State() == domain.StateClosed {
			findings = append(findings, driving.DoctorFinding{
				Category: "closed_parent",
				Severity: "warning",
				Message:  fmt.Sprintf("Open %s %s has closed parent epic %s", item.Role, item.ID, item.ParentID),
				IssueIDs: []string{item.ID.String(), item.ParentID.String()},
			})
		}
	}

	return findings, nil
}

// checkPriorityInversion detects two types of priority inversions:
// (a) A low-priority blocker gating higher-priority work via blocked_by.
// (b) A parent epic with lower priority than one of its children.
func (s *serviceImpl) checkPriorityInversion(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	// Load all non-deleted, non-closed issues.
	allItems, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	// Index issues by ID for quick lookup.
	itemIndex := make(map[string]driven.IssueListItem, len(allItems))
	for _, item := range allItems {
		itemIndex[item.ID.String()] = item
	}

	var findings []driving.DoctorFinding

	for _, item := range allItems {
		// (a) Check blocked_by inversions.
		if item.IsBlocked {
			rels, relErr := uow.Relationships().ListRelationships(ctx, item.ID)
			if relErr != nil {
				continue
			}
			for _, rel := range rels {
				if rel.Type() != domain.RelBlockedBy || rel.SourceID() != item.ID {
					continue
				}
				blocker, ok := itemIndex[rel.TargetID().String()]
				if !ok {
					continue // blocker is closed or deleted
				}
				// Higher numeric priority value = lower urgency.
				if blocker.Priority > item.Priority {
					findings = append(findings, driving.DoctorFinding{
						Category: "priority_inversion",
						Severity: "warning",
						Message: fmt.Sprintf("%s %s %s blocks higher-priority %s %s %s",
							blocker.Priority, blocker.Role, blocker.ID,
							item.Priority, item.Role, item.ID),
						IssueIDs: []string{blocker.ID.String(), item.ID.String()},
					})
				}
			}
		}

		// (b) Check parent-child inversions.
		if !item.ParentID.IsZero() {
			parent, ok := itemIndex[item.ParentID.String()]
			if !ok {
				continue // parent is closed or deleted
			}
			if parent.Priority > item.Priority {
				findings = append(findings, driving.DoctorFinding{
					Category: "priority_inversion",
					Severity: "warning",
					Message: fmt.Sprintf("%s %s %s has higher-priority %s %s %s as a child",
						parent.Priority, parent.Role, parent.ID,
						item.Priority, item.Role, item.ID),
					IssueIDs: []string{parent.ID.String(), item.ID.String()},
				})
			}
		}
	}

	return findings, nil
}

// checkOrphanTasks detects non-closed tasks without a parent epic, excluding
// issues with kind:bug or kind:fix labels. These tasks may be missing
// organizational context.
func (s *serviceImpl) checkOrphanTasks(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	orphans, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		Roles:         []domain.Role{domain.RoleTask},
		Orphan:        true,
		ExcludeClosed: true,
		LabelFilters: []driven.LabelFilter{
			{Key: "kind", Value: "bug", Negate: true},
			{Key: "kind", Value: "fix", Negate: true},
		},
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing orphan tasks: %w", err)
	}

	if len(orphans) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(orphans))
	for _, item := range orphans {
		ids = append(ids, item.ID.String())
	}

	return []driving.DoctorFinding{{
		Category: "orphan_task",
		Severity: "info",
		Message:  fmt.Sprintf("%d open tasks have no parent epic", len(orphans)),
		IssueIDs: ids,
	}}, nil
}

// checkMissingLabels detects open issues missing the "kind" label.
func (s *serviceImpl) checkMissingLabels(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	missing, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
		LabelFilters: []driven.LabelFilter{
			{Key: "kind", Negate: true},
		},
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing issues missing kind label: %w", err)
	}

	if len(missing) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(missing))
	for _, item := range missing {
		ids = append(ids, item.ID.String())
	}

	return []driving.DoctorFinding{{
		Category: "missing_label",
		Severity: "info",
		Message:  fmt.Sprintf("%d open issues are missing the kind label", len(missing)),
		IssueIDs: ids,
	}}, nil
}

// checkDeferrals detects overdue and long-standing deferrals.
// overdue_deferrals (warning): deferred issues past their defer-until date.
// long_deferrals (info): issues deferred for more than 1 week.
func (s *serviceImpl) checkDeferrals(ctx context.Context, uow driven.UnitOfWork, now time.Time) ([]driving.DoctorFinding, error) {
	deferred, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		States: []domain.State{domain.StateDeferred},
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing deferred issues: %w", err)
	}

	var findings []driving.DoctorFinding
	oneWeek := 7 * 24 * time.Hour

	for _, item := range deferred {
		// Check overdue_deferrals: look for defer-until label.
		shown, showErr := uow.Issues().GetIssue(ctx, item.ID, false)
		if showErr != nil {
			continue
		}

		deferUntil, hasDeferUntil := shown.Labels().Get("defer-until")
		if hasDeferUntil {
			parsedDate, parseErr := time.Parse("2006-01-02", deferUntil)
			if parseErr == nil && now.After(parsedDate) {
				overdueDays := int(now.Sub(parsedDate).Hours() / 24)
				findings = append(findings, driving.DoctorFinding{
					Category: "overdue_deferral",
					Severity: "warning",
					Message:  fmt.Sprintf("%s was deferred until %s (%d days overdue)", item.ID, deferUntil, overdueDays),
					IssueIDs: []string{item.ID.String()},
					Action:   &driving.ActionHint{Kind: driving.ActionKindUndefer, IssueID: item.ID.String()},
				})
			}
		}

		// Check long_deferrals: deferred for more than 1 week.
		// Uses CreatedAt as a proxy — the schema has no updated_at column.
		deferredDuration := now.Sub(item.CreatedAt)
		if deferredDuration > oneWeek {
			deferredDays := int(deferredDuration.Hours() / 24)
			findings = append(findings, driving.DoctorFinding{
				Category: "long_deferral",
				Severity: "info",
				Message:  fmt.Sprintf("%s has been deferred for %d days", item.ID, deferredDays),
				IssueIDs: []string{item.ID.String()},
			})
		}
	}

	return findings, nil
}

// checkBlockedByHuman detects issues carrying the waiting_on:human label.
func (s *serviceImpl) checkBlockedByHuman(ctx context.Context, uow driven.UnitOfWork) ([]driving.DoctorFinding, error) {
	waiting, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{
		ExcludeClosed: true,
		LabelFilters: []driven.LabelFilter{
			{Key: "waiting_on", Value: "human"},
		},
	}, driven.OrderByPriority, -1)
	if err != nil {
		return nil, fmt.Errorf("listing human-blocked issues: %w", err)
	}

	var findings []driving.DoctorFinding
	for _, item := range waiting {
		findings = append(findings, driving.DoctorFinding{
			Category: "blocked_by_human",
			Severity: "info",
			Message:  fmt.Sprintf("%s is waiting on human action (label: waiting_on:human)", item.ID),
			IssueIDs: []string{item.ID.String()},
		})
	}

	return findings, nil
}

// checkMultiClaimAuthors detects authors that have multiple active claims open
// simultaneously. Each finding lists the author and their claimed issues.
func (s *serviceImpl) checkMultiClaimAuthors(activeClaims []domain.Claim) []driving.DoctorFinding {
	// Group active claims by author.
	byAuthor := make(map[string][]string)
	for _, c := range activeClaims {
		authorStr := c.Author().String()
		byAuthor[authorStr] = append(byAuthor[authorStr], c.IssueID().String())
	}

	var findings []driving.DoctorFinding
	for authorStr, issueIDs := range byAuthor {
		if len(issueIDs) < 2 {
			continue
		}
		findings = append(findings, driving.DoctorFinding{
			Category: "multi_claim_author",
			Severity: "info",
			Message:  fmt.Sprintf("%q has %d active claims: %s", authorStr, len(issueIDs), strings.Join(issueIDs, ", ")),
			IssueIDs: issueIDs,
		})
	}

	return findings
}

func (s *serviceImpl) GC(ctx context.Context, input driving.GCInput) (driving.GCOutput, error) {
	var output driving.GCOutput

	err := s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		deleted, closed, gcErr := uow.Database().GC(ctx, input.IncludeClosed)
		if gcErr != nil {
			return gcErr
		}
		output.DeletedIssuesRemoved = deleted
		output.ClosedIssuesRemoved = closed
		return nil
	})

	return output, err
}

// --- Backup / Restore ---

func (s *serviceImpl) Backup(ctx context.Context, input driving.BackupInput) (driving.BackupOutput, error) {
	var output driving.BackupOutput

	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		prefix, err := uow.Database().GetPrefix(ctx)
		if err != nil {
			return fmt.Errorf("reading prefix: %w", err)
		}

		header := domain.BackupHeader{
			Prefix:    prefix,
			Timestamp: time.Now().UTC(),
			Version:   domain.BackupAlgorithmVersion,
		}
		if err := input.Writer.WriteHeader(header); err != nil {
			return fmt.Errorf("writing header: %w", err)
		}

		// List all non-deleted issues.
		items, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByCreatedAt, -1)
		if err != nil {
			return fmt.Errorf("listing issues: %w", err)
		}

		for _, item := range items {
			t, err := uow.Issues().GetIssue(ctx, item.ID, false)
			if err != nil {
				return fmt.Errorf("getting issue %s: %w", item.ID, err)
			}

			rec, err := s.buildIssueRecord(ctx, uow, t)
			if err != nil {
				return fmt.Errorf("building record for %s: %w", item.ID, err)
			}

			if err := input.Writer.WriteRecord(rec); err != nil {
				return fmt.Errorf("writing record %s: %w", item.ID, err)
			}
			output.IssueCount++
		}

		return nil
	})

	return output, err
}

// buildIssueRecord assembles a complete domain.BackupIssueRecord from a
// domain issue and all its associated data.
func (s *serviceImpl) buildIssueRecord(ctx context.Context, uow driven.UnitOfWork, t domain.Issue) (domain.BackupIssueRecord, error) {
	rec := domain.BackupIssueRecord{
		IssueID:            t.ID().String(),
		Role:               t.Role().String(),
		Title:              t.Title(),
		Description:        t.Description(),
		AcceptanceCriteria: t.AcceptanceCriteria(),
		Priority:           t.Priority().String(),
		State:              t.State().String(),
		CreatedAt:          t.CreatedAt(),
		IdempotencyKey:     t.IdempotencyKey(),
	}
	if !t.ParentID().IsZero() {
		rec.ParentID = t.ParentID().String()
	}

	// Labels.
	rec.Labels = make([]domain.BackupLabelRecord, 0)
	for k, v := range t.Labels().All() {
		rec.Labels = append(rec.Labels, domain.BackupLabelRecord{Key: k, Value: v})
	}

	// Comments.
	comments, _, err := uow.Comments().ListComments(ctx, t.ID(), driven.CommentFilter{}, -1)
	if err != nil {
		return domain.BackupIssueRecord{}, fmt.Errorf("listing comments: %w", err)
	}
	rec.Comments = make([]domain.BackupCommentRecord, 0, len(comments))
	for _, c := range comments {
		rec.Comments = append(rec.Comments, domain.BackupCommentRecord{
			CommentID: c.ID(),
			Author:    c.Author().String(),
			CreatedAt: c.CreatedAt(),
			Body:      c.Body(),
		})
	}

	// Relationships — capture only source-direction relationships to
	// avoid writing each relationship twice. For symmetric types (refs),
	// ListRelationships swaps source/target so SourceID always matches
	// the queried issue; we break the tie by only capturing the
	// relationship from the lexicographically smaller issue ID.
	rels, err := uow.Relationships().ListRelationships(ctx, t.ID())
	if err != nil {
		return domain.BackupIssueRecord{}, fmt.Errorf("listing relationships: %w", err)
	}
	rec.Relationships = make([]domain.BackupRelationshipRecord, 0)
	for _, rel := range rels {
		if rel.SourceID() != t.ID() {
			continue
		}
		if rel.Type().IsSymmetric() && t.ID().String() > rel.TargetID().String() {
			continue
		}
		rec.Relationships = append(rec.Relationships, domain.BackupRelationshipRecord{
			TargetID: rel.TargetID().String(),
			RelType:  rel.Type().String(),
		})
	}

	// Claims.
	activeClaim, err := uow.Claims().GetClaimByIssue(ctx, t.ID())
	rec.Claims = make([]domain.BackupClaimRecord, 0)
	if err == nil {
		claimDuration := activeClaim.StaleAt().Sub(activeClaim.ClaimedAt())
		rec.Claims = append(rec.Claims, domain.BackupClaimRecord{
			ClaimSHA512:    activeClaim.ID(),
			Author:         activeClaim.Author().String(),
			StaleThreshold: int64(claimDuration),
			LastActivity:   activeClaim.ClaimedAt(),
		})
	}

	// History.
	entries, _, err := uow.History().ListHistory(ctx, t.ID(), driven.HistoryFilter{}, -1)
	if err != nil {
		return domain.BackupIssueRecord{}, fmt.Errorf("listing history: %w", err)
	}
	rec.History = make([]domain.BackupHistoryRecord, 0, len(entries))
	for _, e := range entries {
		changes := make([]domain.BackupFieldChangeRecord, 0, len(e.Changes()))
		for _, fc := range e.Changes() {
			changes = append(changes, domain.BackupFieldChangeRecord{
				Field:  fc.Field,
				Before: fc.Before,
				After:  fc.After,
			})
		}
		rec.History = append(rec.History, domain.BackupHistoryRecord{
			EntryID:   e.ID(),
			Revision:  e.Revision(),
			Author:    e.Author().String(),
			Timestamp: e.Timestamp(),
			EventType: e.EventType().String(),
			Changes:   changes,
		})
	}

	return rec, nil
}

func (s *serviceImpl) Restore(ctx context.Context, input driving.RestoreInput) error {
	header, err := input.Reader.ReadHeader()
	if err != nil {
		return fmt.Errorf("reading backup header: %w", err)
	}

	switch header.Version {
	case 1:
		return s.restoreV1(ctx, header, input.Reader)
	default:
		return fmt.Errorf("unsupported backup version %d", header.Version)
	}
}

// restoreV1 implements restore for backup algorithm version 1. All
// work is performed in a single write transaction for atomicity.
func (s *serviceImpl) restoreV1(ctx context.Context, header domain.BackupHeader, reader driven.BackupReader) error {
	// Collect all records first so we can process them in the correct
	// order within a single transaction.
	var records []domain.BackupIssueRecord
	for {
		rec, ok, err := reader.NextRecord()
		if err != nil {
			return fmt.Errorf("reading backup record: %w", err)
		}
		if !ok {
			break
		}
		records = append(records, rec)
	}

	return s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		db := uow.Database()

		// Clear everything.
		if err := db.ClearAllData(ctx); err != nil {
			return fmt.Errorf("clearing database: %w", err)
		}

		// Re-initialize prefix.
		if err := db.InitDatabase(ctx, header.Prefix); err != nil {
			return fmt.Errorf("restoring prefix: %w", err)
		}

		// Insert issues in two passes: first all issues with parent_id
		// NULL, then update parent_id. This avoids foreign-key ordering
		// issues.
		for _, rec := range records {
			savedParent := rec.ParentID
			rec.ParentID = ""
			if err := db.RestoreIssueRaw(ctx, rec); err != nil {
				return fmt.Errorf("restoring issue %s: %w", rec.IssueID, err)
			}
			rec.ParentID = savedParent
		}

		// Second pass: set parent_id on issues that have one.
		for _, rec := range records {
			if rec.ParentID == "" {
				continue
			}
			t, err := uow.Issues().GetIssue(ctx, mustParseID(rec.IssueID), true)
			if err != nil {
				return fmt.Errorf("getting restored issue %s for parent update: %w", rec.IssueID, err)
			}
			t = t.WithParentID(mustParseID(rec.ParentID))
			if err := uow.Issues().UpdateIssue(ctx, t); err != nil {
				return fmt.Errorf("setting parent on %s: %w", rec.IssueID, err)
			}
		}

		// Insert associated data for each domain.
		for _, rec := range records {
			for _, label := range rec.Labels {
				if err := db.RestoreLabelRaw(ctx, rec.IssueID, label); err != nil {
					return fmt.Errorf("restoring label on %s: %w", rec.IssueID, err)
				}
			}
			for _, c := range rec.Comments {
				if err := db.RestoreCommentRaw(ctx, rec.IssueID, c); err != nil {
					return fmt.Errorf("restoring comment on %s: %w", rec.IssueID, err)
				}
			}
			for _, cl := range rec.Claims {
				if err := db.RestoreClaimRaw(ctx, rec.IssueID, cl); err != nil {
					return fmt.Errorf("restoring claim on %s: %w", rec.IssueID, err)
				}
			}
			for _, rel := range rec.Relationships {
				if err := db.RestoreRelationshipRaw(ctx, rec.IssueID, rel); err != nil {
					return fmt.Errorf("restoring relationship on %s: %w", rec.IssueID, err)
				}
			}
			for _, h := range rec.History {
				if err := db.RestoreHistoryRaw(ctx, rec.IssueID, h); err != nil {
					return fmt.Errorf("restoring history on %s: %w", rec.IssueID, err)
				}
			}
		}

		// Rebuild full-text search indexes.
		if err := db.RebuildFTS(ctx); err != nil {
			return fmt.Errorf("rebuilding FTS: %w", err)
		}

		return nil
	})
}

// mustParseID parses an issue ID string, panicking on failure. Used
// only during restore where IDs have already been validated by the
// original database.
func mustParseID(s string) domain.ID {
	id, err := domain.ParseID(s)
	if err != nil {
		panic(fmt.Sprintf("invalid issue ID in backup: %s", s))
	}
	return id
}

// --- Internal helpers ---

func (s *serviceImpl) validateParent(ctx context.Context, uow driven.UnitOfWork, childID, parentID domain.ID, childRole domain.Role) error {
	parent, err := uow.Issues().GetIssue(ctx, parentID, true)
	if err != nil {
		return fmt.Errorf("parent %s: %w", parentID, err)
	}
	if err := ValidateParent(childID, parentID, parent.IsDeleted()); err != nil {
		return err
	}
	ancestorLookup := func(id domain.ID) (domain.ID, error) {
		return uow.Issues().GetParentID(ctx, id)
	}
	if err := ValidateNoCycle(childID, parentID, ancestorLookup); err != nil {
		return err
	}
	if err := ValidateEpicDepth(childRole, parentID, ancestorLookup); err != nil {
		return err
	}
	return ValidateDepth(parentID, ancestorLookup)
}

func (s *serviceImpl) releaseIssue(ctx context.Context, uow driven.UnitOfWork, issueID domain.ID, claimID string, author domain.Author, now time.Time) error {
	t, err := uow.Issues().GetIssue(ctx, issueID, false)
	if err != nil {
		return err
	}

	releaseState := domain.ReleaseState()

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
			{Field: "state", Before: domain.StateClaimed.String(), After: releaseState.String()},
		},
	}))
	return err
}

func (s *serviceImpl) closeIssue(ctx context.Context, uow driven.UnitOfWork, t domain.Issue, claimID string, author domain.Author, now time.Time) error {
	if err := domain.Transition(t.State(), domain.StateClosed); err != nil {
		return err
	}

	// Ensure all children are closed before allowing close.
	children, err := uow.Issues().GetChildStatuses(ctx, t.ID())
	if err != nil {
		return fmt.Errorf("checking children: %w", err)
	}
	for _, child := range children {
		if child.State != domain.StateClosed {
			return fmt.Errorf("cannot close issue with unclosed children: %w", domain.ErrIllegalTransition)
		}
	}

	t = t.WithState(domain.StateClosed)
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
			{Field: "state", Before: domain.StateClaimed.String(), After: domain.StateClosed.String()},
		},
	}))
	return histErr
}

func (s *serviceImpl) transitionIssue(ctx context.Context, uow driven.UnitOfWork, t domain.Issue, claimID string, author domain.Author, now time.Time, targetState domain.State) error {
	if err := domain.Transition(t.State(), targetState); err != nil {
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
			{Field: "state", Before: domain.StateClaimed.String(), After: targetState.String()},
		},
	}))
	return err
}

// updateFields groups optional field updates for an domain.
type updateFields struct {
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *domain.Priority
	ParentID           *domain.ID
	LabelSet           []driving.LabelInput
	LabelRemove        []string
}

func oneShotToUpdateFields(input driving.OneShotUpdateInput) (updateFields, error) {
	f := updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		LabelSet:           input.LabelSet,
		LabelRemove:        input.LabelRemove,
	}
	if input.Priority != nil {
		f.Priority = input.Priority
	}
	parentID, err := parseOptionalID(input.ParentID)
	if err != nil {
		return updateFields{}, domain.NewValidationError("parent_id", err.Error())
	}
	f.ParentID = parentID
	return f, nil
}

func updateFieldsFromInput(input driving.UpdateIssueInput) (updateFields, error) {
	f := updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		LabelSet:           input.LabelSet,
		LabelRemove:        input.LabelRemove,
	}
	if input.Priority != nil {
		f.Priority = input.Priority
	}
	parentID, err := parseOptionalID(input.ParentID)
	if err != nil {
		return updateFields{}, domain.NewValidationError("parent_id", err.Error())
	}
	f.ParentID = parentID
	return f, nil
}

// parseOptionalID converts *string to *domain.ID. A nil input returns nil. An
// empty string returns a pointer to the zero domain.ID, which signals "clear
// parent" in the update path.
func parseOptionalID(s *string) (*domain.ID, error) {
	if s == nil {
		return nil, nil
	}
	if *s == "" {
		zeroID := domain.ID{}
		return &zeroID, nil
	}
	id, err := domain.ParseID(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *serviceImpl) applyIssueUpdates(ctx context.Context, uow driven.UnitOfWork, issueID domain.ID, claimID string, author domain.Author, now time.Time, staleAt time.Time, fields updateFields) error {
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
			if err := s.validateParent(ctx, uow, issueID, newParentID, t.Role()); err != nil {
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

	// Apply label changes, tracking what actually changed.
	oldLabels := t.Labels()
	labels := oldLabels
	var labelAdded []history.FieldChange
	var labelRemoved []history.FieldChange

	for _, f := range fields.LabelSet {
		domainLabel, labelErr := domain.NewLabel(f.Key, f.Value)
		if labelErr != nil {
			return fmt.Errorf("invalid label %q:%q: %w", f.Key, f.Value, labelErr)
		}
		oldVal, existed := oldLabels.Get(f.Key)
		labels = labels.Set(domainLabel)
		labelStr := f.Key + ":" + f.Value
		if !existed {
			labelAdded = append(labelAdded, history.FieldChange{Field: "label", After: labelStr})
		} else if oldVal != f.Value {
			labelAdded = append(labelAdded, history.FieldChange{Field: "label", Before: f.Key + ":" + oldVal, After: labelStr})
		}
	}
	for _, key := range fields.LabelRemove {
		oldVal, existed := oldLabels.Get(key)
		labels = labels.Remove(key)
		if existed {
			labelRemoved = append(labelRemoved, history.FieldChange{Field: "label", Before: key + ":" + oldVal})
		}
	}
	t = t.WithLabels(labels)

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

	// Record label additions as separate history entries.
	for _, lc := range labelAdded {
		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventLabelAdded,
			Changes:   []history.FieldChange{lc},
		}))
		if err != nil {
			return err
		}
	}

	// Record label removals as separate history entries.
	for _, lc := range labelRemoved {
		revision, _ := uow.History().CountHistory(ctx, issueID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  revision,
			Author:    author,
			Timestamp: now,
			EventType: history.EventLabelRemoved,
			Changes:   []history.FieldChange{lc},
		}))
		if err != nil {
			return err
		}
	}

	// Extend claim staleAt to reflect the recent activity.
	return uow.Claims().UpdateClaimStaleAt(ctx, claimID, staleAt)
}

// --- Reset ---

func (s *serviceImpl) CountAllIssues(ctx context.Context) (int, error) {
	var count int
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		items, _, listErr := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByPriority, -1)
		if listErr != nil {
			return listErr
		}
		count = len(items)
		return nil
	})
	return count, err
}

func (s *serviceImpl) ResetDatabase(ctx context.Context) error {
	// Read the prefix before clearing so it can be restored afterward.
	// Without this, all commands that depend on the prefix (e.g., create)
	// would fail until the user runs "np init" again.
	var prefix string
	err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		p, readErr := uow.Database().GetPrefix(ctx)
		if readErr != nil {
			return readErr
		}
		prefix = p
		return nil
	})
	if err != nil {
		return fmt.Errorf("reading prefix before reset: %w", err)
	}

	err = s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		if clearErr := uow.Database().ClearAllData(ctx); clearErr != nil {
			return clearErr
		}
		// Restore the prefix so the database remains functional after reset.
		return uow.Database().InitDatabase(ctx, prefix)
	})
	if err != nil {
		return fmt.Errorf("clearing database: %w", err)
	}

	if err := s.tx.Vacuum(ctx); err != nil {
		return fmt.Errorf("vacuuming database: %w", err)
	}

	return nil
}

// enrichListItemSecondaryStates populates the SecondaryState field on each
// IssueListItem. For tasks, the secondary state is derived from IsBlocked and
// State — no extra queries are needed because the SQL already computes blocking
// from both direct blockers and ancestors. For epics, the function queries child
// statuses to determine hasChildren and allChildrenClosed.
func enrichListItemSecondaryStates(ctx context.Context, uow driven.UnitOfWork, items []driven.IssueListItem) {
	for i := range items {
		items[i].SecondaryState = computeListSecondaryState(ctx, uow, items[i])
	}
}

// computeListSecondaryState derives the single list-view secondary state for an
// domain. It uses the pre-computed IsBlocked field (which the repository populates
// via SQL including ancestor blocking) rather than re-querying individual blocker
// and ancestor statuses.
func computeListSecondaryState(ctx context.Context, uow driven.UnitOfWork, item driven.IssueListItem) domain.SecondaryState {
	switch item.State {
	case domain.StateClaimed, domain.StateClosed:
		return domain.SecondaryNone

	case domain.StateDeferred:
		if item.IsBlocked {
			return domain.SecondaryBlocked
		}
		return domain.SecondaryNone

	case domain.StateOpen:
		if item.Role == domain.RoleTask {
			if item.IsBlocked {
				return domain.SecondaryBlocked
			}
			return domain.SecondaryReady
		}
		// Epic: need child information.
		return computeEpicListSecondaryState(ctx, uow, item)

	default:
		return domain.SecondaryNone
	}
}

// computeEpicListSecondaryState handles the open-state epic path, which requires
// querying child statuses to distinguish ready/active/completed/blocked.
func computeEpicListSecondaryState(ctx context.Context, uow driven.UnitOfWork, item driven.IssueListItem) domain.SecondaryState {
	children, err := uow.Issues().GetChildStatuses(ctx, item.ID)
	if err != nil {
		// On error, fall back to simple blocked/ready determination.
		if item.IsBlocked {
			return domain.SecondaryBlocked
		}
		return domain.SecondaryReady
	}

	hasChildren := len(children) > 0
	if !hasChildren {
		if item.IsBlocked {
			return domain.SecondaryBlocked
		}
		return domain.SecondaryReady
	}

	allClosed := true
	for _, c := range children {
		if c.State != domain.StateClosed {
			allClosed = false
			break
		}
	}

	// List-view priority: completed > blocked > active.
	if allClosed {
		return domain.SecondaryCompleted
	}
	if item.IsBlocked {
		return domain.SecondaryBlocked
	}
	return domain.SecondaryActive
}

// epicAllChildrenClosed reports whether all children of an epic are closed. Used
// by ShowIssue to provide the allChildrenClosed input to EpicSecondaryState.
// Returns false when the epic has no children or on query error.
func epicAllChildrenClosed(ctx context.Context, uow driven.UnitOfWork, epicID domain.ID) bool {
	children, err := uow.Issues().GetChildStatuses(ctx, epicID)
	if err != nil || len(children) == 0 {
		return false
	}
	for _, c := range children {
		if c.State != domain.StateClosed {
			return false
		}
	}
	return true
}

// --- driving.Service-to-port translation helpers ---

// toPortFilter translates a service-layer driving.IssueFilterInput to the driven-port
// IssueFilter type used by the repository.
func toPortFilter(f driving.IssueFilterInput) (driven.IssueFilter, error) {
	var lf []driven.LabelFilter
	if len(f.LabelFilters) > 0 {
		lf = make([]driven.LabelFilter, len(f.LabelFilters))
		for i, l := range f.LabelFilters {
			lf[i] = driven.LabelFilter{
				Key:    l.Key,
				Value:  l.Value,
				Negate: l.Negate,
			}
		}
	}

	// Parse string ParentIDs to domain domain.ID values.
	var parentIDs []domain.ID
	for _, s := range f.ParentIDs {
		pid, err := domain.ParseID(s)
		if err != nil {
			return driven.IssueFilter{}, domain.NewValidationError("parent_id", fmt.Sprintf("invalid parent ID %q: %v", s, err))
		}
		parentIDs = append(parentIDs, pid)
	}

	// Parse optional DescendantsOf string to domain.ID.
	var descendantsOf domain.ID
	if f.DescendantsOf != "" {
		var err error
		descendantsOf, err = domain.ParseID(f.DescendantsOf)
		if err != nil {
			return driven.IssueFilter{}, domain.NewValidationError("descendants_of", fmt.Sprintf("invalid issue ID %q: %v", f.DescendantsOf, err))
		}
	}

	// Parse optional AncestorsOf string to domain.ID.
	var ancestorsOf domain.ID
	if f.AncestorsOf != "" {
		var err error
		ancestorsOf, err = domain.ParseID(f.AncestorsOf)
		if err != nil {
			return driven.IssueFilter{}, domain.NewValidationError("ancestors_of", fmt.Sprintf("invalid issue ID %q: %v", f.AncestorsOf, err))
		}
	}

	return driven.IssueFilter{
		Roles:          f.Roles,
		States:         f.States,
		Ready:          f.Ready,
		ParentIDs:      parentIDs,
		DescendantsOf:  descendantsOf,
		AncestorsOf:    ancestorsOf,
		LabelFilters:   lf,
		Orphan:         f.Orphan,
		Blocked:        f.Blocked,
		ExcludeClosed:  f.ExcludeClosed,
		IncludeDeleted: f.IncludeDeleted,
	}, nil
}

// toPortLabelFilters translates a slice of service-layer driving.LabelFilterInput
// to the driven-port LabelFilter type.
func toPortLabelFilters(lfs []driving.LabelFilterInput) []driven.LabelFilter {
	if len(lfs) == 0 {
		return nil
	}
	out := make([]driven.LabelFilter, len(lfs))
	for i, l := range lfs {
		out[i] = driven.LabelFilter{
			Key:    l.Key,
			Value:  l.Value,
			Negate: l.Negate,
		}
	}
	return out
}

// toIssueListItemDTO converts a driven-port IssueListItem to a flat
// service-layer DTO with string fields, so driving adapters do not import the
// domain or port packages. The DisplayStatus field is precomputed from the
// primary state and secondary state.
func toIssueListItemDTO(item driven.IssueListItem) driving.IssueListItemDTO {
	blockerIDs := make([]string, 0, len(item.BlockerIDs))
	for _, id := range item.BlockerIDs {
		blockerIDs = append(blockerIDs, id.String())
	}

	return driving.IssueListItemDTO{
		ID:             item.ID.String(),
		Role:           item.Role,
		State:          item.State,
		Priority:       item.Priority,
		Title:          item.Title,
		ParentID:       item.ParentID.String(),
		CreatedAt:      item.CreatedAt,
		IsDeleted:      item.IsDeleted,
		IsBlocked:      item.IsBlocked,
		BlockerIDs:     blockerIDs,
		SecondaryState: item.SecondaryState,
		DisplayStatus:  item.DisplayStatus(),
	}
}

// toIssueListItemDTOs converts a slice of driven-port IssueListItem values to
// service-layer DTOs.
func toIssueListItemDTOs(items []driven.IssueListItem) []driving.IssueListItemDTO {
	if len(items) == 0 {
		return nil
	}
	out := make([]driving.IssueListItemDTO, len(items))
	for i, item := range items {
		out[i] = toIssueListItemDTO(item)
	}
	return out
}

// toCommentDTO converts a domain Comment to a flat service-layer DTO with
// primitive-typed fields, so driving adapters do not import domain/comment.
func toCommentDTO(c domain.Comment) driving.CommentDTO {
	return driving.CommentDTO{
		CommentID: c.ID(),
		DisplayID: c.DisplayID(),
		IssueID:   c.IssueID().String(),
		Author:    c.Author().String(),
		Body:      c.Body(),
		CreatedAt: c.CreatedAt(),
	}
}

// toCommentDTOs converts a slice of domain Comments to service-layer DTOs.
func toCommentDTOs(comments []domain.Comment) []driving.CommentDTO {
	if len(comments) == 0 {
		return nil
	}
	out := make([]driving.CommentDTO, len(comments))
	for i, c := range comments {
		out[i] = toCommentDTO(c)
	}
	return out
}

// toHistoryEntryDTO converts a domain history Entry into a flat DTO.
func toHistoryEntryDTO(e history.Entry) driving.HistoryEntryDTO {
	changes := make([]driving.FieldChangeDTO, len(e.Changes()))
	for i, c := range e.Changes() {
		changes[i] = driving.FieldChangeDTO{
			Field:  c.Field,
			Before: c.Before,
			After:  c.After,
		}
	}
	return driving.HistoryEntryDTO{
		IssueID:   e.IssueID().String(),
		Revision:  e.Revision(),
		Author:    e.Author().String(),
		Timestamp: e.Timestamp(),
		EventType: e.EventType().String(),
		Changes:   changes,
	}
}

// toHistoryEntryDTOs converts a slice of history entries to DTOs.
func toHistoryEntryDTOs(entries []history.Entry) []driving.HistoryEntryDTO {
	if len(entries) == 0 {
		return nil
	}
	out := make([]driving.HistoryEntryDTO, len(entries))
	for i, e := range entries {
		out[i] = toHistoryEntryDTO(e)
	}
	return out
}

// toPortHistoryFilter translates a service-layer driving.HistoryFilterInput to the
// driven-port HistoryFilter type.
func toPortHistoryFilter(f driving.HistoryFilterInput) driven.HistoryFilter {
	pf := driven.HistoryFilter{
		After:  f.After,
		Before: f.Before,
	}
	if f.Author != "" {
		a, err := domain.NewAuthor(f.Author)
		if err == nil {
			pf.Author = a
		}
	}
	return pf
}

// toPortCommentFilter translates a service-layer driving.CommentFilterInput to the
// driven-port CommentFilter type. String fields (authors, issue IDs, parent
// IDs, tree IDs) are parsed into their domain types; an error is returned if
// any value is invalid.
func toPortCommentFilter(f driving.CommentFilterInput) (driven.CommentFilter, error) {
	var authors []domain.Author
	for _, s := range f.Authors {
		a, err := domain.NewAuthor(s)
		if err != nil {
			return driven.CommentFilter{}, fmt.Errorf("invalid author %q: %w", s, err)
		}
		authors = append(authors, a)
	}

	issueIDs, err := parseIDSlice(f.IssueIDs)
	if err != nil {
		return driven.CommentFilter{}, fmt.Errorf("invalid issue ID in filter: %w", err)
	}

	parentIDs, err := parseIDSlice(f.ParentIDs)
	if err != nil {
		return driven.CommentFilter{}, fmt.Errorf("invalid parent ID in filter: %w", err)
	}

	treeIDs, err := parseIDSlice(f.TreeIDs)
	if err != nil {
		return driven.CommentFilter{}, fmt.Errorf("invalid tree ID in filter: %w", err)
	}

	pf := driven.CommentFilter{
		Authors:        authors,
		CreatedAfter:   f.CreatedAfter,
		AfterCommentID: f.AfterCommentID,
		IssueIDs:       issueIDs,
		ParentIDs:      parentIDs,
		TreeIDs:        treeIDs,
		LabelFilters:   toPortLabelFilters(f.LabelFilters),
		FollowRefs:     f.FollowRefs,
	}
	// Set singular Author for repository implementations that only check it.
	if len(authors) == 1 {
		pf.Author = authors[0]
	}
	return pf, nil
}

// parseIDSlice parses a slice of string issue IDs into domain domain.ID values.
func parseIDSlice(ss []string) ([]domain.ID, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	out := make([]domain.ID, len(ss))
	for i, s := range ss {
		id, err := domain.ParseID(s)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", s, err)
		}
		out[i] = id
	}
	return out, nil
}

// toPortOrderBy translates a service-layer driving.OrderBy to the driven-port
// IssueOrderBy type.
func toPortOrderBy(o driving.OrderBy) driven.IssueOrderBy {
	switch o {
	case driving.OrderByCreatedAt:
		return driven.OrderByCreatedAt
	case driving.OrderByUpdatedAt:
		return driven.OrderByUpdatedAt
	default:
		return driven.OrderByPriority
	}
}

// toLabelSlice converts a slice of driving.LabelInput DTOs to domain domain.Label
// values, validating each entry. Returns an error if any label is invalid.
func toLabelSlice(inputs []driving.LabelInput) ([]domain.Label, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	labels := make([]domain.Label, 0, len(inputs))
	for _, li := range inputs {
		l, err := domain.NewLabel(li.Key, li.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid label %q:%q: %w", li.Key, li.Value, err)
		}
		labels = append(labels, l)
	}
	return labels, nil
}
