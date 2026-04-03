package core_test

import (
	"errors"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

func TestPropagateLabel_SetsLabelOnDescendants(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label and two child tasks without the label.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")

	epicID := createEpicWithLabel(t, svc, "Parent epic", "env", "prod", author)
	childA := createChildTask(t, svc, "Child A", epicID, author)
	childB := createChildTask(t, svc, "Child B", epicID, author)

	// When — propagating the "env" label.
	out, err := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "env",
		Author:  author,
	})
	// Then — both children receive the label.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Propagated != 2 {
		t.Errorf("propagated: got %d, want 2", out.Propagated)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
	if out.Value != "prod" {
		t.Errorf("value: got %q, want %q", out.Value, "prod")
	}

	// Verify children have the label.
	for _, childID := range []domain.ID{childA, childB} {
		shown, showErr := svc.ShowIssue(t.Context(), childID.String())
		if showErr != nil {
			t.Fatalf("show %s: %v", childID, showErr)
		}
		val, ok := shown.Labels["env"]
		if !ok || val != "prod" {
			t.Errorf("child %s: expected env=prod, got ok=%v val=%q", childID, ok, val)
		}
	}
}

func TestPropagateLabel_SkipsDescendantsWithMatchingLabel(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label and one child that already has it.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")

	epicID := createEpicWithLabel(t, svc, "Parent", "team", "backend", author)
	childA := createChildTask(t, svc, "Already labeled", epicID, author)

	// Set the label on child A before propagation.
	err := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  childA.String(),
		Author:   author,
		LabelSet: []driving.LabelInput{{Key: "team", Value: "backend"}},
	})
	if err != nil {
		t.Fatalf("precondition: set label: %v", err)
	}

	_ = createChildTask(t, svc, "Not labeled", epicID, author)

	// When — propagating.
	out, propErr := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "team",
		Author:  author,
	})
	// Then — only one descendant was updated (the one without the label).
	if propErr != nil {
		t.Fatalf("unexpected error: %v", propErr)
	}
	if out.Propagated != 1 {
		t.Errorf("propagated: got %d, want 1", out.Propagated)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}
}

func TestPropagateLabel_MissingKey_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — an epic without the requested label.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpicWithLabel(t, svc, "Epic", "foo", "bar", author)

	// When — propagating a label that doesn't exist on the parent.
	_, err := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "nonexistent",
		Author:  author,
	})
	// Then — error returned.
	if err == nil {
		t.Fatal("expected error for missing label")
	}
}

func TestPropagateLabel_NoDescendants_ReturnsZero(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label but no children.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")
	epicID := createEpicWithLabel(t, svc, "Empty epic", "env", "staging", author)

	// When — propagating.
	out, err := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "env",
		Author:  author,
	})
	// Then — zero propagated, zero total.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Propagated != 0 {
		t.Errorf("propagated: got %d, want 0", out.Propagated)
	}
	if out.Total != 0 {
		t.Errorf("total: got %d, want 0", out.Total)
	}
}

func TestPropagateLabel_DescendantClaimedBySameAuthor_Succeeds(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label and one child claimed by the same author.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")

	epicID := createEpicWithLabel(t, svc, "Parent", "env", "prod", author)
	childA := createChildTask(t, svc, "Claimed child", epicID, author)
	childB := createChildTask(t, svc, "Unclaimed child", epicID, author)

	// Claim child A with the same author that will perform propagation.
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: childA.String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim child A: %v", err)
	}

	// When — propagating the "env" label.
	out, propErr := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "env",
		Author:  author,
	})

	// Then — both children receive the label, including the claimed one.
	if propErr != nil {
		t.Fatalf("unexpected error: %v", propErr)
	}
	if out.Propagated != 2 {
		t.Errorf("propagated: got %d, want 2", out.Propagated)
	}
	if out.Total != 2 {
		t.Errorf("total: got %d, want 2", out.Total)
	}

	for _, childID := range []domain.ID{childA, childB} {
		shown, showErr := svc.ShowIssue(t.Context(), childID.String())
		if showErr != nil {
			t.Fatalf("show %s: %v", childID, showErr)
		}
		val, ok := shown.Labels["env"]
		if !ok || val != "prod" {
			t.Errorf("child %s: expected env=prod, got ok=%v val=%q", childID, ok, val)
		}
	}
}

