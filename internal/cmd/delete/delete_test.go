package delete_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/cmd/delete"
	"github.com/pinkhop/nitpicking/internal/cmdutil"
	"github.com/pinkhop/nitpicking/internal/core"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/iostreams"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- Helpers ---

func setupService(t *testing.T) driving.Service {
	t.Helper()
	repo := memory.NewRepository()
	tx := memory.NewTransactor(repo)
	svc := core.New(tx)

	ctx := t.Context()
	if err := svc.Init(ctx, "NP"); err != nil {
		t.Fatalf("precondition: init failed: %v", err)
	}
	return svc
}

func mustAuthor(t *testing.T, name string) string {
	t.Helper()
	return name
}

func createAndClaim(t *testing.T, svc driving.Service, title string) (domain.ID, string) {
	t.Helper()
	ctx := t.Context()
	out, err := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role:   domain.RoleTask,
		Title:  title,
		Author: mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: create issue failed: %v", err)
	}

	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: out.Issue.ID().String(),
		Author:  mustAuthor(t, "test-agent"),
	})
	if err != nil {
		t.Fatalf("precondition: claim failed: %v", err)
	}
	return out.Issue.ID(), claimOut.ClaimID
}

func noColor() *iostreams.ColorScheme {
	return iostreams.NewColorScheme(false)
}

// --- Run Tests ---

func TestRun_MissingConfirm_ReturnsFlagError(t *testing.T) {
	t.Parallel()

	// Given — a claimed issue but confirm is false
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Task to delete")

	var buf bytes.Buffer
	input := delete.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		ClaimID:     claimID,
		Confirm:     false,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := delete.Run(t.Context(), input)
	// Then
	if err == nil {
		t.Fatal("expected error when --confirm is not set")
	}
	if _, ok := errors.AsType[*cmdutil.FlagError](err); !ok {
		t.Errorf("expected FlagError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "confirm") {
		t.Errorf("error should mention 'confirm', got: %v", err)
	}
}

func TestRun_CorrectClaimAndConfirm_DeletesIssue(t *testing.T) {
	t.Parallel()

	// Given — a claimed issue with confirm=true
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Task to delete")

	var buf bytes.Buffer
	input := delete.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		ClaimID:     claimID,
		Confirm:     true,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := delete.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result struct {
		IssueID string `json:"issue_id"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	if result.IssueID != issueID.String() {
		t.Errorf("issue_id: got %q, want %q", result.IssueID, issueID.String())
	}
	if !result.Deleted {
		t.Error("deleted: got false, want true")
	}
}

func TestRun_JSONOutput_ContainsExpectedFields(t *testing.T) {
	t.Parallel()

	// Given — a claimed issue
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Task for JSON check")

	var buf bytes.Buffer
	input := delete.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		ClaimID:     claimID,
		Confirm:     true,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := delete.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, buf.String())
	}
	for _, field := range []string{"issue_id", "deleted"} {
		if _, ok := result[field]; !ok {
			t.Errorf("expected field %q in JSON output", field)
		}
	}
}

func TestRun_TextOutput_ShowsDeletedMessage(t *testing.T) {
	t.Parallel()

	// Given — a claimed issue
	svc := setupService(t)
	issueID, claimID := createAndClaim(t, svc, "Task for text output")

	var buf bytes.Buffer
	input := delete.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		ClaimID:     claimID,
		Confirm:     true,
		JSON:        false,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := delete.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Deleted") {
		t.Errorf("expected 'Deleted' in text output, got: %s", output)
	}
	if !strings.Contains(output, issueID.String()) {
		t.Errorf("expected issue ID %s in text output, got: %s", issueID, output)
	}
}

func TestRun_WrongClaimID_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — a claimed issue but providing a wrong claim ID
	svc := setupService(t)
	issueID, _ := createAndClaim(t, svc, "Task with wrong claim")

	var buf bytes.Buffer
	input := delete.RunInput{
		Service:     svc,
		IssueID:     issueID.String(),
		ClaimID:     "wrong-claim-id",
		Confirm:     true,
		JSON:        true,
		WriteTo:     &buf,
		ColorScheme: noColor(),
	}

	// When
	err := delete.Run(t.Context(), input)
	// Then
	if err == nil {
		t.Fatal("expected error when using wrong claim ID")
	}
}
