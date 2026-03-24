//go:build integration

package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/app/service"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/port"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
	"github.com/pinkhop/nitpicking/internal/storage/sqlite"
)

func setupIntegrationService(t *testing.T) service.Service {
	t.Helper()

	dbPath := t.TempDir() + "/test.db"
	store, err := sqlite.Create(dbPath)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := service.New(store)
	ctx := context.Background()
	if err := svc.Init(ctx, "TEST"); err != nil {
		t.Fatalf("initializing database: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("creating author: %v", err)
	}
	return a
}

func TestIntegration_FullTicketLifecycle(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "integration-test")

	// When — create a task
	createOut, err := svc.CreateTicket(ctx, service.CreateTicketInput{
		Role:   ticket.RoleTask,
		Title:  "Integration test task",
		Author: author,
		Claim:  true,
	})
	// Then
	if err != nil {
		t.Fatalf("creating ticket: %v", err)
	}
	if createOut.ClaimID == "" {
		t.Error("expected claim ID")
	}

	ticketID := createOut.Ticket.ID()

	// When — update the title
	newTitle := "Updated integration task"
	err = svc.UpdateTicket(ctx, service.UpdateTicketInput{
		TicketID: ticketID,
		ClaimID:  createOut.ClaimID,
		Title:    &newTitle,
	})
	// Then
	if err != nil {
		t.Fatalf("updating ticket: %v", err)
	}

	// When — show the ticket
	showOut, err := svc.ShowTicket(ctx, ticketID)
	// Then
	if err != nil {
		t.Fatalf("showing ticket: %v", err)
	}
	if showOut.Ticket.Title() != "Updated integration task" {
		t.Errorf("expected updated title, got %q", showOut.Ticket.Title())
	}

	// When — close the ticket
	err = svc.TransitionState(ctx, service.TransitionInput{
		TicketID: ticketID,
		ClaimID:  createOut.ClaimID,
		Action:   service.ActionClose,
	})
	// Then
	if err != nil {
		t.Fatalf("closing ticket: %v", err)
	}

	// Verify closed.
	showOut, _ = svc.ShowTicket(ctx, ticketID)
	if showOut.Ticket.State() != ticket.StateClosed {
		t.Errorf("expected closed, got %s", showOut.Ticket.State())
	}
}

func TestIntegration_NoteOnClosedTicket(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateTicket(ctx, service.CreateTicketInput{
		Role: ticket.RoleTask, Title: "Task", Author: author, Claim: true,
	})
	_ = svc.TransitionState(ctx, service.TransitionInput{
		TicketID: createOut.Ticket.ID(), ClaimID: createOut.ClaimID, Action: service.ActionClose,
	})

	// When — add note to closed ticket
	noteOut, err := svc.AddNote(ctx, service.AddNoteInput{
		TicketID: createOut.Ticket.ID(), Author: author, Body: "Post-close note",
	})
	// Then — should succeed per spec
	if err != nil {
		t.Fatalf("expected success adding note to closed ticket: %v", err)
	}
	if noteOut.Note.Body() != "Post-close note" {
		t.Errorf("expected note body, got %q", noteOut.Note.Body())
	}
}

func TestIntegration_ListAndPagination(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	for i := range 5 {
		_, err := svc.CreateTicket(ctx, service.CreateTicketInput{
			Role: ticket.RoleTask, Title: "Task " + string(rune('A'+i)), Author: author,
		})
		if err != nil {
			t.Fatalf("creating ticket %d: %v", i, err)
		}
	}

	// When — list with page size 3
	out, err := svc.ListTickets(ctx, service.ListTicketsInput{
		Page: port.PageRequest{PageSize: 3},
	})
	// Then
	if err != nil {
		t.Fatalf("listing tickets: %v", err)
	}
	if out.TotalCount != 5 {
		t.Errorf("expected 5 total, got %d", out.TotalCount)
	}
	if len(out.Items) != 3 {
		t.Errorf("expected 3 items on page, got %d", len(out.Items))
	}
}

func TestIntegration_DeleteAndNotFound(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateTicket(ctx, service.CreateTicketInput{
		Role: ticket.RoleTask, Title: "To delete", Author: author, Claim: true,
	})

	// When — delete
	err := svc.DeleteTicket(ctx, service.DeleteInput{
		TicketID: createOut.Ticket.ID(), ClaimID: createOut.ClaimID,
	})
	// Then
	if err != nil {
		t.Fatalf("deleting ticket: %v", err)
	}

	// Verify not found.
	_, err = svc.ShowTicket(ctx, createOut.Ticket.ID())
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestIntegration_ExtendStaleThreshold(t *testing.T) {
	// Given
	svc := setupIntegrationService(t)
	ctx := context.Background()
	author := mustAuthor(t, "alice")

	createOut, _ := svc.CreateTicket(ctx, service.CreateTicketInput{
		Role: ticket.RoleTask, Title: "Task", Author: author, Claim: true,
	})

	// When
	err := svc.ExtendStaleThreshold(ctx, createOut.Ticket.ID(), createOut.ClaimID, 8*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("extending threshold: %v", err)
	}
}
