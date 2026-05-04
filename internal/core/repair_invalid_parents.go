package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// RepairInvalidParentReferences scans every non-deleted issue for a parent_id
// that refers to an absent or soft-deleted parent. In non-dry-run mode it
// clears those references and records an audit comment on each affected issue —
// all within a single transaction so that a partial failure cannot leave the
// database in a half-repaired state.
//
// This operation bypasses the normal bearer-claim path. Ordinarily every
// mutation requires an active claim, but issues with dangling parent references
// are in an inconsistent state that may prevent claiming from working cleanly.
// The audit comment recorded on each repaired issue provides the same
// accountability that a claim would. This exception is per the specification
// for `np admin fix invalid-parent-reference`.
func (s *serviceImpl) RepairInvalidParentReferences(ctx context.Context, input driving.RepairInvalidParentsInput) (driving.RepairInvalidParentsOutput, error) {
	var output driving.RepairInvalidParentsOutput

	// Validate author unconditionally so that dry-run rehearses the same input
	// validation as a live run. A caller that passes an invalid author in
	// dry-run would discover the error only on re-invocation without --dry-run.
	author, err := parseAuthor(input.Author)
	if err != nil {
		return output, err
	}

	if input.DryRun {
		// Scan only — no writes. A read transaction is sufficient.
		err := s.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
			affected, err := scanInvalidParentReferences(ctx, uow)
			if err != nil {
				return err
			}
			for _, ref := range affected {
				output.Repaired = append(output.Repaired, driving.RepairedParentRecord{
					IssueID:         ref.issueID.String(),
					RemovedParentID: ref.parentID.String(),
				})
			}
			return nil
		})
		return output, err
	}

	// Scan and repair within a single transaction so that a failure on any
	// individual repair rolls back the entire batch.
	err = s.tx.WithTransaction(ctx, func(uow driven.UnitOfWork) error {
		affected, scanErr := scanInvalidParentReferences(ctx, uow)
		if scanErr != nil {
			return scanErr
		}

		now := time.Now()

		for _, ref := range affected {
			if repairErr := repairDanglingParent(ctx, uow, ref, author, now); repairErr != nil {
				return repairErr
			}
			output.Repaired = append(output.Repaired, driving.RepairedParentRecord{
				IssueID:         ref.issueID.String(),
				RemovedParentID: ref.parentID.String(),
			})
		}
		return nil
	})
	return output, err
}

// danglingParentRef is an internal record of an issue whose parent_id refers
// to a parent that is absent or soft-deleted.
type danglingParentRef struct {
	issueID  domain.ID
	parentID domain.ID
}

// scanInvalidParentReferences lists all non-deleted issues and returns those
// whose parent_id refers to a missing or soft-deleted parent. It is the
// shared scan helper used by both the invalid-parent-reference doctor check
// and RepairInvalidParentReferences — both callers call this function to
// avoid duplicating the detection logic.
//
// ListIssues with no state filter returns open, closed, and deferred issues
// (all non-deleted). There is no repository method that filters by "has any
// parent", so this is an O(N) in-memory scan over all issues. For an admin
// repair operation invoked manually this cost is acceptable and avoids adding
// a new driven-port method solely for this scan.
func scanInvalidParentReferences(ctx context.Context, uow driven.UnitOfWork) ([]danglingParentRef, error) {
	items, _, err := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByID, driven.SortAscending, -1)
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	var affected []danglingParentRef
	for _, item := range items {
		if item.ParentID.IsZero() {
			continue
		}

		parent, parentErr := uow.Issues().GetIssue(ctx, item.ParentID, true)
		if parentErr != nil {
			if errors.Is(parentErr, domain.ErrNotFound) {
				// Parent is missing from storage entirely (hard-deleted after GC
				// or never existed). Treat as a dangling reference.
				affected = append(affected, danglingParentRef{
					issueID:  item.ID,
					parentID: item.ParentID,
				})
				continue
			}
			return nil, fmt.Errorf("checking parent %s of issue %s: %w", item.ParentID, item.ID, parentErr)
		}

		if parent.IsDeleted() {
			// Parent exists in storage but is soft-deleted. Both soft-deleted
			// and hard-deleted parents are treated identically as dangling
			// references, per the invalid-parent-reference specification.
			affected = append(affected, danglingParentRef{
				issueID:  item.ID,
				parentID: item.ParentID,
			})
		}
	}
	return affected, nil
}

// repairDanglingParent clears the parent reference on a single issue and
// records an audit comment and history entries. All writes occur within the
// caller's unit of work.
func repairDanglingParent(ctx context.Context, uow driven.UnitOfWork, ref danglingParentRef, author domain.Author, now time.Time) error {
	// Fetch the full issue to perform domain mutations.
	issue, err := uow.Issues().GetIssue(ctx, ref.issueID, false)
	if err != nil {
		return fmt.Errorf("fetching issue %s for repair: %w", ref.issueID, err)
	}

	// Clear parent reference by setting it to the zero (unparented) ID.
	updated := issue.WithParentID(domain.ID{})
	if err := uow.Issues().UpdateIssue(ctx, updated); err != nil {
		return fmt.Errorf("clearing parent on %s: %w", ref.issueID, err)
	}

	// Record a history entry for the parent field change.
	revision, _ := uow.History().CountHistory(ctx, ref.issueID)
	if _, err := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   ref.issueID,
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventUpdated,
		Changes: []history.FieldChange{
			{Field: "parent", Before: ref.parentID.String(), After: domain.ID{}.String()},
		},
	})); err != nil {
		return fmt.Errorf("recording history for %s: %w", ref.issueID, err)
	}

	// Add the spec-mandated audit comment describing the removed parent.
	commentBody := fmt.Sprintf(
		"Removed dangling parent reference %s; parent did not exist in the database. Automated cleanup by 'np admin fix invalid-parent-reference'.",
		ref.parentID.String(),
	)
	comment, err := domain.NewComment(domain.NewCommentParams{
		IssueID:   ref.issueID,
		Author:    author,
		CreatedAt: now,
		Body:      commentBody,
	})
	if err != nil {
		return fmt.Errorf("constructing audit comment for %s: %w", ref.issueID, err)
	}
	commentID, err := uow.Comments().CreateComment(ctx, comment)
	if err != nil {
		return fmt.Errorf("creating audit comment for %s: %w", ref.issueID, err)
	}

	// Record a history entry for the comment.
	revision, _ = uow.History().CountHistory(ctx, ref.issueID)
	if _, err := uow.History().AppendHistory(ctx, history.NewEntry(history.NewEntryParams{
		IssueID:   ref.issueID,
		Revision:  revision,
		Author:    author,
		Timestamp: now,
		EventType: history.EventCommentAdded,
		Changes: []history.FieldChange{
			{Field: "comment_id", After: fmt.Sprintf("%d", commentID)},
			{Field: "body", After: commentBody},
		},
	})); err != nil {
		return fmt.Errorf("recording comment history for %s: %w", ref.issueID, err)
	}

	return nil
}
