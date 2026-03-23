package identity

import "math/rand/v2"

// prng is a PCG-backed random number generator for agent name generation.
// Seeded from the default crypto source at init time for unpredictable
// sequences, but uses PCG for speed — agent names need variety, not
// cryptographic security.
var prng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))

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
func GenerateAgentName() string {
	adj := randomElement(adjectives)
	noun := randomElement(nouns)
	mod := randomElement(modifiers)
	return adj + "-" + noun + "-" + mod
}

// randomElement returns a random element from a string slice using the
// package-level PCG generator.
func randomElement(slice []string) string {
	idx := prng.IntN(len(slice))
	return slice[idx]
}
