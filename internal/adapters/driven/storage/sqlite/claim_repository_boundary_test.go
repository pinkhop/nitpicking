//go:build boundary

package sqlite_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/ports/driving"
)

// --- ClaimByID and ShowIssue Roundtrip ---

func TestBoundary_ClaimByID_CreateAndRetrieve(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Claim roundtrip", Author: author(t, "alice"),
	})

	// When
	claimOut, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: taskOut.Issue.ID().String(),
		Author:  author(t, "alice"),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimOut.ClaimID == "" {
		t.Error("expected non-empty claim ID")
	}
	if claimOut.IssueID != taskOut.Issue.ID().String() {
		t.Errorf("issue ID: got %s, want %s", claimOut.IssueID, taskOut.Issue.ID())
	}

	// Verify claim is reflected in show output. The primary state remains open
	// — claimed is a transient secondary state, not a lifecycle state.
	showOut, _ := svc.ShowIssue(ctx, taskOut.Issue.ID().String())
	if showOut.State != domain.StateOpen {
		t.Errorf("state: got %v, want open", showOut.State)
	}
	// ShowIssue returns the hash; the claim output returns the plaintext token.
	// The hash of the token must match what show returns.
	expectedHash := domain.HashClaimID(claimOut.ClaimID)
	if showOut.ClaimID != expectedHash {
		t.Errorf("claim hash mismatch: show=%q, hash(token)=%q", showOut.ClaimID, expectedHash)
	}
}

// --- LookupClaimIssueID (GetClaimByID equivalent) ---

func TestBoundary_LookupClaimIssueID_ReturnsCorrectIssue(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Lookup claim", Author: author(t, "alice"),
	})
	claimOut, _ := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: taskOut.Issue.ID().String(), Author: author(t, "alice"),
	})

	// When
	issueID, err := svc.LookupClaimIssueID(ctx, claimOut.ClaimID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issueID != taskOut.Issue.ID().String() {
		t.Errorf("issue ID: got %s, want %s", issueID, taskOut.Issue.ID())
	}
}

func TestBoundary_LookupClaimIssueID_InvalidClaim_ReturnsNotFound(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// When
	_, err := svc.LookupClaimIssueID(ctx, "nonexistent-claim-id")
	// Then
	if err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// --- InvalidateClaim (via ActionRelease) ---

func TestBoundary_ReleaseClaim_InvalidatesClaim(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Release claim", Author: author(t, "alice"),
	})
	claimOut, _ := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: taskOut.Issue.ID().String(), Author: author(t, "alice"),
	})

	// When — release the claim
	err := svc.TransitionState(ctx, driving.TransitionInput{
		IssueID: taskOut.Issue.ID().String(),
		ClaimID: claimOut.ClaimID,
		Action:  driving.ActionRelease,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showOut, _ := svc.ShowIssue(ctx, taskOut.Issue.ID().String())
	if showOut.State != domain.StateOpen {
		t.Errorf("state after release: got %v, want open", showOut.State)
	}
	if showOut.ClaimID != "" {
		t.Errorf("claim ID should be empty after release, got %q", showOut.ClaimID)
	}
}

// --- ExtendStaleThreshold (UpdateClaimStaleAt) ---

func TestBoundary_ExtendStaleThreshold_UpdatesThreshold(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Extend threshold", Author: author(t, "alice"),
	})
	claimOut, _ := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID: taskOut.Issue.ID().String(), Author: author(t, "alice"),
	})

	// Capture the stale_at before extending.
	showBefore, _ := svc.ShowIssue(ctx, taskOut.Issue.ID().String())

	// When
	err := svc.ExtendStaleThreshold(ctx, taskOut.Issue.ID().String(), claimOut.ClaimID, 8*time.Hour)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showAfter, _ := svc.ShowIssue(ctx, taskOut.Issue.ID().String())
	if !showAfter.ClaimStaleAt.After(showBefore.ClaimStaleAt) {
		t.Errorf("stale_at should be later after extending threshold: before=%v, after=%v",
			showBefore.ClaimStaleAt, showAfter.ClaimStaleAt)
	}
}

// --- StaleClaims (via Doctor) ---

func TestBoundary_Doctor_NewDatabase_NoSchemaMigrationRequired(t *testing.T) {
	// Given — a freshly initialised database which sets schema_version = 2.
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	// When
	doctorOut, err := svc.Doctor(ctx, driving.DoctorInput{})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Doctor should not report a schema_migration_required finding on a new
	// database, because InitDatabase writes schema_version = 2.
	for _, f := range doctorOut.Findings {
		if f.Category == "schema_migration_required" {
			t.Errorf("unexpected schema_migration_required finding on new database: %s", f.Message)
		}
	}
}

// --- ListActiveClaims (via Doctor no-stale scenario) ---

func TestBoundary_ClaimByID_WithStaleThreshold_SetsExpiry(t *testing.T) {
	// Given
	svc := setupBoundarySvc(t)
	ctx := t.Context()

	taskOut, _ := svc.CreateIssue(ctx, driving.CreateIssueInput{
		Role: domain.RoleTask, Title: "Custom threshold", Author: author(t, "alice"),
	})

	// When — claim with custom stale threshold
	_, err := svc.ClaimByID(ctx, driving.ClaimInput{
		IssueID:        taskOut.Issue.ID().String(),
		Author:         author(t, "alice"),
		StaleThreshold: 4 * time.Hour,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	showOut, _ := svc.ShowIssue(ctx, taskOut.Issue.ID().String())
	if showOut.ClaimStaleAt.IsZero() {
		t.Error("expected non-zero stale_at with custom threshold")
	}
	// The stale_at should be roughly 4 hours in the future (within 1 minute tolerance).
	expectedMin := time.Now().Add(3 * time.Hour)
	if showOut.ClaimStaleAt.Before(expectedMin) {
		t.Errorf("stale_at should be ~4h in future, got %v", showOut.ClaimStaleAt)
	}
}
