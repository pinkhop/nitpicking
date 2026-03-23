package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/note"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
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
	if err := ticket.ValidatePrefix(prefix); err != nil {
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

// --- Ticket Operations ---

func (s *serviceImpl) CreateTicket(ctx context.Context, input CreateTicketInput) (CreateTicketOutput, error) {
	var output CreateTicketOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		// Check idempotency key.
		if input.IdempotencyKey != "" {
			existing, err := uow.Tickets().GetTicketByIdempotencyKey(ctx, input.IdempotencyKey)
			if err == nil {
				output.Ticket = existing
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

		id, err := ticket.GenerateID(prefix, func(id ticket.ID) (bool, error) {
			return uow.Tickets().TicketIDExists(ctx, id)
		})
		if err != nil {
			return err
		}

		now := time.Now()

		// Create ticket.
		var t ticket.Ticket
		switch input.Role {
		case ticket.RoleTask:
			t, err = ticket.NewTask(ticket.NewTaskParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           input.Priority,
				ParentID:           input.ParentID,
				Facets:             ticket.FacetSetFrom(input.Facets),
				CreatedAt:          now,
				IdempotencyKey:     input.IdempotencyKey,
			})
		case ticket.RoleEpic:
			t, err = ticket.NewEpic(ticket.NewEpicParams{
				ID:                 id,
				Title:              input.Title,
				Description:        input.Description,
				AcceptanceCriteria: input.AcceptanceCriteria,
				Priority:           input.Priority,
				ParentID:           input.ParentID,
				Facets:             ticket.FacetSetFrom(input.Facets),
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

		if err := uow.Tickets().CreateTicket(ctx, t); err != nil {
			return err
		}

		// Record creation history.
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			TicketID:  id,
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
			rel, err := ticket.NewRelationship(id, ri.TargetID, ri.Type)
			if err != nil {
				return err
			}
			// Validate target exists and is not deleted.
			target, err := uow.Tickets().GetTicket(ctx, ri.TargetID, false)
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
				TicketID: id,
				Author:   input.Author,
				Now:      now,
			})
			if err != nil {
				return err
			}

			t = t.WithState(ticket.StateClaimed)
			if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
				return err
			}
			if err := uow.Claims().CreateClaim(ctx, c); err != nil {
				return err
			}

			revision, _ := uow.History().CountHistory(ctx, id)
			_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
				TicketID:  id,
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

		output.Ticket = t
		return nil
	})

	return output, err
}

func (s *serviceImpl) ClaimByID(ctx context.Context, input ClaimInput) (ClaimOutput, error) {
	var output ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		t, err := uow.Tickets().GetTicket(ctx, input.TicketID, true)
		if err != nil {
			return err
		}

		// Build claim status.
		activeClaim, err := uow.Claims().GetClaimByTicket(ctx, input.TicketID)
		if err != nil && err != domain.ErrNotFound {
			return err
		}

		status := claim.TicketClaimStatus{
			State:       t.State(),
			IsDeleted:   t.IsDeleted(),
			ActiveClaim: activeClaim,
		}

		if err := claim.ValidateClaim(status, input.AllowSteal, now); err != nil {
			return err
		}

		// Steal if needed.
		if activeClaim.ID() != "" {
			output.Stolen = true
			previousHolder := activeClaim.Author().String()

			// Invalidate old claim.
			if err := uow.Claims().InvalidateClaim(ctx, activeClaim.ID()); err != nil {
				return err
			}

			// Add steal note.
			stealNote, err := note.NewNote(note.NewNoteParams{
				TicketID:  input.TicketID,
				Author:    input.Author,
				CreatedAt: now,
				Body:      claim.StealNote(previousHolder),
			})
			if err != nil {
				return err
			}
			if _, err := uow.Notes().CreateNote(ctx, stealNote); err != nil {
				return err
			}
		}

		// Create new claim.
		c, err := claim.NewClaim(claim.NewClaimParams{
			TicketID:       input.TicketID,
			Author:         input.Author,
			StaleThreshold: input.StaleThreshold,
			Now:            now,
		})
		if err != nil {
			return err
		}

		// Update ticket state to claimed.
		t = t.WithState(ticket.StateClaimed)
		if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
			return err
		}
		if err := uow.Claims().CreateClaim(ctx, c); err != nil {
			return err
		}

		// Record claim history.
		revision, _ := uow.History().CountHistory(ctx, input.TicketID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			TicketID:  input.TicketID,
			Revision:  revision,
			Author:    input.Author,
			Timestamp: now,
			EventType: history.EventClaimed,
		}))
		if err != nil {
			return err
		}

		output.ClaimID = c.ID()
		output.TicketID = input.TicketID
		return nil
	})

	return output, err
}

