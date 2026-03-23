package ticket

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// DescendantInfo describes a descendant ticket for recursive deletion checks.
type DescendantInfo struct {
	// ID is the descendant's ticket ID.
	ID ID
	// IsClaimed is true if the descendant is currently claimed.
	IsClaimed bool
	// ClaimedBy is the author of the active claim, if any.
	ClaimedBy string
}

// DeletionResult holds the outcome of a deletion check: either a set of
// ticket IDs to delete or a conflict error identifying claimed descendants.
type DeletionResult struct {
	// ToDelete contains the IDs of all tickets that should be soft-deleted,
	// including the target ticket itself.
	ToDelete []ID

	// Conflicts contains descendants that are currently claimed and prevent
	// the deletion.
	Conflicts []DescendantInfo
}

// PlanEpicDeletion checks whether an epic can be deleted by examining all its
// descendants. If any descendant is currently claimed, the deletion fails with
// a conflict listing the claimed ticket(s). Otherwise, it returns the set of
// ticket IDs to soft-delete (the epic itself plus all unclaimed descendants).
//
// For tasks, the result contains only the task's own ID (tasks have no
// descendants).
func PlanEpicDeletion(epicID ID, descendants []DescendantInfo) DeletionResult {
	var conflicts []DescendantInfo
	toDelete := []ID{epicID}

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

// ValidateDeletion checks whether a ticket can be deleted. A ticket must be
// claimed (and the caller must hold the claim) and must not already be deleted.
func ValidateDeletion(isDeleted bool) error {
	if isDeleted {
		return fmt.Errorf("ticket is already deleted: %w", domain.ErrDeletedTicket)
	}
	return nil
}
