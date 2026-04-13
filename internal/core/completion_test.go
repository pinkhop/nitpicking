package core_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestComputeEpicProgress_AllClosed_ReturnsCompleted(t *testing.T) {
	t.Parallel()

	// Given — two children, both closed.
	children := []domain.ChildStatus{
		{State: domain.StateClosed},
		{State: domain.StateClosed},
	}

	// When — computing progress.
	p := core.ComputeEpicProgress(children)

	// Then — 100% complete.
	if p.Total != 2 {
		t.Errorf("total: got %d, want 2", p.Total)
	}
	if p.Closed != 2 {
		t.Errorf("closed: got %d, want 2", p.Closed)
	}
	if !p.Completed {
		t.Error("expected completed when all children closed")
	}
	if p.Percent != 100 {
		t.Errorf("percent: got %d, want 100", p.Percent)
	}
}

func TestComputeEpicProgress_Partial_ReturnsNotCompleted(t *testing.T) {
	t.Parallel()

	// Given — three children: one closed, one open, one open+claimed (secondary).
	// Claimed is a secondary state of open — it counts under Open, not a separate bucket.
	children := []domain.ChildStatus{
		{State: domain.StateClosed},
		{State: domain.StateOpen},
		{State: domain.StateOpen}, // open+claimed secondary state is still StateOpen
	}

	// When — computing progress.
	p := core.ComputeEpicProgress(children)

	// Then — partial progress, not completed.
	if p.Total != 3 {
		t.Errorf("total: got %d, want 3", p.Total)
	}
	if p.Closed != 1 {
		t.Errorf("closed: got %d, want 1", p.Closed)
	}
	if p.Open != 2 {
		t.Errorf("open: got %d, want 2", p.Open)
	}
	if p.Completed {
		t.Error("expected not completed when children are not all closed")
	}
	if p.Percent != 33 {
		t.Errorf("percent: got %d, want 33", p.Percent)
	}
}

func TestComputeEpicProgress_AllStates_CountsEachState(t *testing.T) {
	t.Parallel()

	// Given — five children covering open, deferred, blocked, and closed states.
	// Claimed is now a secondary state of open and counts under Open.
	children := []domain.ChildStatus{
		{State: domain.StateClosed},
		{State: domain.StateOpen}, // open (claimed secondary state also maps here)
		{State: domain.StateOpen},
		{State: domain.StateOpen, IsBlocked: true},
		{State: domain.StateDeferred},
		{State: domain.StateOpen},
	}

	// When — computing progress.
	p := core.ComputeEpicProgress(children)

	// Then — per-state counts are correct; the blocked child is counted
	// as blocked rather than open.
	if p.Total != 6 {
		t.Errorf("total: got %d, want 6", p.Total)
	}
	if p.Closed != 1 {
		t.Errorf("closed: got %d, want 1", p.Closed)
	}
	if p.Open != 3 {
		t.Errorf("open: got %d, want 3", p.Open)
	}
	if p.Blocked != 1 {
		t.Errorf("blocked: got %d, want 1", p.Blocked)
	}
	if p.Deferred != 1 {
		t.Errorf("deferred: got %d, want 1", p.Deferred)
	}
	if p.Percent != 16 {
		t.Errorf("percent: got %d, want 16", p.Percent)
	}
}

func TestComputeEpicProgress_NoChildren_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — no children.
	var children []domain.ChildStatus

	// When — computing progress.
	p := core.ComputeEpicProgress(children)

	// Then — zero progress, not completed.
	if p.Total != 0 {
		t.Errorf("total: got %d, want 0", p.Total)
	}
	if p.Completed {
		t.Error("expected not completed with no children")
	}
}
