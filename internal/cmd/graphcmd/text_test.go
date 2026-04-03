package graphcmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/graphcmd"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestRunText_RootTasks_RendersFlat(t *testing.T) {
	t.Parallel()

	// Given — two root-level tasks with no parent.
	svc := setupService(t)
	idA := createTask(t, svc, "Alpha task")
	idB := createTask(t, svc, "Beta task")

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service: svc,
		Format:  graphcmd.FormatText,
		WriteTo: &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then — both tasks appear in output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, idA.String()) {
		t.Errorf("expected issue %s in output", idA)
	}
	if !strings.Contains(output, idB.String()) {
		t.Errorf("expected issue %s in output", idB)
	}
	if !strings.Contains(output, "Alpha task") {
		t.Error("expected 'Alpha task' in output")
	}
}

func TestRunText_EpicWithChildren_RendersTree(t *testing.T) {
	t.Parallel()

	// Given — an epic with two child tasks.
	svc := setupService(t)
	epicOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  "Parent epic",
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	epicID := epicOut.Issue.ID()

	childAOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child A",
		ParentID: epicID.String(),
		Author:   mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create child A failed: %v", err)
	}

	childBOut, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    "Child B",
		ParentID: epicID.String(),
		Author:   mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create child B failed: %v", err)
	}

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service: svc,
		Format:  graphcmd.FormatText,
		WriteTo: &buf,
	}

	// When
	err = graphcmd.Run(t.Context(), input)
	// Then — the tree structure is visible.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()

	// The epic should appear.
	if !strings.Contains(output, epicID.String()) {
		t.Errorf("expected epic %s in output", epicID)
	}

	// Children should appear with tree-drawing characters.
	if !strings.Contains(output, childAOut.Issue.ID().String()) {
		t.Errorf("expected child A %s in output", childAOut.Issue.ID())
	}
	if !strings.Contains(output, childBOut.Issue.ID().String()) {
		t.Errorf("expected child B %s in output", childBOut.Issue.ID())
	}

	// At least one tree connector should be present.
	if !strings.Contains(output, "├──") && !strings.Contains(output, "└──") {
		t.Error("expected tree-drawing characters in output")
	}
}

func TestRunText_Relationships_RenderedBeneathNode(t *testing.T) {
	t.Parallel()

	// Given — two tasks with a blocked_by relationship.
	svc := setupService(t)
	blockerID := createTask(t, svc, "Blocker")
	blockedID := createTask(t, svc, "Blocked")

	err := svc.AddRelationship(t.Context(), blockedID.String(), driving.RelationshipInput{
		TargetID: blockerID.String(),
		Type:     domain.RelBlockedBy,
	}, mustAuthor(t, "test-agent"))
	if err != nil {
		t.Fatalf("precondition: add relationship failed: %v", err)
	}

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service: svc,
		Format:  graphcmd.FormatText,
		WriteTo: &buf,
	}

	// When
	err = graphcmd.Run(t.Context(), input)
	// Then — the blocked_by relationship appears in output.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "blocked_by") {
		t.Error("expected 'blocked_by' relationship in output")
	}
	if !strings.Contains(output, blockerID.String()) {
		t.Errorf("expected blocker ID %s in relationship line", blockerID)
	}
}

func TestRunText_ExcludesClosedByDefault(t *testing.T) {
	t.Parallel()

	// Given — one open task and one closed task.
	svc := setupService(t)
	_ = createTask(t, svc, "Open task")
	closedID := createTask(t, svc, "Closed task")
	claimAndClose(t, svc, closedID)

	var buf bytes.Buffer
	input := graphcmd.RunInput{
		Service:       svc,
		Format:        graphcmd.FormatText,
		IncludeClosed: false,
		WriteTo:       &buf,
	}

	// When
	err := graphcmd.Run(t.Context(), input)
	// Then — closed issue is excluded.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if strings.Contains(output, closedID.String()) {
		t.Errorf("closed issue %s should be excluded", closedID)
	}
	if !strings.Contains(output, "Open task") {
		t.Error("expected 'Open task' in output")
	}
}
