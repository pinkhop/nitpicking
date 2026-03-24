package issue

import (
	"fmt"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// ValidateParent checks parent assignment constraints:
//   - An issue cannot be its own parent.
//   - A deleted issue cannot be assigned as a parent.
//
// Any issue role (task or epic) may be a parent of any other issue role.
// Cycle detection is handled separately by ValidateNoCycle.
func ValidateParent(childID, parentID ID, parentDeleted bool) error {
	if childID == parentID {
		return fmt.Errorf("issue cannot be its own parent: %w", domain.ErrCycleDetected)
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
