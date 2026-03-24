package epiccmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/epiccmd"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func TestComputeProgress_AllClosed_Eligible(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{State: issue.StateClosed},
		{State: issue.StateClosed},
	}

	// When
	p := epiccmd.ComputeProgress(children)

	// Then
	if p.Total != 2 {
		t.Errorf("total: got %d, want 2", p.Total)
	}
	if p.Closed != 2 {
		t.Errorf("closed: got %d, want 2", p.Closed)
	}
	if !p.Eligible {
		t.Error("expected eligible when all children closed")
	}
	if p.Percent != 100 {
		t.Errorf("percent: got %d, want 100", p.Percent)
	}
}

func TestComputeProgress_Partial_NotEligible(t *testing.T) {
	t.Parallel()

	// Given
	children := []issue.ChildStatus{
		{State: issue.StateClosed},
		{State: issue.StateOpen},
		{State: issue.StateClaimed},
	}

	// When
	p := epiccmd.ComputeProgress(children)

	// Then
	if p.Total != 3 {
		t.Errorf("total: got %d, want 3", p.Total)
	}
	if p.Closed != 1 {
		t.Errorf("closed: got %d, want 1", p.Closed)
	}
	if p.Eligible {
		t.Error("expected not eligible when children are not all closed")
	}
	if p.Percent != 33 {
		t.Errorf("percent: got %d, want 33", p.Percent)
	}
}

func TestComputeProgress_NoChildren_NotEligible(t *testing.T) {
	t.Parallel()

	// Given
	var children []issue.ChildStatus

	// When
	p := epiccmd.ComputeProgress(children)

	// Then
	if p.Total != 0 {
		t.Errorf("total: got %d, want 0", p.Total)
	}
	if p.Eligible {
		t.Error("expected not eligible with no children")
	}
}
