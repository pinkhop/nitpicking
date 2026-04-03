package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func mustID(t *testing.T) domain.ID {
	t.Helper()
	id, err := domain.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate ID: %v", err)
	}
	return id
}

func TestNewTask_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)
	now := time.Now()

	// When
	tk, err := domain.NewTask(domain.NewTaskParams{
		ID:        id,
		Title:     "Fix login bug",
		CreatedAt: now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.ID() != id {
		t.Errorf("expected ID %s, got %s", id, tk.ID())
	}
	if tk.Role() != domain.RoleTask {
		t.Errorf("expected task role, got %s", tk.Role())
	}
	if tk.Title() != "Fix login bug" {
		t.Errorf("expected title, got %q", tk.Title())
	}
	if tk.State() != domain.StateOpen {
		t.Errorf("expected open state, got %s", tk.State())
	}
	if tk.Priority() != domain.P2 {
		t.Errorf("expected default P2, got %s", tk.Priority())
	}
	if !tk.IsTask() {
		t.Error("expected IsTask true")
	}
	if tk.IsEpic() {
		t.Error("expected IsEpic false")
	}
}

func TestNewEpic_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	tk, err := domain.NewEpic(domain.NewEpicParams{
		ID:       id,
		Title:    "Auth overhaul",
		Priority: domain.P1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Role() != domain.RoleEpic {
		t.Errorf("expected epic role, got %s", tk.Role())
	}
	if tk.State() != domain.StateOpen {
		t.Errorf("expected open state, got %s", tk.State())
	}
	if tk.Priority() != domain.P1 {
		t.Errorf("expected P1, got %s", tk.Priority())
	}
	if !tk.IsEpic() {
		t.Error("expected IsEpic true")
	}
}

func TestNewTask_EmptyTitle_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for empty title")
	}
	if !errors.Is(err, &domain.ValidationError{}) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestNewTask_NonAlphanumericTitle_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "---",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for non-alphanumeric title")
	}
}

func TestNewTask_ZeroID_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewTask(domain.NewTaskParams{
		Title: "Valid title",
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero ID")
	}
}

func TestNewTask_ExplicitP0_PreservesP0(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	tk, err := domain.NewTask(domain.NewTaskParams{
		ID:       id,
		Title:    "Critical security fix",
		Priority: domain.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != domain.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestNewEpic_ExplicitP0_PreservesP0(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	tk, err := domain.NewEpic(domain.NewEpicParams{
		ID:       id,
		Title:    "Critical epic",
		Priority: domain.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != domain.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestIssue_WithTitle_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Original",
	})

	// When
	updated, err := original.WithTitle("Updated")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Title() != "Updated" {
		t.Errorf("expected Updated, got %s", updated.Title())
	}
	if original.Title() != "Original" {
		t.Error("expected original to be unchanged")
	}
}

func TestIssue_WithPriority_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	updated := original.WithPriority(domain.P0)

	// Then
	if updated.Priority() != domain.P0 {
		t.Errorf("expected P0, got %s", updated.Priority())
	}
	if original.Priority() != domain.P2 {
		t.Error("expected original unchanged")
	}
}

func TestIssue_WithDeleted_MarksAsDeleted(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	deleted := tk.WithDeleted()

	// Then
	if !deleted.IsDeleted() {
		t.Error("expected deleted")
	}
	if tk.IsDeleted() {
		t.Error("expected original not deleted")
	}
}

func TestIssue_WithState_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	claimed := tk.WithState(domain.StateClaimed)

	// Then
	if claimed.State() != domain.StateClaimed {
		t.Errorf("expected claimed, got %s", claimed.State())
	}
	if tk.State() != domain.StateOpen {
		t.Error("expected original unchanged")
	}
}

func TestIssue_WithParentID_SetsAndClears(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	parentID := mustID(t)

	// When — set parent
	withParent := tk.WithParentID(parentID)

	// Then
	if withParent.ParentID() != parentID {
		t.Errorf("expected parent %s, got %s", parentID, withParent.ParentID())
	}

	// When — clear parent
	var zeroID domain.ID
	withoutParent := withParent.WithParentID(zeroID)

	// Then
	if !withoutParent.ParentID().IsZero() {
		t.Error("expected zero parent ID after clearing")
	}
}

func TestIssue_WithDescription_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	updated := original.WithDescription("new description")

	// Then
	if updated.Description() != "new description" {
		t.Errorf("expected %q, got %q", "new description", updated.Description())
	}
	if original.Description() != "" {
		t.Errorf("expected original unchanged, got %q", original.Description())
	}
}

func TestIssue_WithDescription_ChainingWorks(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	chained := original.WithDescription("first").WithDescription("second")

	// Then
	if chained.Description() != "second" {
		t.Errorf("expected %q, got %q", "second", chained.Description())
	}
	if original.Description() != "" {
		t.Errorf("expected original unchanged, got %q", original.Description())
	}
}