func TestPropagateLabel_DescendantClaimedByDifferentAuthor_FailsAtomically(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label, two children: one unclaimed, one claimed
	// by a different author.
	svc, _ := setupService(t)
	propagator := mustAuthor(t, "propagator")
	otherAgent := mustAuthor(t, "other-agent")

	epicID := createEpicWithLabel(t, svc, "Parent", "env", "staging", propagator)
	childA := createChildTask(t, svc, "Unclaimed child", epicID, propagator)
	childB := createChildTask(t, svc, "Other-claimed child", epicID, otherAgent)

	// Claim child B with a different author.
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: childB.String(),
		Author:  otherAgent,
	})
	if err != nil {
		t.Fatalf("precondition: claim child B: %v", err)
	}

	// When — propagating the "env" label.
	_, propErr := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "env",
		Author:  propagator,
	})

	// Then — error is a ClaimConflictError.
	if propErr == nil {
		t.Fatal("expected ClaimConflictError, got nil")
	}
	var conflict *domain.ClaimConflictError
	if !errors.As(propErr, &conflict) {
		t.Fatalf("expected *ClaimConflictError, got %T: %v", propErr, propErr)
	}

	// Then — no labels were changed on the unclaimed child (atomicity).
	shown, showErr := svc.ShowIssue(t.Context(), childA.String())
	if showErr != nil {
		t.Fatalf("show child A: %v", showErr)
	}
	if _, hasLabel := shown.Labels["env"]; hasLabel {
		t.Error("child A should not have label env — propagation should have been atomic")
	}
}

func TestPropagateLabel_DescendantClaimedBySameAuthor_LabelAlreadySet_SkipsIt(t *testing.T) {
	t.Parallel()

	// Given — an epic with a label, one child claimed by the same author that
	// already has the matching label.
	svc, _ := setupService(t)
	author := mustAuthor(t, "test-agent")

	epicID := createEpicWithLabel(t, svc, "Parent", "env", "prod", author)
	childA := createChildTask(t, svc, "Already labeled", epicID, author)

	// Set the label on child A before claiming.
	setErr := svc.OneShotUpdate(t.Context(), driving.OneShotUpdateInput{
		IssueID:  childA.String(),
		Author:   author,
		LabelSet: []driving.LabelInput{{Key: "env", Value: "prod"}},
	})
	if setErr != nil {
		t.Fatalf("precondition: set label: %v", setErr)
	}

	// Now claim child A.
	_, err := svc.ClaimByID(t.Context(), driving.ClaimInput{
		IssueID: childA.String(),
		Author:  author,
	})
	if err != nil {
		t.Fatalf("precondition: claim child A: %v", err)
	}

	// When — propagating.
	out, propErr := svc.PropagateLabel(t.Context(), driving.PropagateLabelInput{
		IssueID: epicID.String(),
		Key:     "env",
		Author:  author,
	})

	// Then — zero propagated because the label already matched.
	if propErr != nil {
		t.Fatalf("unexpected error: %v", propErr)
	}
	if out.Propagated != 0 {
		t.Errorf("propagated: got %d, want 0", out.Propagated)
	}
}

// createEpicWithLabel creates an epic and sets a label on it.
func createEpicWithLabel(t *testing.T, svc driving.Service, title, key, value string, author string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: author,
		Labels: []driving.LabelInput{{Key: key, Value: value}},
	})
	if err != nil {
		t.Fatalf("create epic: %v", err)
	}
	return out.Issue.ID()
}

// createChildTask creates a task under the given parent.
func createChildTask(t *testing.T, svc driving.Service, title string, parentID domain.ID, author string) domain.ID {
	t.Helper()

	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:     domain.RoleTask,
		Title:    title,
		ParentID: parentID.String(),
		Author:   author,
	})
	if err != nil {
		t.Fatalf("create child task: %v", err)
	}
	return out.Issue.ID()
}
