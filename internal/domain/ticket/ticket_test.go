package ticket_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

func mustID(t *testing.T) ticket.ID {
	t.Helper()
	id, err := ticket.GenerateID("NP", nil)
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
	tk, err := ticket.NewTask(ticket.NewTaskParams{
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
	if tk.Role() != ticket.RoleTask {
		t.Errorf("expected task role, got %s", tk.Role())
	}
	if tk.Title() != "Fix login bug" {
		t.Errorf("expected title, got %q", tk.Title())
	}
	if tk.State() != ticket.StateOpen {
		t.Errorf("expected open state, got %s", tk.State())
	}
	if tk.Priority() != ticket.P2 {
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
	tk, err := ticket.NewEpic(ticket.NewEpicParams{
		ID:       id,
		Title:    "Auth overhaul",
		Priority: ticket.P1,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Role() != ticket.RoleEpic {
		t.Errorf("expected epic role, got %s", tk.Role())
	}
	if tk.State() != ticket.StateActive {
		t.Errorf("expected active state, got %s", tk.State())
	}
	if tk.Priority() != ticket.P1 {
		t.Errorf("expected P1, got %s", tk.Priority())
	}
	if !tk.IsEpic() {
		t.Error("expected IsEpic true")
	}
}

func TestNewTask_EmptyTitle_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := ticket.NewTask(ticket.NewTaskParams{
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
	_, err := ticket.NewTask(ticket.NewTaskParams{
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
	_, err := ticket.NewTask(ticket.NewTaskParams{
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
	tk, err := ticket.NewTask(ticket.NewTaskParams{
		ID:       id,
		Title:    "Critical security fix",
		Priority: ticket.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != ticket.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestNewEpic_ExplicitP0_PreservesP0(t *testing.T) {
	t.Parallel()

	// Given
	id := mustID(t)

	// When
	tk, err := ticket.NewEpic(ticket.NewEpicParams{
		ID:       id,
		Title:    "Critical epic",
		Priority: ticket.P0,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.Priority() != ticket.P0 {
		t.Errorf("expected P0, got %s", tk.Priority())
	}
}

func TestTicket_WithTitle_ReturnsNewTicket(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := ticket.NewTask(ticket.NewTaskParams{
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

func TestTicket_WithPriority_ReturnsNewTicket(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := ticket.NewTask(ticket.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	updated := original.WithPriority(ticket.P0)

	// Then
	if updated.Priority() != ticket.P0 {
		t.Errorf("expected P0, got %s", updated.Priority())
	}
	if original.Priority() != ticket.P2 {
		t.Error("expected original unchanged")
	}
}

func TestTicket_WithDeleted_MarksAsDeleted(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := ticket.NewTask(ticket.NewTaskParams{
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

func TestTicket_WithState_ReturnsNewTicket(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := ticket.NewTask(ticket.NewTaskParams{
		ID:    mustID(t),
		Title: "Task",
	})

	// When
	claimed := tk.WithState(ticket.StateClaimed)

	// Then
	if claimed.State() != ticket.StateClaimed {
		t.Errorf("expected claimed, got %s", claimed.State())
	}
	if tk.State() != ticket.StateOpen {
		t.Error("expected original unchanged")
	}
}

func TestTicket_WithParentID_SetsAndClears(t *testing.T) {
	t.Parallel()

	// Given
	tk, _ := ticket.NewTask(ticket.NewTaskParams{
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
	var zeroID ticket.ID
	withoutParent := withParent.WithParentID(zeroID)

	// Then
	if !withoutParent.ParentID().IsZero() {
		t.Error("expected zero parent ID after clearing")
	}
}