func (s *serviceImpl) ClaimNextReady(ctx context.Context, input ClaimNextReadyInput) (ClaimOutput, error) {
	var output ClaimOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		filter := port.TicketFilter{
			Ready:        true,
			Role:         input.Role,
			FacetFilters: input.FacetFilters,
		}

		items, _, err := uow.Tickets().ListTickets(ctx, filter, port.OrderByPriority, port.PageRequest{PageSize: 1})
		if err != nil {
			return err
		}

		if len(items) > 0 {
			result, err := s.ClaimByID(ctx, ClaimInput{
				TicketID:       items[0].ID,
				Author:         input.Author,
				StaleThreshold: input.StaleThreshold,
			})
			if err != nil {
				return err
			}
			output = result
			return nil
		}

		// No ready tickets — try steal fallback.
		if !input.StealFallback {
			return domain.ErrNotFound
		}

		staleClaims, err := uow.Claims().ListStaleClaims(ctx, time.Now())
		if err != nil {
			return err
		}

		if len(staleClaims) == 0 {
			return domain.ErrNotFound
		}

		// Steal the highest-priority stale ticket.
		result, err := s.ClaimByID(ctx, ClaimInput{
			TicketID:       staleClaims[0].TicketID(),
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

		// Claim.
		claimResult, err := s.ClaimByID(ctx, ClaimInput{
			TicketID: input.TicketID,
			Author:   input.Author,
		})
		if err != nil {
			return err
		}

		// Update.
		if err := s.applyTicketUpdates(ctx, uow, input.TicketID, claimResult.ClaimID, input.Author, now, oneShotToUpdateFields(input)); err != nil {
			return err
		}

		// Release.
		return s.releaseTicket(ctx, uow, input.TicketID, claimResult.ClaimID, input.Author, now)
	})
}

func (s *serviceImpl) UpdateTicket(ctx context.Context, input UpdateTicketInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Verify claim.
		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.TicketID() != input.TicketID {
			return fmt.Errorf("claim %s does not match ticket %s", input.ClaimID, input.TicketID)
		}

		if err := s.applyTicketUpdates(ctx, uow, input.TicketID, input.ClaimID, c.Author(), now, updateFieldsFromInput(input)); err != nil {
			return err
		}

		// Add note if provided.
		if input.NoteBody != "" {
			n, err := note.NewNote(note.NewNoteParams{
				TicketID:  input.TicketID,
				Author:    c.Author(),
				CreatedAt: now,
				Body:      input.NoteBody,
			})
			if err != nil {
				return err
			}
			if _, err := uow.Notes().CreateNote(ctx, n); err != nil {
				return err
			}
		}

		// Update claim last activity.
		return uow.Claims().UpdateClaimLastActivity(ctx, input.ClaimID, now)
	})
}

func (s *serviceImpl) ExtendStaleThreshold(ctx context.Context, ticketID ticket.ID, claimID string, threshold time.Duration) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		c, err := uow.Claims().GetClaimByID(ctx, claimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.TicketID() != ticketID {
			return fmt.Errorf("claim %s does not match ticket %s", claimID, ticketID)
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
		if c.TicketID() != input.TicketID {
			return fmt.Errorf("claim %s does not match ticket %s", input.ClaimID, input.TicketID)
		}

		t, err := uow.Tickets().GetTicket(ctx, input.TicketID, false)
		if err != nil {
			return err
		}

		switch input.Action {
		case ActionRelease:
			return s.releaseTicket(ctx, uow, input.TicketID, input.ClaimID, c.Author(), now)
		case ActionClose:
			return s.closeTicket(ctx, uow, t, input.ClaimID, c.Author(), now)
		case ActionDefer:
			return s.transitionTicket(ctx, uow, t, input.ClaimID, c.Author(), now, ticket.StateDeferred)
		case ActionWait:
			return s.transitionTicket(ctx, uow, t, input.ClaimID, c.Author(), now, ticket.StateWaiting)
		default:
			return fmt.Errorf("invalid transition action")
		}
	})
}

