package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
	"github.com/pinkhop/nitpicking/internal/fake"
)

func setupService(t *testing.T) (service.Service, *fake.Repository) {
	t.Helper()
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	ctx := context.Background()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	return svc, repo
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

// --- Init ---

func TestInit_ValidPrefix_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	// When
	err := svc.Init(context.Background(), "NP")
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInit_InvalidPrefix_Fails(t *testing.T) {
	t.Parallel()

	// Given
	repo := fake.NewRepository()
	tx := fake.NewTransactor(repo)
	svc := service.New(tx)

	// When
	err := svc.Init(context.Background(), "np")

	// Then
	if err == nil {
		t.Fatal("expected error for lowercase prefix")
	}
}

// --- AgentName ---

func TestAgentName_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	name, err := svc.AgentName(context.Background())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name == "" {
		t.Error("expected non-empty name")
	}
}

// --- CreateTicket ---

func TestCreateTicket_Task_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// When
	output, err := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Fix login bug",
		Author: author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Ticket.ID().IsZero() {
		t.Error("expected non-zero ticket ID")
	}
	if output.Ticket.Title() != "Fix login bug" {
		t.Errorf("expected title, got %q", output.Ticket.Title())
	}
	if output.Ticket.State() != ticket.StateOpen {
		t.Errorf("expected open state, got %s", output.Ticket.State())
	}
}

func TestCreateTicket_WithClaim_ReturnsClaimID(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	output, err := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: mustAuthor(t, "alice"),
		Claim:  true,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ClaimID == "" {
		t.Error("expected non-empty claim ID when created with claim")
	}
}

func TestCreateTicket_IdempotencyKey_ReturnsSameTicket(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	input := service.CreateTicketInput{
		Role:           ticket.RoleTask,
		Title:          "Idempotent task",
		Author:         author,
		IdempotencyKey: "idem-1",
	}

	// When — create twice with same key
	out1, err1 := svc.CreateTicket(context.Background(), input)
	out2, err2 := svc.CreateTicket(context.Background(), input)

	// Then
	if err1 != nil {
		t.Fatalf("first create failed: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("second create failed: %v", err2)
	}
	if out1.Ticket.ID() != out2.Ticket.ID() {
		t.Errorf("expected same ticket ID, got %s and %s", out1.Ticket.ID(), out2.Ticket.ID())
	}
}

func TestCreateTicket_InvalidTitle_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)

	// When
	_, err := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "---",
		Author: mustAuthor(t, "alice"),
	})

	// Then
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// --- ClaimByID ---

func TestClaimByID_UnclaimedTicket_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ClaimByID(context.Background(), service.ClaimInput{
		TicketID: created.Ticket.ID(),
		Author:   author,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.ClaimID == "" {
		t.Error("expected non-empty claim ID")
	}
}

func TestClaimByID_AlreadyClaimed_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	bob := mustAuthor(t, "bob")
	_, err := svc.ClaimByID(context.Background(), service.ClaimInput{
		TicketID: created.Ticket.ID(),
		Author:   bob,
	})

	// Then
	if !errors.Is(err, &domain.ClaimConflictError{}) {
		t.Errorf("expected ClaimConflictError, got %v", err)
	}
}

// --- TransitionState ---

func TestTransitionState_Close_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(context.Background(), service.TransitionInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
		Action:   service.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ticket is closed.
	show, _ := svc.ShowTicket(context.Background(), created.Ticket.ID())
	if show.Ticket.State() != ticket.StateClosed {
		t.Errorf("expected closed, got %s", show.Ticket.State())
	}
}

func TestTransitionState_Release_ReturnsToDefault(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.TransitionState(context.Background(), service.TransitionInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
		Action:   service.ActionRelease,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowTicket(context.Background(), created.Ticket.ID())
	if show.Ticket.State() != ticket.StateOpen {
		t.Errorf("expected open after release, got %s", show.Ticket.State())
	}
}

// --- UpdateTicket ---

func TestUpdateTicket_ChangesTitle(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Original",
		Author: author,
		Claim:  true,
	})

	// When
	newTitle := "Updated title"
	err := svc.UpdateTicket(context.Background(), service.UpdateTicketInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
		Title:    &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowTicket(context.Background(), created.Ticket.ID())
	if show.Ticket.Title() != "Updated title" {
		t.Errorf("expected Updated title, got %q", show.Ticket.Title())
	}
}

// --- OneShotUpdate ---

func TestOneShotUpdate_ChangesAndReleases(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Original",
		Author: author,
	})

	// When
	newTitle := "Quick fix"
	err := svc.OneShotUpdate(context.Background(), service.OneShotUpdateInput{
		TicketID: created.Ticket.ID(),
		Author:   author,
		Title:    &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	show, _ := svc.ShowTicket(context.Background(), created.Ticket.ID())
	if show.Ticket.Title() != "Quick fix" {
		t.Errorf("expected Quick fix, got %q", show.Ticket.Title())
	}
	if show.Ticket.State() != ticket.StateOpen {
		t.Errorf("expected open after one-shot, got %s", show.Ticket.State())
	}
}

// --- AddNote ---

func TestAddNote_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.AddNote(context.Background(), service.AddNoteInput{
		TicketID: created.Ticket.ID(),
		Author:   author,
		Body:     "This is a note.",
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.Note.Body() != "This is a note." {
		t.Errorf("expected note body, got %q", output.Note.Body())
	}
}

func TestAddNote_DeletedTicket_Fails(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// Delete the ticket.
	_ = svc.DeleteTicket(context.Background(), service.DeleteInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
	})

	// When
	_, err := svc.AddNote(context.Background(), service.AddNoteInput{
		TicketID: created.Ticket.ID(),
		Author:   author,
		Body:     "Note on deleted ticket",
	})

	// Then
	if !errors.Is(err, domain.ErrDeletedTicket) {
		t.Errorf("expected ErrDeletedTicket, got %v", err)
	}
}

func TestAddNote_ClosedTicket_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	_ = svc.TransitionState(context.Background(), service.TransitionInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
		Action:   service.ActionClose,
	})

	// When — notes CAN be added to closed tickets
	_, err := svc.AddNote(context.Background(), service.AddNoteInput{
		TicketID: created.Ticket.ID(),
		Author:   author,
		Body:     "Post-mortem note",
	})
	// Then
	if err != nil {
		t.Fatalf("expected success adding note to closed ticket, got: %v", err)
	}
}

