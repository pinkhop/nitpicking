package core

import (
	"context"
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// ImportIssues creates issues from a slice of validated import records. Each
// record is processed independently — a failure on one line does not prevent
// subsequent lines from being imported.
//
// The function performs three phases:
//  1. Create all issues (collecting the idempotency-label-to-ID mapping).
//  2. Add relationships (blocked_by, blocks, refs) using the mapping.
//  3. Add comments and transition states (deferred/closed) as needed.
//
// Phase 2 and 3 are best-effort: errors are recorded in the per-line results
// but do not prevent other operations from proceeding.
func (s *serviceImpl) ImportIssues(ctx context.Context, input driving.ImportInput) (driving.ImportOutput, error) {
	var output driving.ImportOutput
	output.Results = make([]driving.ImportLineResult, len(input.Records))

	// Phase 1: create issues and build idempotency-label-to-ID mapping.
	// Keys are the canonical "key:value" string of each record's IdempotencyLabel.
	keyToID := make(map[string]domain.ID, len(input.Records))

	for i, rec := range input.Records {
		author := s.resolveImportAuthor(rec, input)

		// Convert domain labels from the validated record to service-layer
		// LabelInput DTOs.
		labelInputs := make([]driving.LabelInput, len(rec.Labels))
		for j, l := range rec.Labels {
			labelInputs[j] = driving.LabelInput{Key: l.Key(), Value: l.Value()}
		}

		createInput := driving.CreateIssueInput{
			Role:               rec.Role,
			Title:              rec.Title,
			Description:        rec.Description,
			AcceptanceCriteria: rec.AcceptanceCriteria,
			Priority:           rec.Priority,
			Labels:             labelInputs,
			Author:             author,
			IdempotencyLabel:   rec.IdempotencyLabel,
			// Claim creates a transient claim row immediately after creation,
			// leaving the issue open but claimed. Only set when rec.Claim is
			// true; validation ensures Claim is only true for open records.
			Claim: rec.Claim,
		}

		// Resolve parent reference if present.
		if rec.Parent != "" {
			parentID, resolveErr := s.resolveImportRef(ctx, rec.Parent, keyToID)
			if resolveErr != nil {
				output.Results[i] = driving.ImportLineResult{
					IdempotencyLabel: rec.IdempotencyLabel,
					Err:              fmt.Errorf("resolving parent: %w", resolveErr),
				}
				output.Failed++
				continue
			}
			createInput.ParentID = parentID.String()
		}

		createOut, err := s.CreateIssue(ctx, createInput)
		if err != nil {
			output.Results[i] = driving.ImportLineResult{
				IdempotencyLabel: rec.IdempotencyLabel,
				Err:              fmt.Errorf("creating issue: %w", err),
			}
			output.Failed++
			continue
		}

		// Key the intra-file idempotency-label map by the canonical
		// "key:value" string so that reference resolution in Phase 2 can
		// look up created issue IDs by their idempotency label string.
		// This is done for both new and deduplicated issues so that subsequent
		// records in the same batch can reference them by label.
		issueID := createOut.Issue.ID()
		keyToID[rec.IdempotencyLabel.String()] = issueID
		output.Results[i] = driving.ImportLineResult{
			IdempotencyLabel: rec.IdempotencyLabel,
			IssueID:          issueID,
			Skipped:          createOut.Skipped,
		}
		if createOut.Skipped {
			// The issue already exists; count the skip and do not apply
			// relationships, comments, or state transitions from this record.
			// The existing issue is not mutated on re-import.
			output.Skipped++
		} else {
			output.Created++
		}
	}

	// Phase 2: add relationships.
	for i, rec := range input.Records {
		if output.Results[i].Err != nil {
			continue // Skip failed issues.
		}
		if output.Results[i].Skipped {
			continue // Skip deduplicated records — the existing issue is not mutated.
		}
		issueID := output.Results[i].IssueID
		if issueID.IsZero() {
			continue
		}

		author := s.resolveImportAuthor(rec, input)

		// blocked_by: this issue is blocked by the target.
		for _, ref := range rec.BlockedBy {
			targetID, err := s.resolveImportRef(ctx, ref, keyToID)
			if err != nil {
				output.Results[i].Err = fmt.Errorf("resolving blocked_by ref %q: %w", ref, err)
				continue
			}
			if err := s.AddRelationship(ctx, issueID.String(), driving.RelationshipInput{
				Type:     domain.RelBlockedBy,
				TargetID: targetID.String(),
			}, author); err != nil {
				output.Results[i].Err = fmt.Errorf("adding blocked_by relationship: %w", err)
			}
		}

		// blocks: the target is blocked by this domain.
		for _, ref := range rec.Blocks {
			targetID, err := s.resolveImportRef(ctx, ref, keyToID)
			if err != nil {
				output.Results[i].Err = fmt.Errorf("resolving blocks ref %q: %w", ref, err)
				continue
			}
			if err := s.AddRelationship(ctx, targetID.String(), driving.RelationshipInput{
				Type:     domain.RelBlockedBy,
				TargetID: issueID.String(),
			}, author); err != nil {
				output.Results[i].Err = fmt.Errorf("adding blocks relationship: %w", err)
			}
		}

		// refs: informational reference from this issue to the target.
		for _, ref := range rec.Refs {
			targetID, err := s.resolveImportRef(ctx, ref, keyToID)
			if err != nil {
				output.Results[i].Err = fmt.Errorf("resolving refs ref %q: %w", ref, err)
				continue
			}
			if err := s.AddRelationship(ctx, issueID.String(), driving.RelationshipInput{
				Type:     domain.RelRefs,
				TargetID: targetID.String(),
			}, author); err != nil {
				output.Results[i].Err = fmt.Errorf("adding ref relationship: %w", err)
			}
		}
	}

	// Phase 3: add comments and transition states.
	for i, rec := range input.Records {
		if output.Results[i].Err != nil {
			continue
		}
		if output.Results[i].Skipped {
			continue // Skip deduplicated records — the existing issue is not mutated.
		}
		issueID := output.Results[i].IssueID
		if issueID.IsZero() {
			continue
		}

		author := s.resolveImportAuthor(rec, input)

		// Add comment if present.
		if rec.Comment != "" {
			if _, err := s.AddComment(ctx, driving.AddCommentInput{
				IssueID: issueID.String(),
				Author:  author,
				Body:    rec.Comment,
			}); err != nil {
				output.Results[i].Err = fmt.Errorf("adding comment: %w", err)
			}
		}

		// Transition state if not open.
		if rec.State != domain.StateOpen {
			if err := s.transitionImportedIssue(ctx, issueID, rec.State, author); err != nil {
				output.Results[i].Err = fmt.Errorf("transitioning to %s: %w", rec.State, err)
			}
		}
	}

	return output, nil
}

// resolveImportAuthor returns the effective author for an import record,
// applying the default/force-author rules.
func (s *serviceImpl) resolveImportAuthor(rec domain.ValidatedRecord, input driving.ImportInput) string {
	if input.ForceAuthor || rec.Author == "" {
		return input.DefaultAuthor
	}
	return rec.Author
}

// resolveImportRef resolves a reference string to an issue ID. It first
// checks the intra-file idempotency-label-to-ID mapping (for intra-file
// references), then falls back to parsing it as an issue ID.
func (s *serviceImpl) resolveImportRef(ctx context.Context, ref string, keyToID map[string]domain.ID) (domain.ID, error) {
	// Check intra-file mapping first.
	if id, ok := keyToID[ref]; ok {
		return id, nil
	}

	// Try parsing as an issue ID.
	id, err := domain.ParseID(ref)
	if err != nil {
		return domain.ID{}, fmt.Errorf("cannot resolve reference %q: not a known idempotency label or valid issue ID", ref)
	}

	// Verify the issue exists.
	_, showErr := s.ShowIssue(ctx, id.String())
	if showErr != nil {
		return domain.ID{}, fmt.Errorf("issue %s does not exist: %w", id, showErr)
	}

	return id, nil
}

// transitionImportedIssue acquires a transient claim on the issue, transitions
// it directly from open to the target state, and releases the claim as part of
// the transition. Claiming does not change the primary state — the issue
// remains open until the transition completes. This is only called for
// non-open target states (deferred, closed).
func (s *serviceImpl) transitionImportedIssue(ctx context.Context, issueID domain.ID, targetState domain.State, author string) error {
	// Acquire a transient claim to authorize the transition. The issue state
	// remains open; claiming is local-only bookkeeping.
	claimOut, err := s.ClaimByID(ctx, driving.ClaimInput{
		IssueID: issueID.String(),
		Author:  author,
	})
	if err != nil {
		return fmt.Errorf("claiming for state transition: %w", err)
	}

	var action driving.TransitionAction
	switch targetState {
	case domain.StateDeferred:
		action = driving.ActionDefer
	case domain.StateClosed:
		action = driving.ActionClose
	default:
		// This path should not be reached: ImportIssues only calls
		// transitionImportedIssue when rec.State != StateOpen, and the only
		// non-open importable states are deferred and closed.
		_ = s.TransitionState(ctx, driving.TransitionInput{
			IssueID: issueID.String(),
			ClaimID: claimOut.ClaimID,
			Action:  driving.ActionRelease,
		})
		return fmt.Errorf("unsupported import target state %q", targetState)
	}

	if err := s.TransitionState(ctx, driving.TransitionInput{
		IssueID: issueID.String(),
		ClaimID: claimOut.ClaimID,
		Action:  action,
	}); err != nil {
		// Best effort: release the claim even if the transition fails, so the
		// issue does not remain permanently locked.
		_ = s.TransitionState(ctx, driving.TransitionInput{
			IssueID: issueID.String(),
			ClaimID: claimOut.ClaimID,
			Action:  driving.ActionRelease,
		})
		return err
	}

	return nil
}
