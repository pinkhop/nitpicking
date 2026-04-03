package core

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// DeletionResult holds the outcome of a deletion check: either a set of
// issue IDs to delete or a conflict error identifying claimed descendants.
type DeletionResult struct {
	// ToDelete contains the IDs of all issues that should be soft-deleted,
	// including the target issue itself.
	ToDelete []domain.ID

	// Conflicts contains descendants that are currently claimed and prevent
	// the deletion.
	Conflicts []domain.DescendantInfo
}

// PlanEpicDeletion checks whether an epic can be deleted by examining all its
// descendants. If any descendant is currently claimed, the deletion fails with
// a conflict listing the claimed issue(s). Otherwise, it returns the set of
// issue IDs to soft-delete (the epic itself plus all unclaimed descendants).
//
// For tasks, the result contains only the task's own ID (tasks have no
// descendants).
func PlanEpicDeletion(epicID domain.ID, descendants []domain.DescendantInfo) DeletionResult {
	var conflicts []domain.DescendantInfo
	toDelete := []domain.ID{epicID}

	for _, d := range descendants {
		if d.IsClaimed {
			conflicts = append(conflicts, d)
		} else {
			toDelete = append(toDelete, d.ID)
		}
	}

	if len(conflicts) > 0 {
		return DeletionResult{Conflicts: conflicts}
	}

	return DeletionResult{ToDelete: toDelete}
}

// ValidateDeletion checks whether an issue can be deleted. An issue must not
// already be deleted.
func ValidateDeletion(isDeleted bool) error {
	if isDeleted {
		return fmt.Errorf("issue is already deleted: %w", domain.ErrDeletedIssue)
	}
	return nil
}
