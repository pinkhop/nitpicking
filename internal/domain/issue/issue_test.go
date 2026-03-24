package issue_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func mustID(t *testing.T) issue.ID {
	t.Helper()
	id, err := issue.GenerateID("NP", nil)
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
	tk, err := issue.NewTask(issue.NewTaskParams{
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
	if tk.Role() != issue.RoleTask {
		t.Errorf("expected task role, got %s", tk.Role())
	}
	if tk.Title() != "Fix login bug" {
		t.Errorf("expected title, got %q", tk.Title())
	}
	if tk.State() != issue.StateOpen {
		t.Errorf("expected open state, got %s", tk.State())
	}
	if tk.Priority() != issue.P2 {
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
	tk, err := issue.NewEpic(issue.NewEpicParams{
		ID:       id,
		Title:    "Auth overhaul",
		Priority: issue.P1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Role() != issue.RoleEpic {
		t.Errorf("expected epic role, got %s", tk.Role())
	}
	if tk.State() != issue.StateOpen {
		t.Errorf("expected open state, got %s", tk.State())
	}
	if tk.Priority() != issue.P1 {
		t.Errorf("expected P1, got %s", tk.Priority())
	}
	if !tk.IsEpic() {
		t.Error("expected IsEpic true")
	}
}

func TestNewTask_EmptyTitle_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := issue.NewTask(issue.NewTaskParams{
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
	_, err := issue.NewTask(issue.NewTaskParams{
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
	_, err := issue.NewTask(issue.NewTaskParams{
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
	tk, err := issue.NewTask(issue.NewTaskParams{
		ID:       id,
		Title:    "Critical security fix",
		Priority: issue.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != issue.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestNewEpic_ExplicitP0_PreservesP0(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	tk, err := issue.NewEpic(issue.NewEpicParams{
		ID:       id,
		Title:    "Critical epic",
		Priority: issue.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != issue.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestIssue_WithTitle_ReturnsNewIssue(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := issue.NewTask(issue.NewTaskParams{
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
	original, _ := issue.NewTask(issue.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	updated := original.WithPriority(issue.P0)

	// Then
	if updated.Priority() != issue.P0 {
		t.Errorf("expected P0, got %s", updated.Priority())
	}
	if original.Priority() != issue.P2 {
		t.Error("expected original unchanged")
	}
}

func TestIssue_WithDeleted_MarksAsDeleted(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := issue.NewTask(issue.NewTaskParams{
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
	tk, _ := issue.NewTask(issue.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	claimed := tk.WithState(issue.StateClaimed)

	// Then
	if claimed.State() != issue.StateClaimed {
		t.Errorf("expected claimed, got %s", claimed.State())
	}
	if tk.State() != issue.StateOpen {
		t.Error("expected original unchanged")
	}
}

func TestIssue_WithParentID_SetsAndClears(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := issue.NewTask(issue.NewTaskParams{
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
	var zeroID issue.ID
	withoutParent := withParent.WithParentID(zeroID)

	// Then
	if !withoutParent.ParentID().IsZero() {
		t.Error("expected zero parent ID after clearing")
	}
}
