package domain_test

import (
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain"
)

// isCrockfordCharClaim reports whether r is a valid lowercase Crockford Base32
// character. Duplicated from id.go for test independence (unexported).
func isCrockfordCharClaim(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		return r != 'i' && r != 'l' && r != 'o' && r != 'u'
	}
	return false
}

func mustAuthor(t *testing.T, name string) domain.Author {
	t.Helper()
	a, err := domain.NewAuthor(name)
	if err != nil {
		t.Fatalf("failed to create author: %v", err)
	}
	return a
}

func TestNewClaim_ValidParams_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	tid := mustID(t)
	author := mustAuthor(t, "alice")
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: tid,
		Author:  author,
		Now:     now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ID is the SHA-512 hash (128 hex chars).
	if c.ID() == "" {
		t.Error("expected non-empty claim hash ID")
	}
	if len(c.ID()) != 128 {
		t.Errorf("expected 128-char SHA-512 hex hash, got %d chars", len(c.ID()))
	}
	// Token is the Crockford Base32 plaintext (26 chars).
	if c.Token() == "" {
		t.Error("expected non-empty claim token")
	}
	if len(c.Token()) != 26 {
		t.Errorf("expected 26-char Crockford Base32 token, got %d chars: %q", len(c.Token()), c.Token())
	}
	// Every character in Token must be in the Crockford Base32 alphabet.
	for _, r := range c.Token() {
		if !isCrockfordCharClaim(r) {
			t.Errorf("claim token contains non-Crockford character %q in %q", r, c.Token())
			break
		}
	}
	// ID must not equal Token — one is the hash, the other is the plaintext.
	if c.ID() == c.Token() {
		t.Error("hash ID and plaintext token must differ")
	}
	if c.IssueID() != tid {
		t.Errorf("expected issue ID %s, got %s", tid, c.IssueID())
	}
	if !c.Author().Equal(author) {
		t.Errorf("expected author alice, got %s", c.Author())
	}
	// Verify staleAt is now + DefaultStaleThreshold.
	expectedStaleAt := now.Add(domain.DefaultStaleThreshold)
	if !c.StaleAt().Equal(expectedStaleAt) {
		t.Errorf("expected stale at %v, got %v", expectedStaleAt, c.StaleAt())
	}
}

func TestNewClaim_CustomStaleDuration_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       mustID(t),
		Author:        mustAuthor(t, "bob"),
		StaleDuration: 6 * time.Hour,
		Now:           now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(6 * time.Hour)
	if !c.StaleAt().Equal(expected) {
		t.Errorf("expected stale at %v, got %v", expected, c.StaleAt())
	}
}

func TestNewClaim_AbsoluteStaleAt_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(3 * time.Hour)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "bob"),
		StaleAt: staleAt,
		Now:     now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.StaleAt().Equal(staleAt) {
		t.Errorf("expected stale at %v, got %v", staleAt, c.StaleAt())
	}
}

func TestNewClaim_AbsoluteStaleAt_TakesPrecedenceOverDuration(t *testing.T) {
	t.Parallel()

	// Given — both StaleAt and StaleDuration are set; StaleAt should win.
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(5 * time.Hour)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       mustID(t),
		Author:        mustAuthor(t, "bob"),
		StaleDuration: 1 * time.Hour,
		StaleAt:       staleAt,
		Now:           now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.StaleAt().Equal(staleAt) {
		t.Errorf("expected stale at %v (from StaleAt), got %v", staleAt, c.StaleAt())
	}
}

func TestNewClaim_AbsoluteStaleAt_ExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// Given — StaleAt is more than 24h from now.
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(25 * time.Hour)

	// When
	_, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "bob"),
		StaleAt: staleAt,
		Now:     now,
	})

	// Then
	if err == nil {
		t.Fatal("expected error for stale-at exceeding max distance")
	}
}

func TestNewClaim_StaleDurationExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       mustID(t),
		Author:        mustAuthor(t, "bob"),
		StaleDuration: 25 * time.Hour,
		Now:           time.Now(),
	})

	// Then
	if err == nil {
		t.Fatal("expected error for stale duration exceeding max")
	}
}

