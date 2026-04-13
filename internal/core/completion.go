package core

import "github.com/pinkhop/nitpicking/internal/domain"

// EpicProgress holds the computed completion metrics for an epic.
type EpicProgress struct {
	// Total is the number of direct children.
	Total int
	// Closed is the number of children in the closed state.
	Closed int
	// Open is the number of non-blocked children in the open state.
	// Includes children with an active claim (open + claimed secondary state).
	Open int
	// Blocked is the number of children that are blocked (any primary state
	// with an unresolved blocked_by relationship).
	Blocked int
	// Deferred is the number of non-blocked children in the deferred state.
	Deferred int
	// Percent is the completion percentage (0–100).
	Percent int
	// Completed is true when the epic has at least one child and all
	// children are closed.
	Completed bool
}

// ComputeEpicProgress derives completion metrics from a list of child statuses.
// An epic is completed when it has at least one child and all children are
// closed. Returns a zero-value EpicProgress when the child list is empty.
//
// Blocked children are counted separately regardless of their primary state.
// Claimed is now a secondary state of open, so open children with an active
// claim count under Open rather than a separate bucket.
func ComputeEpicProgress(children []domain.ChildStatus) EpicProgress {
	total := len(children)
	if total == 0 {
		return EpicProgress{}
	}

	var closed, open, blocked, deferred int
	for _, c := range children {
		if c.IsBlocked && c.State != domain.StateClosed {
			blocked++
			continue
		}
		switch c.State {
		case domain.StateClosed:
			closed++
		case domain.StateDeferred:
			deferred++
		default:
			// StateOpen (including open+claimed secondary state).
			open++
		}
	}

	return EpicProgress{
		Total:     total,
		Closed:    closed,
		Open:      open,
		Blocked:   blocked,
		Deferred:  deferred,
		Percent:   closed * 100 / total,
		Completed: closed == total,
	}
}
