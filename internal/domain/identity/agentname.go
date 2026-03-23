package identity

import "math/rand/v2"

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
// invocation returns a fresh random name. Uses math/rand/v2 which is backed
// by crypto/rand by default in Go 1.22+.
func GenerateAgentName() string {
	adj := randomElement(adjectives)
	noun := randomElement(nouns)
	mod := randomElement(modifiers)
	return adj + "-" + noun + "-" + mod
}

// randomElement returns a cryptographically random element from a string slice.
func randomElement(slice []string) string {
	return slice[rand.N(len(slice))]
}
