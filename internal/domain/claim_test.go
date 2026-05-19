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
	// Verify expiresAt is now + DefaultExpiryThreshold.
	expectedExpiresAt := now.Add(domain.DefaultExpiryThreshold)
	if !c.ExpiresAt().Equal(expectedExpiresAt) {
		t.Errorf("expected expires at %v, got %v", expectedExpiresAt, c.ExpiresAt())
	}
}

func TestNewClaim_CustomExpiresAfter_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:      mustID(t),
		Author:       mustAuthor(t, "bob"),
		ExpiresAfter: 6 * time.Hour,
		Now:          now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(6 * time.Hour)
	if !c.ExpiresAt().Equal(expected) {
		t.Errorf("expected expires at %v, got %v", expected, c.ExpiresAt())
	}
}

func TestNewClaim_AbsoluteExpiresAt_Succeeds(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(3 * time.Hour)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:   mustID(t),
		Author:    mustAuthor(t, "bob"),
		ExpiresAt: expiresAt,
		Now:       now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.ExpiresAt().Equal(expiresAt) {
		t.Errorf("expected expires at %v, got %v", expiresAt, c.ExpiresAt())
	}
}

func TestNewClaim_AbsoluteExpiresAt_TakesPrecedenceOverDuration(t *testing.T) {
	t.Parallel()

	// Given — both ExpiresAt and ExpiresAfter are set; ExpiresAt should win.
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(5 * time.Hour)

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:      mustID(t),
		Author:       mustAuthor(t, "bob"),
		ExpiresAfter: 1 * time.Hour,
		ExpiresAt:    expiresAt,
		Now:          now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.ExpiresAt().Equal(expiresAt) {
		t.Errorf("expected expires at %v (from ExpiresAt), got %v", expiresAt, c.ExpiresAt())
	}
}

func TestNewClaim_AbsoluteExpiresAt_ExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// Given — ExpiresAt is more than 24h from now.
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(25 * time.Hour)

	// When
	_, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:   mustID(t),
		Author:    mustAuthor(t, "bob"),
		ExpiresAt: expiresAt,
		Now:       now,
	})

	// Then
	if err == nil {
		t.Fatal("expected error for expires-at exceeding max distance")
	}
}

func TestNewClaim_ExpiresAfterExceedsMax_Fails(t *testing.T) {
	t.Parallel()

	// When
	_, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:      mustID(t),
		Author:       mustAuthor(t, "bob"),
		ExpiresAfter: 25 * time.Hour,
		Now:          time.Now(),
	})

	// Then
	if err == nil {
		t.Fatal("expected error for expires-after exceeding max")
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

func TestClaim_IsExpired_BeforeThreshold_NotExpired(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	expired := c.IsExpired(now.Add(1 * time.Hour))

	// Then
	if expired {
		t.Error("expected not expired within threshold")
	}
}

func TestClaim_IsExpired_AfterThreshold_Expired(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	expired := c.IsExpired(now.Add(3 * time.Hour))

	// Then
	if !expired {
		t.Error("expected expired after threshold")
	}
}

func TestClaim_ExpiresAt_ReturnsCorrectTimestamp(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	c, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	expiresAt := c.ExpiresAt()

	// Then
	expected := now.Add(2 * time.Hour)
	if !expiresAt.Equal(expected) {
		t.Errorf("expected expires at %v, got %v", expected, expiresAt)
	}
}

func TestClaim_WithExpiresAt_ReturnsNewClaim(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	original, _ := domain.NewClaim(domain.NewClaimParams{
		IssueID: mustID(t),
		Author:  mustAuthor(t, "alice"),
		Now:     now,
	})

	// When
	newExpiresAt := now.Add(6 * time.Hour)
	updated := original.WithExpiresAt(newExpiresAt)

	// Then — updated claim has the new expiresAt
	if !updated.ExpiresAt().Equal(newExpiresAt) {
		t.Errorf("expected expiresAt %v, got %v", newExpiresAt, updated.ExpiresAt())
	}
	// Original is unchanged (value semantics)
	originalExpected := now.Add(domain.DefaultExpiryThreshold)
	if !original.ExpiresAt().Equal(originalExpected) {
		t.Errorf("expected original expiresAt %v, got %v", originalExpected, original.ExpiresAt())
	}
}

func TestClaim_IsExpired_AtExactThreshold_NotExpired(t *testing.T) {
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

	// When — check expiry at exactly the expires-at boundary
	exactBoundary := c.ExpiresAt()
	expired := c.IsExpired(exactBoundary)

	// Then — at the exact boundary, the claim is not yet expired (strict >)
	if expired {
		t.Error("expected not expired at exact threshold boundary")
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
		IssueID:      issueID,
		Author:       author,
		ExpiresAfter: threshold,
		Now:          now,
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
	if !reconstructed.ExpiresAt().Equal(original.ExpiresAt()) {
		t.Errorf("ExpiresAt: expected %v, got %v", original.ExpiresAt(), reconstructed.ExpiresAt())
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

func TestNewClaim_ExpiresAtField_EqualsNowPlusDuration(t *testing.T) {
	t.Parallel()

	// Given
	now := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	threshold := 4 * time.Hour

	// When
	c, err := domain.NewClaim(domain.NewClaimParams{
		IssueID:      mustID(t),
		Author:       mustAuthor(t, "alice"),
		ExpiresAfter: threshold,
		Now:          now,
	})
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := now.Add(threshold)
	if !c.ExpiresAt().Equal(expected) {
		t.Errorf("expected ExpiresAt() = %v, got %v", expected, c.ExpiresAt())
	}
}

func TestNewClaim_DefaultThreshold_ExpiresAtEqualsNowPlusDefault(t *testing.T) {
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
	expected := now.Add(domain.DefaultExpiryThreshold)
	if !c.ExpiresAt().Equal(expected) {
		t.Errorf("expected ExpiresAt() = %v, got %v", expected, c.ExpiresAt())
	}
}

func TestReconstructClaim_ClaimedAt_PreservesInput(t *testing.T) {
	t.Parallel()

	// Given
	claimedAt := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := claimedAt.Add(4 * time.Hour)

	// When
	c := domain.ReconstructClaim("somehash", mustID(t), mustAuthor(t, "alice"), claimedAt, expiresAt)

	// Then
	if !c.ClaimedAt().Equal(claimedAt) {
		t.Errorf("expected ClaimedAt() = %v, got %v", claimedAt, c.ClaimedAt())
	}
}

func TestReconstructClaim_ExpiresAt_PreservesInput(t *testing.T) {
	t.Parallel()

	// Given
	claimedAt := time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := claimedAt.Add(4 * time.Hour)

	// When
	c := domain.ReconstructClaim("somehash", mustID(t), mustAuthor(t, "alice"), claimedAt, expiresAt)

	// Then
	if !c.ExpiresAt().Equal(expiresAt) {
		t.Errorf("expected ExpiresAt() = %v, got %v", expiresAt, c.ExpiresAt())
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