func (s *serviceImpl) DeleteTicket(ctx context.Context, input DeleteInput) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		c, err := uow.Claims().GetClaimByID(ctx, input.ClaimID)
		if err != nil {
			return fmt.Errorf("invalid claim ID: %w", err)
		}
		if c.TicketID() != input.TicketID {
			return fmt.Errorf("claim %s does not match ticket %s", input.ClaimID, input.TicketID)
		}

		t, err := uow.Tickets().GetTicket(ctx, input.TicketID, false)
		if err != nil {
			return err
		}

		if err := ticket.ValidateDeletion(t.IsDeleted()); err != nil {
			return err
		}

		// For epics, check descendants.
		if t.IsEpic() {
			descendants, err := uow.Tickets().GetDescendants(ctx, input.TicketID)
			if err != nil {
				return err
			}

			result := ticket.PlanEpicDeletion(input.TicketID, descendants)
			if len(result.Conflicts) > 0 {
				return &domain.ClaimConflictError{
					TicketID:      input.TicketID.String(),
					CurrentHolder: result.Conflicts[0].ClaimedBy,
					StaleAt:       time.Now(),
				}
			}

			// Delete all descendants.
			for _, id := range result.ToDelete {
				if id == input.TicketID {
					continue
				}
				descendant, err := uow.Tickets().GetTicket(ctx, id, false)
				if err != nil {
					continue
				}
				deleted := descendant.WithDeleted()
				if err := uow.Tickets().UpdateTicket(ctx, deleted); err != nil {
					return err
				}

				revision, _ := uow.History().CountHistory(ctx, id)
				_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
					TicketID:  id,
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

		// Delete the ticket itself.
		deleted := t.WithDeleted()
		if err := uow.Tickets().UpdateTicket(ctx, deleted); err != nil {
			return err
		}
		if err := uow.Claims().InvalidateClaim(ctx, input.ClaimID); err != nil {
			return err
		}

		revision, _ := uow.History().CountHistory(ctx, input.TicketID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			TicketID:  input.TicketID,
			Revision:  revision,
			Author:    c.Author(),
			Timestamp: now,
			EventType: history.EventDeleted,
		}))
		return err
	})
}

func (s *serviceImpl) ShowTicket(ctx context.Context, id ticket.ID) (ShowTicketOutput, error) {
	var output ShowTicketOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		t, err := uow.Tickets().GetTicket(ctx, id, false)
		if err != nil {
			return err
		}

		output.Ticket = t

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
		ancestors, _ := uow.Tickets().GetAncestorStatuses(ctx, id)

		if t.IsTask() {
			output.IsReady = ticket.IsTaskReady(t.State(), blockers, ancestors)
		} else {
			hasChildren, _ := uow.Tickets().HasChildren(ctx, id)
			output.IsReady = ticket.IsEpicReady(t.State(), hasChildren, blockers, ancestors)

			// Completion.
			children, _ := uow.Tickets().GetChildStatuses(ctx, id)
			output.IsComplete = ticket.IsEpicComplete(children)
		}

		// Claim info.
		activeClaim, err := uow.Claims().GetClaimByTicket(ctx, id)
		if err == nil {
			output.ClaimID = activeClaim.ID()
			output.ClaimAuthor = activeClaim.Author().String()
			output.ClaimStaleAt = activeClaim.StaleAt()
		}

		return nil
	})

	return output, err
}

func (s *serviceImpl) ListTickets(ctx context.Context, input ListTicketsInput) (ListTicketsOutput, error) {
	var output ListTicketsOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		items, result, err := uow.Tickets().ListTickets(ctx, input.Filter, input.OrderBy, input.Page)
		if err != nil {
			return err
		}
		output.Items = items
		output.TotalCount = result.TotalCount
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchTickets(ctx context.Context, input SearchTicketsInput) (ListTicketsOutput, error) {
	var output ListTicketsOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		items, result, err := uow.Tickets().SearchTickets(ctx, input.Query, input.Filter, input.OrderBy, input.Page)
		if err != nil {
			return err
		}
		output.Items = items
		output.TotalCount = result.TotalCount
		return nil
	})

	return output, err
}

// --- Relationship Operations ---

func (s *serviceImpl) AddRelationship(ctx context.Context, sourceID ticket.ID, ri RelationshipInput, author identity.Author) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Validate target exists and is not deleted.
		if _, err := uow.Tickets().GetTicket(ctx, ri.TargetID, false); err != nil {
			return fmt.Errorf("relationship target %s: %w", ri.TargetID, err)
		}

		rel, err := ticket.NewRelationship(sourceID, ri.TargetID, ri.Type)
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
				TicketID:  sourceID,
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

