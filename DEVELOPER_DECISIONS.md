# Nitpicking — Developer Decisions

> Records design decisions made during implementation that are not covered by the specification.

---

## DD-001: Priority zero value vs default

**Decision:** Priority is an `int` type starting at `iota`. P0 = 0, which is also Go's zero value. The `NewTask`/`NewEpic` constructors treat a zero Priority as "not specified" and default it to P2. This means callers must explicitly pass `P0` to get the highest urgency, and any struct that includes a Priority field naturally defaults to P2 through the constructor.

**Why:** Using `iota` (starting at 0) maps naturally to "lower number = higher urgency" from the spec. The constructor-level defaulting avoids the need for an `Option` pattern or sentinel value.

---

## DD-002: Claim IDs use 128-bit hex tokens from the crypto PRNG

**Decision:** Claim IDs are 16 random bytes hex-encoded (32 characters), generated from the default `math/rand/v2` source (backed by `crypto/rand` in Go 1.22+). This provides 128 bits of cryptographic entropy — equivalent to UUIDs — making them unguessable per the spec's "bearer authentication" requirement.

**Why:** The spec requires "random, unguessable token". Claim IDs are bearer authentication tokens — presenting the claim ID is the sole proof of ownership — so they must be cryptographically unpredictable. UUID format would work but adds a dependency; raw hex from the crypto-backed default source is simpler and equally secure. See DD-010 for the contrasting treatment of issue IDs and agent names.

---

## DD-003: DimensionSet uses value semantics (copy-on-write)

**Decision:** DimensionSet is an immutable value type. All mutation methods (Set, Remove) return a new DimensionSet with a cloned underlying map. The Issue type stores a DimensionSet by value.

**Why:** The CLAUDE.md coding standards require immutable structures for concurrent access. Copy-on-write maps are simple and correct, and dimension sets are small (typically under 10 entries).

---

## DD-004: Agent name format is adjective-noun-modifier

**Decision:** Agent names use three components (`adjective-noun-modifier`) instead of Docker's two-component (`adjective-noun`) format. This gives a name space of ~50 * 60 * 60 = 180,000 combinations versus Docker's ~3,300.

**Why:** The spec says "Docker-style" but collision probability with two components is high in multi-agent scenarios. Three components keep the style while reducing collisions.

---

## DD-005: Issue entity stores no revision or author

**Decision:** Per the spec (§4.1), `revision = history count − 1` and `author` is derived from the most recent history entry. The Issue struct omits both fields. They are computed at read time from the history.

**Why:** Storing derived values invites inconsistency. The history is the source of truth.

---

## DD-006: State machine uses lookup tables, not methods on states

**Decision:** State transitions are validated by `TransitionTask`/`TransitionEpic` functions that consult a `map[State]map[State]bool` lookup table, rather than methods on each state type.

**Why:** A lookup table is explicit, readable, and easy to audit against the specification's transition rules. It avoids the complexity of polymorphic dispatch for a small, fixed state machine.

---

## DD-007: Deleted is a flag, not a state

**Decision:** Per the spec notes on 1.7 and 1.8, `deleted` is a separate boolean flag on the Issue struct, not a value in the state machine. The state machine only knows about lifecycle states (open, active, claimed, closed, deferred, waiting).

**Why:** The spec explicitly says "deleted is terminal but separate from the state machine." Keeping it as a flag simplifies the state machine and makes the terminal-check logic clearer.

---

## DD-008: FTS5 uses standalone tables, not external content

**Decision:** The FTS5 virtual tables (`issues_fts`, `notes_fts`) are standalone (not backed by `content=issues`). Sync is managed manually in the repository layer via INSERT/DELETE on create and update operations.

**Why:** The issues table is `WITHOUT ROWID` (per §4.7), which means it has no implicit `rowid` column. FTS5's external content mode (`content=`) requires a `content_rowid=` mapping to an integer column, but `WITHOUT ROWID` tables lack this. Standalone FTS tables avoid this incompatibility at the cost of slightly more storage and manual sync.

---

## DD-009: Factory provides the database connection, not the application service

**Decision:** The `Factory.Store` field is a `func() (*sqlite.Store, error)` closure that lazily discovers the database and opens the SQLite connection on first access. The Factory provides the architecture-neutral database connection; the application service is constructed from it by `cmdutil.NewTracker(f)`, which commands call when they need the use-case layer.

**Why:** Per the design guide, Factory fields should be infrastructure-level dependencies (database connections, HTTP clients), not application-layer constructs. This separates configuration-driven plumbing from business logic. Database discovery (walking up directories) is a side-effectful operation that should not happen at factory construction time. The `NewTracker` helper avoids boilerplate — every command that needs the service calls it, but the Factory itself remains architecture-neutral.

---

## DD-010: Issue IDs and agent names use PCG; claim IDs use the crypto PRNG

**Decision:** Issue ID generation and agent name generation use explicit PCG generators (`rand.New(rand.NewPCG(...))`) seeded from the crypto source at init time. Claim ID generation uses the default `math/rand/v2` source, which is backed by `crypto/rand`.

**Why:** Issue IDs need collision resistance across a ~33 million ID space, and agent names need variety across ~180,000 combinations — neither requires cryptographic unpredictability. PCG is a high-quality, fast PRNG that is more than sufficient for both. Claim IDs, by contrast, are bearer authentication tokens (presenting the claim ID is the sole proof of ownership), so they must be cryptographically unpredictable. Seeding the PCG generators from the crypto source at init time ensures sequences are unpredictable across process restarts without paying the crypto overhead on every call.
