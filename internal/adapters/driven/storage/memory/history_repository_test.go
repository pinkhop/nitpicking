package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/storage/memory"
	"github.com/pinkhop/nitpicking/internal/domain"
	"github.com/pinkhop/nitpicking/internal/domain/history"
	"github.com/pinkhop/nitpicking/internal/ports/driven"
)

// --- AppendHistory ---

func TestAppendHistory_AssignsAutoIncrementID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given
	issueID := mustIssueID(t)
	author := mustAuthor(t, "alice")
	now := time.Now()

	entry := history.NewEntry(history.NewEntryParams{
		IssueID:   issueID,
		Revision:  0,
		Author:    author,
		Timestamp: now,
		EventType: history.EventCreated,
	})

	// When
	id1, err := repo.AppendHistory(ctx, entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry2 := history.NewEntry(history.NewEntryParams{
		IssueID:   issueID,
		Revision:  1,
		Author:    author,
		Timestamp: now.Add(time.Second),
		EventType: history.EventUpdated,
	})
	id2, err := repo.AppendHistory(ctx, entry2)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 >= id2 {
		t.Errorf("expected auto-increment IDs (id1=%d < id2=%d)", id1, id2)
	}
}

// --- ListHistory ---

func TestListHistory_ReturnsEntriesForIssue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — two entries for the same issue
	issueID := mustIssueID(t)
	author := mustAuthor(t, "bob")
	now := time.Now()

	for i := 0; i < 2; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  i,
			Author:    author,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			EventType: history.EventCreated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: append history: %v", err)
		}
	}

	// When
	entries, hasMore, err := repo.ListHistory(ctx, issueID, driven.HistoryFilter{}, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
	if hasMore {
		t.Error("expected hasMore=false with negative limit")
	}
}

func TestListHistory_FilterByAuthor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — two entries by different authors
	issueID := mustIssueID(t)
	alice := mustAuthor(t, "alice")
	bob := mustAuthor(t, "bob")
	now := time.Now()

	for _, a := range []domain.Author{alice, bob} {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Author:    a,
			Timestamp: now,
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	entries, _, err := repo.ListHistory(ctx, issueID, driven.HistoryFilter{Author: alice}, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry by alice, got %d", len(entries))
	}
}

func TestListHistory_FilterByAfter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — three entries at t=0, t=1h, t=2h
	issueID := mustIssueID(t)
	author := mustAuthor(t, "carol")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Author:    author,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — filter to entries after the 1h mark
	entries, _, err := repo.ListHistory(ctx, issueID, driven.HistoryFilter{
		After: base.Add(time.Hour),
	}, -1)
	// Then — only the t=2h entry should match (After is exclusive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after 1h, got %d", len(entries))
	}
}

func TestListHistory_FilterByBefore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — three entries at t=0, t=1h, t=2h
	issueID := mustIssueID(t)
	author := mustAuthor(t, "dave")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Author:    author,
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — filter to entries before the 1h mark
	entries, _, err := repo.ListHistory(ctx, issueID, driven.HistoryFilter{
		Before: base.Add(time.Hour),
	}, -1)
	// Then — only the t=0 entry should match (Before is exclusive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry before 1h, got %d", len(entries))
	}
}

func TestListHistory_Pagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — 3 entries
	issueID := mustIssueID(t)
	author := mustAuthor(t, "eve")
	now := time.Now()

	for i := 0; i < 3; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Author:    author,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When — limit 2
	entries, hasMore, err := repo.ListHistory(ctx, issueID, driven.HistoryFilter{}, 2)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
	if !hasMore {
		t.Error("expected hasMore=true with 3 entries and limit=2")
	}
}

func TestListHistory_EmptyForUnknownIssue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	entries, hasMore, err := repo.ListHistory(ctx, mustIssueID(t), driven.HistoryFilter{}, -1)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if hasMore {
		t.Error("expected hasMore=false for empty history")
	}
}

// --- CountHistory ---

func TestCountHistory_ReturnsCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — 3 entries for one issue
	issueID := mustIssueID(t)
	author := mustAuthor(t, "frank")
	now := time.Now()

	for i := 0; i < 3; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Author:    author,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	count, err := repo.CountHistory(ctx, issueID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}
}

func TestCountHistory_ZeroForUnknownIssue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	count, err := repo.CountHistory(ctx, mustIssueID(t))
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count=0, got %d", count)
	}
}

// --- GetLatestHistory ---

func TestGetLatestHistory_ReturnsMostRecent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// Given — 3 entries; the last appended should be "latest"
	issueID := mustIssueID(t)
	author := mustAuthor(t, "grace")
	now := time.Now()

	for i := 0; i < 3; i++ {
		e := history.NewEntry(history.NewEntryParams{
			IssueID:   issueID,
			Revision:  i,
			Author:    author,
			Timestamp: now.Add(time.Duration(i) * time.Second),
			EventType: history.EventUpdated,
		})
		if _, err := repo.AppendHistory(ctx, e); err != nil {
			t.Fatalf("precondition: %v", err)
		}
	}

	// When
	latest, err := repo.GetLatestHistory(ctx, issueID)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest.Revision() != 2 {
		t.Errorf("expected revision 2 (most recent), got %d", latest.Revision())
	}
}

func TestGetLatestHistory_EmptyHistory_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := memory.NewRepository()

	// When
	_, err := repo.GetLatestHistory(ctx, mustIssueID(t))

	// Then
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
