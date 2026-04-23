package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/rand/v2"
)

// ErrEmptyAgentNameSeed is returned by GenerateAgentNameFromSeed when it is
// called with an empty seed. An empty seed almost always indicates an
// unexpanded shell variable (e.g. `--seed=$PPID` in an environment where PPID
// is unset) rather than a deliberate choice; returning an error surfaces the
// misconfiguration at the point it originates instead of silently producing a
// stable but useless name shared by every affected caller.
var ErrEmptyAgentNameSeed = errors.New("agent name seed must not be empty")

// agentNamePRNG is a PCG-backed random number generator for agent name generation.
// Seeded from the default crypto source at init time for unpredictable
// sequences, but uses PCG for speed — agent names need variety, not
// cryptographic security.
var agentNamePRNG = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) // #nosec G404 -- agent names need variety, not cryptographic security; PCG seeded from crypto/rand

// adjectives is a curated list of positive, non-offensive adjectives for
// Docker-style agent name generation.
var adjectives = []string{
	"bold", "brave", "bright", "calm", "clever",
	"cosmic", "crisp", "dashing", "eager", "electric",
	"epic", "fair", "fancy", "fast", "fierce",
	"fleet", "fresh", "gentle", "glad", "golden",
	"grand", "happy", "hardy", "keen", "kind",
	"light", "lively", "lucky", "merry", "mighty",
	"noble", "plucky", "proud", "quick", "quiet",
	"rapid", "ready", "sharp", "shiny", "sleek",
	"smart", "snappy", "solar", "sonic", "steady",
	"sunny", "super", "sweet", "swift", "vivid",
}

// nouns is a curated list of concrete, non-offensive nouns for name generation.
var nouns = []string{
	"anchor", "arrow", "beacon", "bloom", "bolt",
	"bridge", "brook", "canyon", "cedar", "comet",
	"coral", "crane", "creek", "crest", "crown",
	"delta", "drift", "ember", "falcon", "fern",
	"flame", "flint", "forest", "frost", "gale",
	"garden", "glacier", "glider", "grove", "harbor",
	"hawk", "heron", "jade", "lake", "lantern",
	"lark", "maple", "meadow", "moss", "oak",
	"orbit", "otter", "peak", "pine", "pixel",
	"quartz", "rain", "reef", "ridge", "river",
	"robin", "sage", "spark", "spruce", "star",
	"stone", "storm", "stream", "summit", "timber",
	"trail", "valley", "wave", "willow", "wind",
}

// modifiers adds a third component for extra uniqueness.
var modifiers = []string{
	"alpha", "beta", "blaze", "bloom", "breeze",
	"burst", "byte", "charm", "chase", "cipher",
	"clash", "core", "craft", "crystal", "dash",
	"dawn", "deep", "drift", "echo", "edge",
	"flash", "flow", "flux", "forge", "frost",
	"glow", "glint", "haven", "haze", "jump",
	"light", "link", "loop", "nova", "path",
	"phase", "plume", "prime", "pulse", "quest",
	"ray", "rise", "rush", "sail", "scout",
	"shade", "shift", "shine", "skip", "snap",
	"spark", "spire", "spray", "spring", "stride",
	"surge", "trace", "vault", "vibe", "whirl",
}

// GenerateAgentName produces a Docker-style random agent name in the format
// "adjective-noun-modifier" (e.g., "dashing-storage-glitter"). Each
// invocation returns a fresh random name using the package-level PCG generator.
// The result is not reproducible across calls; use GenerateAgentNameFromSeed
// when determinism is required.
func GenerateAgentName() string {
	adj := randomElement(agentNamePRNG, adjectives)
	noun := randomElement(agentNamePRNG, nouns)
	mod := randomElement(agentNamePRNG, modifiers)
	return adj + "-" + noun + "-" + mod
}

// GenerateAgentNameFromSeed produces a Docker-style deterministic agent name
// in the format "adjective-noun-modifier" derived from seed. The same seed
// value always yields the same name; distinct seeds overwhelmingly produce
// distinct names.
//
// An empty seed returns ErrEmptyAgentNameSeed. Empty seeds almost always
// indicate an unexpanded shell variable rather than a deliberate choice, and
// silently producing a name would make that misconfiguration very hard to
// diagnose.
//
// The seed bytes are passed through SHA-256 to derive two uint64 values that
// seed a PCG generator. Using SHA-256 as a KDF (rather than parsing the seed
// string directly as an integer) ensures that the full entropy of any-length
// seed is mixed uniformly into the PCG state and that the derivation is
// well-defined across all non-empty seed inputs.
func GenerateAgentNameFromSeed(seed string) (string, error) {
	if seed == "" {
		return "", ErrEmptyAgentNameSeed
	}

	s1, s2 := seedFromString(seed)
	rng := rand.New(rand.NewPCG(s1, s2)) // #nosec G404 -- agent names need variety, not cryptographic security; PCG seeded from SHA-256 KDF over the caller-supplied seed

	adj := randomElement(rng, adjectives)
	noun := randomElement(rng, nouns)
	mod := randomElement(rng, modifiers)
	return adj + "-" + noun + "-" + mod, nil
}

// seedFromString derives two uint64 PCG seed values from an arbitrary string
// using SHA-256 as a KDF. The 32-byte digest is split into two 8-byte
// big-endian halves, giving independent high-entropy seeds regardless of the
// length or content of the input.
func seedFromString(s string) (s1, s2 uint64) {
	digest := sha256.Sum256([]byte(s))
	s1 = binary.BigEndian.Uint64(digest[:8])
	s2 = binary.BigEndian.Uint64(digest[8:16])
	return s1, s2
}

// randomElement returns a random element from a string slice using the
// provided random number generator.
func randomElement(rng *rand.Rand, slice []string) string {
	idx := rng.IntN(len(slice))
	return slice[idx]
}
