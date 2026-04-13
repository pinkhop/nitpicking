package formcmd_test

import (
	"bytes"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/formcmd"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Test helpers ---

// setupCreateService initialises a service backed by the in-memory adapter
// and returns it ready for form create tests.
func setupCreateService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx, nil)

	if err := svc.Init(t.Context(), "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

// createParentEpic creates an epic and returns its ID for use as a parent in
// tests.
func createParentEpic(t *testing.T, svc driving.Service, title string) domain.ID {
	t.Helper()
	out, err := svc.CreateIssue(t.Context(), driving.CreateIssueInput{
		Role:   domain.RoleEpic,
		Title:  title,
		Author: "test-agent",
	})
	if err != nil {
		t.Fatalf("precondition: create epic failed: %v", err)
	}
	return out.Issue.ID()
}

// --- RunFormCreate Tests ---

func TestRunFormCreate_MinimalFields_CreatesTask(t *testing.T) {
	t.Parallel()

	// Given: a form runner that populates minimal required fields.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Title = "Fix login bug"
			data.Author = "alice"
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)
	// Then: no error, and confirmation message contains the issue ID.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("NP-")) {
		t.Errorf("expected output to contain issue ID (NP-...), got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("Fix login bug")) {
		t.Errorf("expected output to contain issue title, got: %s", out)
	}
}

func TestRunFormCreate_AllFields_CreatesIssueWithAllData(t *testing.T) {
	t.Parallel()

	// Given: a form runner that populates all fields including optional ones.
	svc := setupCreateService(t)
	epicID := createParentEpic(t, svc, "Parent epic")
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Title = "Full featured task"
			data.Description = "A detailed description"
			data.AcceptanceCriteria = "All tests pass"
			data.Priority = "P0"
			data.Parent = epicID.String()
			data.Labels = "kind:feat, area:auth"
			data.Author = "bob"
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)
	// Then: no error, and the issue was created with correct priority.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("P0")) {
		t.Errorf("expected output to contain priority P0, got: %s", out)
	}
}

func TestRunFormCreate_EpicRole_CreatesEpic(t *testing.T) {
	t.Parallel()

	// Given: a form runner that selects the epic role.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "epic"
			data.Title = "Large initiative"
			data.Author = "charlie"
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)
	// Then: no error, and output confirms epic creation.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !bytes.Contains([]byte(out), []byte("epic")) {
		t.Errorf("expected output to mention 'epic', got: %s", out)
	}
}

func TestRunFormCreate_UserAborts_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that simulates user abort.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(_ *formcmd.CreateFormData) error {
			return formcmd.ErrUserAborted
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)

	// Then: the abort error is surfaced.
	if err == nil {
		t.Fatal("expected error for user abort, got nil")
	}
}

func TestRunFormCreate_MissingTitle_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides role but no title.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Author = "alice"
			// Title is intentionally empty.
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)

	// Then: an error is returned because title is required.
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
}

func TestRunFormCreate_MissingAuthor_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides role and title but no author.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Title = "Some task"
			// Author is intentionally empty.
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)

	// Then: an error is returned because author is required.
	if err == nil {
		t.Fatal("expected error for missing author, got nil")
	}
}

func TestRunFormCreate_InvalidLabels_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides a label without colon separator.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Title = "Task with bad labels"
			data.Author = "alice"
			data.Labels = "no-colon"
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)

	// Then: an error is returned because the label format is invalid.
	if err == nil {
		t.Fatal("expected error for invalid label format, got nil")
	}
}

func TestRunFormCreate_InvalidParent_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given: a form runner that provides an invalid parent ID.
	svc := setupCreateService(t)
	var stdout bytes.Buffer

	input := formcmd.RunFormCreateInput{
		Service: svc,
		WriteTo: &stdout,
		FormRunner: func(data *formcmd.CreateFormData) error {
			data.Role = "task"
			data.Title = "Task with bad parent"
			data.Author = "alice"
			data.Parent = "INVALID-ID"
			return nil
		},
	}

	// When
	err := formcmd.RunFormCreate(t.Context(), input)

	// Then: an error is returned because the parent ID is invalid.
	if err == nil {
		t.Fatal("expected error for invalid parent ID, got nil")
	}
}
