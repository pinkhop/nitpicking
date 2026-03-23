package claim

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/pinkhop/nitpicking/internal/domain/identity"
	"github.com/pinkhop/nitpicking/internal/domain/ticket"
)

// DefaultStaleThreshold is the default duration after which a claim becomes
// stale and eligible for stealing.
const DefaultStaleThreshold = 2 * time.Hour

// MaxStaleThreshold is the maximum allowed stale threshold.
const MaxStaleThreshold = 24 * time.Hour

// claimIDBytes is the number of random bytes used to generate a claim ID.
// 16 bytes = 128 bits of entropy, matching UUID-level uniqueness.
const claimIDBytes = 16

// Claim represents active ownership of a ticket. Claims are immutable value
// objects — "extending" or "updating last activity" produces a new Claim.
type Claim struct {
	id             string
	ticketID       ticket.ID
	author         identity.Author
	staleThreshold time.Duration
	lastActivity   time.Time
}

// NewClaimParams holds the parameters for creating a new claim.
type NewClaimParams struct {
	TicketID       ticket.ID
	Author         identity.Author
	StaleThreshold time.Duration
	Now            time.Time
}

// NewClaim creates a new claim with a randomly generated claim ID.
func NewClaim(p NewClaimParams) (Claim, error) {
	if p.TicketID.IsZero() {
		return Claim{}, fmt.Errorf("claim requires a ticket ID")
	}
	if p.Author.IsZero() {
		return Claim{}, fmt.Errorf("claim requires an author")
	}

	threshold := p.StaleThreshold
	if threshold == 0 {
		threshold = DefaultStaleThreshold
	}
	if threshold > MaxStaleThreshold {
		return Claim{}, fmt.Errorf("stale threshold %v exceeds maximum %v", threshold, MaxStaleThreshold)
	}

	claimID := generateClaimID()

	now := p.Now
	if now.IsZero() {
		now = time.Now()
	}

	return Claim{
		id:             claimID,
		ticketID:       p.TicketID,
		author:         p.Author,
		staleThreshold: threshold,
		lastActivity:   now,
	}, nil
}

// ReconstructClaim rebuilds a Claim from persisted data without generating
// a new ID. Used by the storage layer when loading claims from the database.
func ReconstructClaim(id string, ticketID ticket.ID, author identity.Author, staleThreshold time.Duration, lastActivity time.Time) Claim {
	return Claim{
		id:             id,
		ticketID:       ticketID,
		author:         author,
		staleThreshold: staleThreshold,
		lastActivity:   lastActivity,
	}
}

// ID returns the random, unguessable claim identifier.
func (c Claim) ID() string { return c.id }

// TicketID returns the ID of the claimed ticket.
func (c Claim) TicketID() ticket.ID { return c.ticketID }

// Author returns the author who holds the claim.
func (c Claim) Author() identity.Author { return c.author }

// StaleThreshold returns the duration after which this claim becomes stale.
func (c Claim) StaleThreshold() time.Duration { return c.staleThreshold }

// LastActivity returns the timestamp of the most recent activity on this claim.
func (c Claim) LastActivity() time.Time { return c.lastActivity }

// IsStale reports whether the claim is stale at the given time.
func (c Claim) IsStale(now time.Time) bool {
	return now.Sub(c.lastActivity) > c.staleThreshold
}

// StaleAt returns the timestamp at which this claim becomes stale.
func (c Claim) StaleAt() time.Time {
	return c.lastActivity.Add(c.staleThreshold)
}

// WithLastActivity returns a new Claim with the last activity timestamp
// updated.
func (c Claim) WithLastActivity(t time.Time) Claim {
	c.lastActivity = t
	return c
}

// WithStaleThreshold returns a new Claim with the stale threshold updated.
// Returns an error if the new threshold exceeds the maximum.
func (c Claim) WithStaleThreshold(d time.Duration) (Claim, error) {
	if d > MaxStaleThreshold {
		return Claim{}, fmt.Errorf("stale threshold %v exceeds maximum %v", d, MaxStaleThreshold)
	}
	c.staleThreshold = d
	return c, nil
}

// generateClaimID produces a cryptographically random hex-encoded claim ID.
// Uses math/rand/v2 which is backed by crypto/rand by default in Go 1.22+.
// Panics on CSPRNG failure — the correct behavior since a broken random
// source should halt the process.
func generateClaimID() string {
	var buf [claimIDBytes]byte
	hi := rand.Uint64()
	lo := rand.Uint64()
	binary.BigEndian.PutUint64(buf[:8], hi)
	binary.BigEndian.PutUint64(buf[8:], lo)
	return hex.EncodeToString(buf[:])
}