func (s *serviceImpl) RemoveRelationship(ctx context.Context, sourceID ticket.ID, ri RelationshipInput, author identity.Author) error {
	return s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		deleted, err := uow.Relationships().DeleteRelationship(ctx, sourceID, ri.TargetID, ri.Type)
		if err != nil {
			return err
		}

		if deleted {
			revision, _ := uow.History().CountHistory(ctx, sourceID)
			_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
				TicketID:  sourceID,
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

// --- Note Operations ---

func (s *serviceImpl) AddNote(ctx context.Context, input AddNoteInput) (AddNoteOutput, error) {
	var output AddNoteOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		now := time.Now()

		// Verify ticket exists and is not deleted.
		t, err := uow.Tickets().GetTicket(ctx, input.TicketID, true)
		if err != nil {
			return err
		}
		if t.IsDeleted() {
			return fmt.Errorf("cannot add note to deleted ticket: %w", domain.ErrDeletedTicket)
		}

		n, err := note.NewNote(note.NewNoteParams{
			TicketID:  input.TicketID,
			Author:    input.Author,
			CreatedAt: now,
			Body:      input.Body,
		})
		if err != nil {
			return err
		}

		id, err := uow.Notes().CreateNote(ctx, n)
		if err != nil {
			return err
		}

		// Update claim last activity if claimed.
		activeClaim, err := uow.Claims().GetClaimByTicket(ctx, input.TicketID)
		if err == nil {
			_ = uow.Claims().UpdateClaimLastActivity(ctx, activeClaim.ID(), now)
		}

		output.Note, err = uow.Notes().GetNote(ctx, id)
		return err
	})

	return output, err
}

func (s *serviceImpl) ShowNote(ctx context.Context, noteID int64) (note.Note, error) {
	var result note.Note

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		n, err := uow.Notes().GetNote(ctx, noteID)
		if err != nil {
			return err
		}
		result = n
		return nil
	})

	return result, err
}

func (s *serviceImpl) ListNotes(ctx context.Context, input ListNotesInput) (ListNotesOutput, error) {
	var output ListNotesOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		notes, result, err := uow.Notes().ListNotes(ctx, input.TicketID, input.Filter, input.Page)
		if err != nil {
			return err
		}
		output.Notes = notes
		output.TotalCount = result.TotalCount
		return nil
	})

	return output, err
}

func (s *serviceImpl) SearchNotes(ctx context.Context, input SearchNotesInput) (ListNotesOutput, error) {
	var output ListNotesOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		filter := input.Filter
		filter.TicketID = input.TicketID

		notes, result, err := uow.Notes().SearchNotes(ctx, input.Query, filter, input.Page)
		if err != nil {
			return err
		}
		output.Notes = notes
		output.TotalCount = result.TotalCount
		return nil
	})

	return output, err
}

// --- History Operations ---

func (s *serviceImpl) ShowHistory(ctx context.Context, input ListHistoryInput) (ListHistoryOutput, error) {
	var output ListHistoryOutput

	err := s.tx.WithReadTransaction(ctx, func(uow port.UnitOfWork) error {
		entries, result, err := uow.History().ListHistory(ctx, input.TicketID, input.Filter, input.Page)
		if err != nil {
			return err
		}
		output.Entries = entries
		output.TotalCount = result.TotalCount
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
				Category:  "stale_claim",
				Severity:  "warning",
				Message:   fmt.Sprintf("Ticket %s has been claimed by %q since %s (stale)", c.TicketID(), c.Author(), c.LastActivity().Format(time.RFC3339)),
				TicketIDs: []string{c.TicketID().String()},
			})
		}

		return nil
	})

	return output, err
}

func (s *serviceImpl) GC(ctx context.Context, input GCInput) (GCOutput, error) {
	var output GCOutput

	err := s.tx.WithTransaction(ctx, func(uow port.UnitOfWork) error {
		return uow.Database().GC(ctx, input.IncludeClosed)
	})

	return output, err
}

// --- Internal helpers ---

