package issue

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// ValidateParent checks parent assignment constraints from §4.1.1:
//   - Only an epic can be a parent.
//   - An issue cannot be its own parent.
//   - A deleted issue cannot be assigned as a parent.
//
// The parentRole and parentDeleted parameters describe the proposed parent
// issue. Cycle detection is handled separately by ValidateNoCycle.
func ValidateParent(childID, parentID ID, parentRole Role, parentDeleted bool) error {
	if childID == parentID {
		return fmt.Errorf("issue cannot be its own parent: %w", domain.ErrCycleDetected)
	}
	if parentRole != RoleEpic {
		return fmt.Errorf("only epics can be parents, got %s: %w", parentRole, domain.ErrIllegalTransition)
	}
	if parentDeleted {
		return fmt.Errorf("cannot assign deleted issue as parent: %w", domain.ErrDeletedIssue)
	}
	return nil
}

// AncestorLookup is a callback that returns the parent ID of a given issue.
// It returns a zero ID if the issue has no parent. An error indicates a
// lookup failure.
type AncestorLookup func(id ID) (parentID ID, err error)

// ValidateNoCycle walks the ancestor chain of proposedParent to ensure that
// assigning it as the parent of childID would not create a cycle. A cycle
// exists if childID appears as an ancestor of proposedParent.
func ValidateNoCycle(childID, proposedParent ID, lookup AncestorLookup) error {
	current := proposedParent
	visited := make(map[ID]bool)

	for !current.IsZero() {
		if current == childID {
			return fmt.Errorf("assigning %s as parent of %s would create a cycle: %w",
				proposedParent, childID, domain.ErrCycleDetected)
		}
		if visited[current] {
			// Cycle in existing data — don't loop forever, just stop.
			return nil
		}
		visited[current] = true

		parentID, err := lookup(current)
		if err != nil {
			return fmt.Errorf("looking up ancestor of %s: %w", current, err)
		}
		current = parentID
	}

	return nil
}