func TestIssue_WithAcceptanceCriteria_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	updated := original.WithAcceptanceCriteria("must pass all tests")

	// Then
	if updated.AcceptanceCriteria() != "must pass all tests" {
		t.Errorf("expected %q, got %q", "must pass all tests", updated.AcceptanceCriteria())
	}
	if original.AcceptanceCriteria() != "" {
		t.Errorf("expected original unchanged, got %q", original.AcceptanceCriteria())
	}
}

func TestIssue_WithAcceptanceCriteria_ChainingWorks(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	chained := original.WithAcceptanceCriteria("v1").WithAcceptanceCriteria("v2")

	// Then
	if chained.AcceptanceCriteria() != "v2" {
		t.Errorf("expected %q, got %q", "v2", chained.AcceptanceCriteria())
	}
	if original.AcceptanceCriteria() != "" {
		t.Errorf("expected original unchanged, got %q", original.AcceptanceCriteria())
	}
}

func TestIssue_WithLabels_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	lbl, err := domain.NewLabel("kind", "bug")
	if err != nil {
		t.Fatalf("precondition: label creation failed: %v", err)
	}
	labels := domain.NewLabelSet().Set(lbl)

	// When
	updated := original.WithLabels(labels)

	// Then
	got, ok := updated.Labels().Get("kind")
	if !ok || got != "bug" {
		t.Errorf("expected label kind=bug, got %q (ok=%v)", got, ok)
	}
	if original.Labels().Len() != 0 {
		t.Errorf("expected original to have no labels, got %d", original.Labels().Len())
	}
}

func TestIssue_WithLabels_OriginalMapUnchanged(t *testing.T) {
	t.Parallel()

	// Given — start with a label already set
	lbl1, err := domain.NewLabel("team", "backend")
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	original = original.WithLabels(domain.NewLabelSet().Set(lbl1))

	lbl2, err := domain.NewLabel("kind", "fix")
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	newLabels := original.Labels().Set(lbl2)

	// When
	updated := original.WithLabels(newLabels)

	// Then — updated has both labels
	if updated.Labels().Len() != 2 {
		t.Errorf("expected 2 labels on updated, got %d", updated.Labels().Len())
	}
	// original still has only one label
	if original.Labels().Len() != 1 {
		t.Errorf("expected 1 label on original, got %d", original.Labels().Len())
	}
}

func TestIssue_WithLabels_ChainingWorks(t *testing.T) {
	t.Parallel()

	// Given
	original, err := domain.NewTask(domain.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	lbl1, err := domain.NewLabel("kind", "bug")
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}
	lbl2, err := domain.NewLabel("team", "infra")
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When
	chained := original.
		WithLabels(domain.NewLabelSet().Set(lbl1)).
		WithLabels(domain.NewLabelSet().Set(lbl1).Set(lbl2))

	// Then
	if chained.Labels().Len() != 2 {
		t.Errorf("expected 2 labels, got %d", chained.Labels().Len())
	}
	if original.Labels().Len() != 0 {
		t.Errorf("expected original unchanged with 0 labels, got %d", original.Labels().Len())
	}
}

func TestNewTask_AllFieldAccessors_ReturnExpectedValues(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	lbl, err := domain.NewLabel("kind", "bug")
	if err != nil {
		t.Fatalf("precondition: label creation failed: %v", err)
	}
	labels := domain.NewLabelSet().Set(lbl)

	// When
	tk, err := domain.NewTask(domain.NewTaskParams{
		ID:                 id,
		Title:              "Fix login timeout",
		Description:        "Users report timeouts after 30s",
		AcceptanceCriteria: "Login completes within 5s",
		Priority:           domain.P1,
		Labels:             labels,
		CreatedAt:          now,
		IdempotencyKey:     "idem-key-123",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Description() != "Users report timeouts after 30s" {
		t.Errorf("Description: expected %q, got %q", "Users report timeouts after 30s", tk.Description())
	}
	if tk.AcceptanceCriteria() != "Login completes within 5s" {
		t.Errorf("AcceptanceCriteria: expected %q, got %q", "Login completes within 5s", tk.AcceptanceCriteria())
	}
	got, ok := tk.Labels().Get("kind")
	if !ok || got != "bug" {
		t.Errorf("Labels: expected kind=bug, got %q (ok=%v)", got, ok)
	}
	if !tk.CreatedAt().Equal(now) {
		t.Errorf("CreatedAt: expected %v, got %v", now, tk.CreatedAt())
	}
	if tk.IdempotencyKey() != "idem-key-123" {
		t.Errorf("IdempotencyKey: expected %q, got %q", "idem-key-123", tk.IdempotencyKey())
	}
}