func (s *serviceImpl) validateParent(ctx context.Context, uow port.UnitOfWork, childID, parentID ticket.ID) error {
	parent, err := uow.Tickets().GetTicket(ctx, parentID, true)
	if err != nil {
		return fmt.Errorf("parent %s: %w", parentID, err)
	}
	if err := ticket.ValidateParent(childID, parentID, parent.Role(), parent.IsDeleted()); err != nil {
		return err
	}
	return ticket.ValidateNoCycle(childID, parentID, func(id ticket.ID) (ticket.ID, error) {
		return uow.Tickets().GetParentID(ctx, id)
	})
}

func (s *serviceImpl) releaseTicket(ctx context.Context, uow port.UnitOfWork, ticketID ticket.ID, claimID string, author identity.Author, now time.Time) error {
	t, err := uow.Tickets().GetTicket(ctx, ticketID, false)
	if err != nil {
		return err
	}

	releaseState := ticket.ReleaseStateForRole(t.Role())

	t = t.WithState(releaseState)
	if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, ticketID)
	_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		TicketID:  ticketID,
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventReleased,
		Changes: []history.FieldChange{
			{Field: "state", Before: ticket.StateClaimed.String(), After: releaseState.String()},
		},
	}))
	return err
}

func (s *serviceImpl) closeTicket(ctx context.Context, uow port.UnitOfWork, t ticket.Ticket, claimID string, author identity.Author, now time.Time) error {
	if !t.IsTask() {
		return fmt.Errorf("only tasks can be closed: %w", domain.ErrIllegalTransition)
	}

	if err := ticket.TransitionTask(t.State(), ticket.StateClosed); err != nil {
		return err
	}

	t = t.WithState(ticket.StateClosed)
	if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, t.ID())
	_, err := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		TicketID:  t.ID(),
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventStateChanged,
		Changes: []history.FieldChange{
			{Field: "state", Before: ticket.StateClaimed.String(), After: ticket.StateClosed.String()},
		},
	}))
	return err
}

func (s *serviceImpl) transitionTicket(ctx context.Context, uow port.UnitOfWork, t ticket.Ticket, claimID string, author identity.Author, now time.Time, targetState ticket.State) error {
	var transErr error
	if t.IsTask() {
		transErr = ticket.TransitionTask(t.State(), targetState)
	} else {
		transErr = ticket.TransitionEpic(t.State(), targetState)
	}
	if transErr != nil {
		return transErr
	}

	t = t.WithState(targetState)
	if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
		return err
	}
	if err := uow.Claims().InvalidateClaim(ctx, claimID); err != nil {
		return err
	}

	revision, _ := uow.History().CountHistory(ctx, t.ID())
	_, err := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		TicketID:  t.ID(),
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventStateChanged,
		Changes: []history.FieldChange{
			{Field: "state", Before: ticket.StateClaimed.String(), After: targetState.String()},
		},
	}))
	return err
}

// updateFields groups optional field updates for a ticket.
type updateFields struct {
	Title              *string
	Description        *string
	AcceptanceCriteria *string
	Priority           *ticket.Priority
	ParentID           *ticket.ID
	FacetSet           []ticket.Facet
	FacetRemove        []string
}

func oneShotToUpdateFields(input OneShotUpdateInput) updateFields {
	return updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		ParentID:           input.ParentID,
		FacetSet:           input.FacetSet,
		FacetRemove:        input.FacetRemove,
	}
}

func updateFieldsFromInput(input UpdateTicketInput) updateFields {
	return updateFields{
		Title:              input.Title,
		Description:        input.Description,
		AcceptanceCriteria: input.AcceptanceCriteria,
		Priority:           input.Priority,
		ParentID:           input.ParentID,
		FacetSet:           input.FacetSet,
		FacetRemove:        input.FacetRemove,
	}
}

func (s *serviceImpl) applyTicketUpdates(ctx context.Context, uow port.UnitOfWork, ticketID ticket.ID, claimID string, author identity.Author, now time.Time, fields updateFields) error {
	t, err := uow.Tickets().GetTicket(ctx, ticketID, false)
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
			if err := s.validateParent(ctx, uow, ticketID, newParentID); err != nil {
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

	// Apply facet changes.
	facets := t.Facets()
	for _, f := range fields.FacetSet {
		facets = facets.Set(f)
	}
	for _, key := range fields.FacetRemove {
		facets = facets.Remove(key)
	}
	t = t.WithFacets(facets)

	if err := uow.Tickets().UpdateTicket(ctx, t); err != nil {
		return err
	}

	if len(changes) > 0 {
		revision, _ := uow.History().CountHistory(ctx, ticketID)
		_, err = uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
			TicketID:  ticketID,
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