func TestNewClaim_ZeroIssueID_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewClaim(domain.NewClaimParams{
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
	_, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
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
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
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
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
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
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
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

func TestClaim_WithStaleAt_ReturnsNewClaim(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	original, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	newStaleAt := now.Add(6 * time.Hour)
	updated := original.WithStaleAt(newStaleAt)

	// Then — updated claim has the new staleAt
	if !updated.StaleAt().Equal(newStaleAt) {
		t.Errorf("expected staleAt %v, got %v", newStaleAt, updated.StaleAt())
	}
	// Original is unchanged (value semantics)
	originalExpected := now.Add(domain.DefaultStaleThreshold)
	if !original.StaleAt().Equal(originalExpected) {
		t.Errorf("expected original staleAt %v, got %v", originalExpected, original.StaleAt())
	}
}

func TestClaim_IsStale_AtExactThreshold_NotStale(t *testing.T) {
	t.Parallel()

	// Given — claim created at a known time with default 2h threshold
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — check staleness at exactly the stale-at boundary
	exactBoundary := c.StaleAt()
	stale := c.IsStale(exactBoundary)

	// Then — at the exact boundary, the claim is not yet stale (strict >)
	if stale {
		t.Error("expected not stale at exact threshold boundary")
	}
}

func TestReconstructClaim_PreservesHashID(t *testing.T) {
	t.Parallel()

	// Given — create a claim via NewClaim
	issueID := mustID(t)
	author := mustAuthor(t, "alice")
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	threshold := 4 * time.Hour
	original, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       issueID,
		Author:        author,
		StaleDuration: threshold,
		Now:           now,
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — reconstruct with the hash ID (simulating DB load)
	reconstructed := domain.ReconstructClaim(
		original.ID(),
		issueID,
		author,
		now,
		now.Add(threshold),
	)

	// Then — hash IDs match; token is empty for reconstructed claims
	if reconstructed.ID() != original.ID() {
		t.Errorf("ID: expected %q, got %q", original.ID(), reconstructed.ID())
	}
	if reconstructed.Token() != "" {
		t.Errorf("Token: expected empty for reconstructed claim, got %q", reconstructed.Token())
	}
	if reconstructed.IssueID() != original.IssueID() {
		t.Errorf("IssueID: expected %s, got %s", original.IssueID(), reconstructed.IssueID())
	}
	if !reconstructed.Author().Equal(original.Author()) {
		t.Errorf("Author: expected %s, got %s", original.Author(), reconstructed.Author())
	}
	if !reconstructed.ClaimedAt().Equal(original.ClaimedAt()) {
		t.Errorf("ClaimedAt: expected %v, got %v", original.ClaimedAt(), reconstructed.ClaimedAt())
	}
	if !reconstructed.StaleAt().Equal(original.StaleAt()) {
		t.Errorf("StaleAt: expected %v, got %v", original.StaleAt(), reconstructed.StaleAt())
	}
}

func TestHashClaimID_ProducesDeterministicSHA512(t *testing.T) {
	t.Parallel()

	// Given — a Crockford Base32 token
	token := "0a1b2c3d4e5f6g7h8j9k0a1b2c"

	// When — hash via HashClaimID
	hash := domain.HashClaimID(token)

	// Then — hash is 128 hex chars (SHA-512 = 64 bytes = 128 hex)
	if len(hash) != 128 {
		t.Fatalf("expected 128-char hash, got %d chars", len(hash))
	}

	// The hash must be deterministic.
	if domain.HashClaimID(token) != hash {
		t.Error("expected deterministic hash")
	}

	// Different tokens must produce different hashes.
	other := "00000000000000000000000000"
	if domain.HashClaimID(other) == hash {
		t.Error("expected different hashes for different tokens")
	}
}

func TestHashClaimID_IDMatchesHashOfToken(t *testing.T) {
	t.Parallel()

	// Given — a freshly created claim
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("precondition: %v", err)
	}

	// When — hash the token
	expectedHash := domain.HashClaimID(c.Token())

	// Then — it must match the claim's ID
	if c.ID() != expectedHash {
		t.Errorf("ID %q does not match HashClaimID(Token()) %q", c.ID(), expectedHash)
	}
}

func TestNewClaim_ClaimedAt_EqualsNow(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.ClaimedAt().Equal(now) {
		t.Errorf("expected ClaimedAt() = %v, got %v", now, c.ClaimedAt())
	}
}

func TestNewClaim_StaleAtField_EqualsNowPlusDuration(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	threshold := 4 * time.Hour

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:       mustID(t),
		Author:        mustAuthor(t, "alice"),
		StaleDuration: threshold,
		Now:           now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(threshold)
	if !c.StaleAt().Equal(expected) {
		t.Errorf("expected StaleAt() = %v, got %v", expected, c.StaleAt())
	}
}

func TestNewClaim_DefaultThreshold_StaleAtEqualsNowPlusDefault(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(domain.DefaultStaleThreshold)
	if !c.StaleAt().Equal(expected) {
		t.Errorf("expected StaleAt() = %v, got %v", expected, c.StaleAt())
	}
}

func TestReconstructClaim_ClaimedAt_PreservesInput(t *testing.T) {
	t.Parallel()

	// Given
	claimedAt := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	staleAt := claimedAt.Add(4 * time.Hour)

	// When
	c := domain.ReconstructClaim("somehash", mustID(t), mustAuthor(t, "alice"), claimedAt, staleAt)

	// Then
	if !c.ClaimedAt().Equal(claimedAt) {
		t.Errorf("expected ClaimedAt() = %v, got %v", claimedAt, c.ClaimedAt())
	}
}

func TestReconstructClaim_StaleAt_PreservesInput(t *testing.T) {
	t.Parallel()

	// Given
	claimedAt := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	staleAt := claimedAt.Add(4 * time.Hour)

	// When
	c := domain.ReconstructClaim("somehash", mustID(t), mustAuthor(t, "alice"), claimedAt, staleAt)

	// Then
	if !c.StaleAt().Equal(staleAt) {
		t.Errorf("expected StaleAt() = %v, got %v", staleAt, c.StaleAt())
	}
}

func TestNewClaim_GeneratesUniqueTokensAndHashes(t *testing.T) {
	t.Parallel()

	// When
	c1, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     time.Now(),
	})
	c2, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "bob"),
		Now:     time.Now(),
	})

	// Then — both tokens and hash IDs are unique.
	if c1.Token() == c2.Token() {
		t.Error("expected unique claim tokens")
	}
	if c1.ID() == c2.ID() {
		t.Error("expected unique claim hash IDs")
	}
}
