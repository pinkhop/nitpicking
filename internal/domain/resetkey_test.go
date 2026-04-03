package domain_test

import (
	"encoding/hex"
	"testing"

	"github.com/pinkhop/nitpicking/internal/domain"
)

func TestResetKeyGenerate_ReturnsNonEmptyKey(t *testing.T) {
	t.Parallel()

	// When
	key := domain.ResetKeyGenerate()

	// Then
	if key == "" {
		t.Fatal("expected non-empty key")
	}
}

func TestResetKeyGenerate_Returns26CharCrockfordBase32(t *testing.T) {
	t.Parallel()

	// When
	key := domain.ResetKeyGenerate()

	// Then — 26 characters encoding 128 bits of entropy.
	if len(key) != 26 {
		t.Errorf("key length: got %d, want 26", len(key))
	}

	const crockford = "0123456789abcdefghjkmnpqrstvwxyz"
	for i, c := range key {
		found := false
		for _, valid := range crockford {
			if c == valid {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("key[%d] = %c is not a valid Crockford Base32 character", i, c)
		}
	}
}

func TestResetKeyGenerate_ProducesDifferentKeys(t *testing.T) {
	t.Parallel()

	// When
	key1 := domain.ResetKeyGenerate()
	key2 := domain.ResetKeyGenerate()

	// Then
	if key1 == key2 {
		t.Errorf("expected different keys, got same: %q", key1)
	}
}

func TestResetKeyHash_ReturnsSHA512HexOfBinaryKey(t *testing.T) {
	t.Parallel()

	// Given — a generated key.
	key := domain.ResetKeyGenerate()

	// When
	hashHex, err := domain.ResetKeyHash(key)
	// Then — no error, and the hash is a 128-character hex string (512 bits).
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hashHex) != 128 {
		t.Errorf("hash length: got %d, want 128 hex characters", len(hashHex))
	}

	// Verify it's valid hex.
	_, err = hex.DecodeString(hashHex)
	if err != nil {
		t.Errorf("hash is not valid hex: %v", err)
	}
}

func TestResetKeyHash_IsDeterministic(t *testing.T) {
	t.Parallel()

	// Given — a generated key.
	key := domain.ResetKeyGenerate()

	// When
	hash1, err1 := domain.ResetKeyHash(key)
	hash2, err2 := domain.ResetKeyHash(key)

	// Then
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if hash1 != hash2 {
		t.Errorf("same key produced different hashes: %q vs %q", hash1, hash2)
	}
}

func TestResetKeyHash_DifferentKeysProduceDifferentHashes(t *testing.T) {
	t.Parallel()

	// Given
	key1 := domain.ResetKeyGenerate()
	key2 := domain.ResetKeyGenerate()

	// When
	hash1, _ := domain.ResetKeyHash(key1)
	hash2, _ := domain.ResetKeyHash(key2)

	// Then
	if hash1 == hash2 {
		t.Errorf("different keys produced the same hash")
	}
}

func TestResetKeyHash_InvalidInput_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "too short", input: "abc"},
		{name: "too long", input: "00000000000000000000000000000"},
		{name: "invalid characters", input: "abcdefghijklmnopqrstuvwxyz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// When
			_, err := domain.ResetKeyHash(tc.input)

			// Then
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestResetKeyHash_NormalizesCrockfordInput(t *testing.T) {
	t.Parallel()

	// Given — a key and its Crockford-confusable variant (I→1, L→1, O→0).
	key := domain.ResetKeyGenerate()

	// When — hash the key in uppercase (Crockford normalization should handle).
	upper := ""
	for _, c := range key {
		if c >= 'a' && c <= 'z' {
			upper += string(rune(c - 32))
		} else {
			upper += string(c)
		}
	}
	hashLower, err1 := domain.ResetKeyHash(key)
	hashUpper, err2 := domain.ResetKeyHash(upper)

	// Then — both produce the same hash.
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if hashLower != hashUpper {
		t.Errorf("case-insensitive Crockford normalization failed: %q vs %q", hashLower, hashUpper)
	}
}
