# Claim ID Security Model

This document describes the threat model for claim IDs, explains the randomness and hashing choices, and records the rationale for decisions that might otherwise look under-engineered.

---

## What Claim IDs Protect

A claim ID is a bearer token that gates all mutations on an issue. An agent must present the correct claim ID to update fields, transition state, or delete an issue. The claim mechanism prevents two agents from stepping on each other's in-progress work.

Claim IDs do **not** protect against:

- Remote attackers brute-forcing credentials over a network.
- Determined adversaries with physical access to the database file.
- Long-lived credential theft (claim IDs expire via the stale threshold — default 2 hours, maximum 24 hours).

The adversary model is narrow: a co-located agent that might guess or collide with another agent's claim, or a developer mistake that inadvertently leaks a claim ID through command output. That's it.

## Claim ID Generation: `math/rand/v2`, Not `crypto/rand` Directly

Claim IDs are 128-bit random values (16 bytes) hex-encoded to 32 characters.

Starting with Go 1.22, `math/rand/v2` is seeded from the operating system's cryptographic random source by default — it uses the same entropy pool as `crypto/rand`. There is no practical difference in randomness quality between the two for this use case.

We use `math/rand/v2` rather than calling `crypto/rand.Read` directly because:

1. **Simpler API.** `rand.Uint64()` returns a value directly; `crypto/rand.Read` requires a buffer, length management, and explicit error handling for an error that should never occur (and if it does, the system is in a catastrophic state).
2. **Same entropy source.** Since Go 1.22, `math/rand/v2`'s default source is `runtime.fastrand64`, which is seeded from the OS CSPRNG. The output is cryptographically random.
3. **No downgrade risk.** There is no way to accidentally install a weak seed — `math/rand/v2` does not have a global `Seed()` function.

The `#nosec G404` annotations in the source acknowledge the `gosec` lint rule (which predates Go 1.22's CSPRNG-backed `math/rand/v2`) and explain the rationale inline.

## Claim ID Storage: SHA-512, Not bcrypt/argon2/scrypt

> **Note:** SHA-512 hashing of stored claim IDs is planned but not yet implemented. This section documents the rationale for the design decision.

When implemented, the database will store `SHA-512(claim_id)` rather than the plaintext claim ID. Validation will hash the presented claim ID and compare it to the stored hash.

### Why SHA-512 is sufficient

Password-grade algorithms (bcrypt, argon2, scrypt) are designed to be **intentionally slow** — they add computational cost so that an attacker who obtains a hash database cannot feasibly brute-force short, low-entropy passwords (typically 8–20 characters, drawn from a small alphabet).

Claim IDs are none of those things:

| Property | Passwords | Claim IDs |
|----------|-----------|-----------|
| Entropy | ~40–80 bits (user-chosen) | 128 bits (random) |
| Lifetime | Months to years | Hours (stale threshold) |
| Adversary | Remote attacker with hash dump | Co-located agent or developer mistake |
| Brute-force feasibility | Feasible without slow hash | Infeasible at 128 bits regardless of hash speed |

At 128 bits of entropy, a brute-force search requires on the order of 2^128 attempts. Even with a fast hash like SHA-512, this is computationally infeasible — the hash algorithm's speed is irrelevant when the search space is this large.

### Why bcrypt/argon2 would be harmful

A password-grade hash adds 100–500 ms of intentional latency per evaluation. Every claim validation — `json update`, `form update`, `close`, `issue defer`, `issue delete`, `label add`, `label remove` — would pay this cost. For an AI agent workflow where dozens of mutations happen in rapid succession, this latency would be a meaningful performance regression with zero security benefit.

## Output Redaction

Claim IDs are bearer tokens and must not appear in any command output except the `claim` command itself (which returns the token to the agent that created it). All other commands — `show`, `list`, `history`, `doctor`, `graph`, `epic status`, `epic children` — must either omit the claim ID entirely or display a redacted placeholder.

This prevents an unauthorized agent from extracting a claim ID by running a read-only command and then using it to mutate another agent's claimed issue.

## Summary of Design Choices

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Random source | `math/rand/v2` | CSPRNG-backed since Go 1.22; simpler API than `crypto/rand` |
| Entropy | 128 bits | Matches UUID-level uniqueness; brute-force infeasible |
| Encoding | Hex (32 chars) | Simple, deterministic length, no ambiguous characters |
| Storage hash | SHA-512 (planned) | Fast; 128-bit input makes slow hashing unnecessary |
| Not bcrypt/argon2 | Intentional | 100–500 ms latency per validation; no security benefit at 128-bit entropy |
| Output redaction | All non-claim commands | Bearer tokens must not leak through read-only commands |
