package core

import (
	"context"
	"fmt"
	"sort"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// runClosableParentIssues detects open issues (epics or tasks) that have at
// least one child and whose children are all in state closed. Each such issue
// can be closed via `np epic close-completed [--include-tasks]` to acknowledge
// that all its work is done. result.Meta is set to a bool indicating whether
// any closable parent is in the task role (used by the registry FixFn to
// append --include-tasks to the fix command).
func runClosableParentIssues(ctx context.Context, svc *serviceImpl, _ driving.DoctorInput) (*doctorRunResult, error) {
	if svc.tx == nil {
		return nil, fmt.Errorf("closable-parent-issues: database connection unavailable (cascade should have protected this check)")
	}

	var rows []driving.ClosableParentIssueRow
	var anyTaskParent bool

	err := svc.tx.WithReadTransaction(ctx, func(uow driven.UnitOfWork) error {
		issues, byID, listErr := loadGraphIssues(ctx, uow)
		if listErr != nil {
			return listErr
		}

		closable := computeClosableSet(issues, byID)

		for parentID := range closable {
			rows = append(rows, driving.ClosableParentIssueRow{Issue: parentID})
			if parent, ok := byID[parentID]; ok && parent.Role == domain.RoleTask {
				anyTaskParent = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("closable-parent-issues: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Issue < rows[j].Issue
	})

	affected := make([]any, len(rows))
	for i, r := range rows {
		affected[i] = r
	}
	return &doctorRunResult{
		Summary:  fmt.Sprintf("%d issue(s) eligible for close-completed", len(rows)),
		Affected: affected,
		Meta:     anyTaskParent,
	}, nil
}
