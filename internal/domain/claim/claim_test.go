package claim_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/claim"
	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/issue"
)

func mustAuthor(t *testing.T, name string) identity.Author {
	t.Helper()
	a, err := identity.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func mustIssueID(t *testing.T) issue.ID {
	t.Helper()
	id, err := issue.GenerateID("NP", nil)
	if err != nil {
		t.Fatalf("failed to generate issue ID: %v", err)
	}
	return id
}

func TestNewClaim_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustIssueID(t)
	author := mustAuthor(t, "alice")
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := claim.NewClaim(claim.NewClaimParams{
		IssueID: tid,
		Author:  author,
		Now:     now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ID() == "" {
		t.Error("expected non-empty claim ID")
	}
	if len(c.ID()) != 32 {
		t.Errorf("expected 32-char hex ID, got %d chars", len(c.ID()))
	}
	if c.IssueID() != tid {
		t.Errorf("expected issue ID %s, got %s", tid, c.IssueID())
	}
	if !c.Author().Equal(author) {
		t.Errorf("expected author alice, got %s", c.Author())
	}
	if c.StaleThreshold() != claim.DefaultStaleThreshold {
		t.Errorf("expected default threshold, got %v", c.StaleThreshold())
	}
}

func TestNewClaim_CustomThreshold_Succeeds(t *testing.T) {
	t.Parallel()

	// When
	c, err := claim.NewClaim(claim.NewClaimParams{
		IssueID:        mustIssueID(t),
		Author:         mustAuthor(t, "bob"),
		StaleThreshold: 6 * time.Hour,
		Now:            time.Now(),
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.StaleThreshold() != 6*time.Hour {
		t.Errorf("expected 6h threshold, got %v", c.StaleThreshold())
	}
}

func TestNewClaim_ThresholdExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := claim.NewClaim(claim.NewClaimParams{
		IssueID:        mustIssueID(t),
		Author:         mustAuthor(t, "bob"),
		StaleThreshold: 25 * time.Hour,
		Now:            time.Now(),
	})

	// Then
	if err == nil {
		t.Fatal("expected error for threshold exceeding max")
	}
}

func TestNewClaim_ZeroIssueID_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := claim.NewClaim(claim.NewClaimParams{
		Author: mustAuthor(t, "alice"),
		Now:    time.Now(),
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero issue ID")
	}
}

func TestNewClaim_ZeroAuthor_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Now:     time.Now(),
	})

	// Then
	if err == nil {
		t.Fatal("expected error for zero author")
	}
}

func TestClaim_IsStale_BeforeThreshold_NotStale(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	stale := c.IsStale(now.Add(1 * time.Hour))

	// Then
	if stale {
		t.Error("expected not stale within threshold")
	}
}

func TestClaim_IsStale_AfterThreshold_Stale(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	stale := c.IsStale(now.Add(3 * time.Hour))

	// Then
	if !stale {
		t.Error("expected stale after threshold")
	}
}

func TestClaim_StaleAt_ReturnsCorrectTimestamp(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	staleAt := c.StaleAt()

	// Then
	expected := now.Add(2 * time.Hour)
	if !staleAt.Equal(expected) {
		t.Errorf("expected stale at %v, got %v", expected, staleAt)
	}
}

func TestClaim_WithLastActivity_ReturnsNewClaim(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	original, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	later := now.Add(30 * time.Minute)
	updated := original.WithLastActivity(later)

	// Then
	if !updated.LastActivity().Equal(later) {
		t.Errorf("expected updated last activity, got %v", updated.LastActivity())
	}
	if !original.LastActivity().Equal(now) {
		t.Error("expected original unchanged")
	}
}

func TestClaim_WithStaleThreshold_ReturnsNewClaim(t *testing.T) {
	t.Parallel()

	// Given
	original, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})

	// When
	updated, err := original.WithStaleThreshold(12 * time.Hour)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.StaleThreshold() != 12*time.Hour {
		t.Errorf("expected 12h, got %v", updated.StaleThreshold())
	}
}

func TestClaim_WithStaleThreshold_ExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// Given
	c, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})

	// When
	_, err := c.WithStaleThreshold(25 * time.Hour)

	// Then
	if err == nil {
		t.Fatal("expected error for threshold exceeding max")
	}
}

func TestNewClaim_GeneratesUniqueIDs(t *testing.T) {
	t.Parallel()

	// When
	c1, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})
	c2, _ := claim.NewClaim(claim.NewClaimParams{
		IssueID: mustIssueID(t),
		Author:  mustAuthor(t, "bob"),
		Now:     time.Now(),
	})

	// Then
	if c1.ID() == c2.ID() {
		t.Error("expected unique claim IDs")
	}
}
