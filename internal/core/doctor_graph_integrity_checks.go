package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// runClosedParentWithOpenChild detects closed issues that still have open or
// deferred children. It performs a single in-memory scan: all non-deleted
// issues are listed, those that are open or deferred and have a parent are
// grouped by parent, and any parent whose primary state is closed is recorded
// as a violation. One row is emitted per closed parent; NonClosedChildren is
// sorted ascending by issue ID.
func runClosedParentWithOpenChild(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("closed-parent-with-open-child: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.ClosedParentWithOpenChildRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		items, _, listErr := uow.Issues().ListIssues(ctx, driven.IssueFilter{}, driven.OrderByID, driven.SortAscending, -1)
		if listErr != nil {
			return fmt.Errorf("listing issues: %w", listErr)
		}

		// Build a state lookup: issueID string → State.
		stateByID := make(map[string]domain.State, len(items))
		for _, item := range items {
			stateByID[item.ID.String()] = item.State
		}

		// For each non-closed issue with a parent, record it under its parent
		// if the parent is closed.
		nonClosedByParent := make(map[string][]string)
		for _, item := range items {
			if item.ParentID.IsZero() {
				continue
			}
			if item.State != domain.StateOpen && item.State != domain.StateDeferred {
				continue
			}
			parentStr := item.ParentID.String()
			if stateByID[parentStr] == domain.StateClosed {
				nonClosedByParent[parentStr] = append(nonClosedByParent[parentStr], item.ID.String())
			}
		}

		if len(nonClosedByParent) == 0 {
			return nil
		}

		// Collect parent IDs in sorted order for deterministic output.
		parentIDs := make([]string, 0, len(nonClosedByParent))
		for pid := range nonClosedByParent {
			parentIDs = append(parentIDs, pid)
		}
		sort.Strings(parentIDs)

		for _, pid := range parentIDs {
			children := nonClosedByParent[pid]
			sort.Strings(children)
			rows = append(rows, driving.ClosedParentWithOpenChildRow{
				Issue:             pid,
				NonClosedChildren: children,
			})
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("closed-parent-with-open-child: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d closed issue(s) with non-closed children", len(rows)),
		Affected: affected,
	}, nil
}

// runInvalidParentReference detects issues whose parent_id refers to an absent
// or soft-deleted parent. It delegates detection to scanInvalidParentReferences,
// the same helper used by RepairInvalidParentReferences, so both the doctor
// check and the repair command operate on exactly the same set of violations.
// One row is emitted per child; the Affected array is sorted ascending by issue
// ID.
func runInvalidParentReference(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("invalid-parent-reference: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.InvalidParentReferenceRow

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		refs, scanErr := scanInvalidParentReferences(ctx, uow)
		if scanErr != nil {
			return scanErr
		}
		for _, ref := range refs {
			rows = append(rows, driving.InvalidParentReferenceRow{
				Issue:           ref.issueID.String(),
				MissingParentID: ref.parentID.String(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid-parent-reference: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Sort by issue ID ascending so that output is deterministic regardless of
	// the iteration order produced by the underlying IssueRepository.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Issue < rows[j].Issue
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d issue(s) reference a parent that does not exist", len(rows)),
		Affected: affected,
	}, nil
}
