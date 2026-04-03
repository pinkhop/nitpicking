package issuecmd_test

import (
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestOrphanFilter_ReturnsOnlyIssuesWithoutParent(t *testing.T) {
	t.Parallel()

	// Given: two tasks — one orphan (no parent), one child of an epic.
	svc := setupService(t)
	ctx := t.Context()
	author := mustAuthor(t, "test-agent")

	epicOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Parent epic",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}

	_, err = svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child task",
		Author:   author,
		ParentID: epicOut.Issue.ID().String(),
	})
	if err != nil {
		t.Fatalf("precondition: create child task failed: %v", err)
	}

	orphanOut, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  "Orphan task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create orphan task failed: %v", err)
	}

	// When: list with orphan filter.
	result, err := svc.ListIssues(ctx, driving.ListIssuesInput{
		Filter: driving.IssueFilterInput{Orphan: true},
	})
	// Then: should return the epic (no parent) and the orphan task, but not the child.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundOrphan := false
	foundEpic := false
	foundChild := false
	for _, item := range result.Items {
		switch item.ID {
		case orphanOut.Issue.ID().String():
			foundOrphan = true
		case epicOut.Issue.ID().String():
			foundEpic = true
		default:
			foundChild = true
		}
	}

	if !foundOrphan {
		t.Error("orphan task should be in results")
	}
	if !foundEpic {
		t.Error("epic (which has no parent) should be in results")
	}
	if foundChild {
		t.Error("child task should NOT be in results")
	}
}