// --- ShowTicket ---

func TestShowTicket_ReturnsRevisionAndAuthor(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	show, err := svc.ShowTicket(context.Background(), created.Ticket.ID())
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if show.Revision != 0 {
		t.Errorf("expected revision 0, got %d", show.Revision)
	}
	if !show.Author.Equal(author) {
		t.Errorf("expected author alice, got %s", show.Author)
	}
}

// --- ListTickets ---

func TestListTickets_FilterByReady(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	// Create two tasks — one open (ready), one claimed (not ready).
	_, _ = svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Ready task",
		Author: author,
	})
	_, _ = svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Claimed task",
		Author: author,
		Claim:  true,
	})

	// When
	output, err := svc.ListTickets(context.Background(), service.ListTicketsInput{
		Filter: port.TicketFilter{Ready: true},
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.TotalCount != 1 {
		t.Errorf("expected 1 ready ticket, got %d", output.TotalCount)
	}
}

func TestListTickets_ExcludeClosed_HidesClosedTickets(t *testing.T) {
	t.Parallel()

	// Given: one open task and one closed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateTicket(t.Context(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateTicket(t.Context(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), service.TransitionInput{
		TicketID: closed.Ticket.ID(),
		ClaimID:  closed.ClaimID,
		Action:   service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: listing with ExcludeClosed.
	output, err := svc.ListTickets(t.Context(), service.ListTicketsInput{
		Filter: port.TicketFilter{ExcludeClosed: true},
	})
	// Then: only the open task appears.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.TotalCount != 1 {
		t.Errorf("expected 1 ticket, got %d", output.TotalCount)
	}
	if len(output.Items) == 1 && output.Items[0].Title != "Open task" {
		t.Errorf("expected Open task, got %q", output.Items[0].Title)
	}
}

func TestListTickets_ExcludeClosed_WithExplicitClosedState_ShowsClosed(t *testing.T) {
	t.Parallel()

	// Given: one open task and one closed task.
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")

	_, err := svc.CreateTicket(t.Context(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Open task",
		Author: author,
	})
	if err != nil {
		t.Fatalf("precondition: create open task: %v", err)
	}

	closed, err := svc.CreateTicket(t.Context(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Closed task",
		Author: author,
		Claim:  true,
	})
	if err != nil {
		t.Fatalf("precondition: create closed task: %v", err)
	}
	err = svc.TransitionState(t.Context(), service.TransitionInput{
		TicketID: closed.Ticket.ID(),
		ClaimID:  closed.ClaimID,
		Action:   service.ActionClose,
	})
	if err != nil {
		t.Fatalf("precondition: close task: %v", err)
	}

	// When: ExcludeClosed is set but States explicitly requests closed — States
	// takes precedence because it represents an explicit user intent.
	output, err := svc.ListTickets(t.Context(), service.ListTicketsInput{
		Filter: port.TicketFilter{
			ExcludeClosed: true,
			States:        []ticket.State{ticket.StateClosed},
		},
	})
	// Then: only the closed task appears; ExcludeClosed is overridden.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.TotalCount != 1 {
		t.Errorf("expected 1 ticket, got %d", output.TotalCount)
	}
	if len(output.Items) == 1 && output.Items[0].Title != "Closed task" {
		t.Errorf("expected Closed task, got %q", output.Items[0].Title)
	}
}

// --- DeleteTicket ---

func TestDeleteTicket_TaskSucceeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.DeleteTicket(context.Background(), service.DeleteInput{
		TicketID: created.Ticket.ID(),
		ClaimID:  created.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Show should fail.
	_, err = svc.ShowTicket(context.Background(), created.Ticket.ID())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for deleted ticket, got %v", err)
	}
}

// --- ExtendStaleThreshold ---

func TestExtendStaleThreshold_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
		Claim:  true,
	})

	// When
	err := svc.ExtendStaleThreshold(context.Background(), created.Ticket.ID(), created.ClaimID, 12*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ShowHistory ---

func TestShowHistory_ReturnsEntries(t *testing.T) {
	t.Parallel()

	// Given
	svc, _ := setupService(t)
	author := mustAuthor(t, "alice")
	created, _ := svc.CreateTicket(context.Background(), service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Task",
		Author: author,
	})

	// When
	output, err := svc.ShowHistory(context.Background(), service.ListHistoryInput{
		TicketID: created.Ticket.ID(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output.TotalCount < 1 {
		t.Error("expected at least 1 history entry (creation)")
	}
}
