# Nitpicking — Project Plan

> Derived from `SPECIFICATION.md` v1.0 (2026-03-23)
> Single senior engineer; serial execution; inside-out development.

---

## Starting Point

The repo contains a Go quick-start template with:

- CLI framework (`urfave/cli/v3`) wired into a root command with a `version` subcommand.
- Factory pattern for dependency injection (`cmdutil.Factory`).
- IOStreams abstraction (TTY detection, color, test doubles).
- Exit codes, structured logging, signal handling, Makefile with build/test/lint/sec targets.
- No domain code, no persistence, no application-layer logic.

### Technology Decisions

- **SQLite driver:** `modernc.org/sqlite` (pure Go, no CGO). This satisfies the `CGO_ENABLED=0` static binary requirement in the Makefile.

---

## Phase 1 — Core Domain Model

Pure domain types, value objects, and validation. No ports, no persistence. Every task produces unit-tested code with in-memory construction only.

### 1.1 Domain Error Types

Define typed errors for the domain: `ErrIssueNotFound`, `ErrClaimConflict`, `ErrIllegalTransition`, `ErrValidation`, `ErrCycleDetected`, `ErrDeletedIssue`, `ErrTerminalState`. These map to exit codes at the adapter layer, not here.

`ErrClaimConflict` must carry structured context: the current holder's author and the timestamp at which the claim becomes stale. This enables self-describing error responses per §9.

- **Depends on:** nothing
- **Package:** `internal/domain`

### 1.2 Issue ID Value Object

Define the `IssueID` type: uppercase prefix (1–10 chars, A–Z), separator `-`, 5 lowercase Crockford Base32 random characters. Implement parsing, validation, generation (with collision retry callback), and `String()`. No database interaction — collision detection is injected.

- **Depends on:** nothing
- **Package:** `internal/domain/issue`

### 1.3 Author Value Object

Define the `Author` type with validation: at least one alphanumeric char, max 64 Unicode runes, no whitespace, NFC-normalized on input, case-sensitive equality.

- **Depends on:** nothing
- **Package:** `internal/domain/identity`

### 1.4 Priority Value Object

Define `Priority` as an enumeration: `P0`–`P4`. Include ordering (lower number = higher urgency), parsing from string, default (`P2`), and `String()`.

- **Depends on:** nothing
- **Package:** `internal/domain/issue`

### 1.5 Issue Role Enumeration

Define `Role` as `Epic` or `Task`. Immutable after creation. Include parsing and `String()`.

- **Depends on:** nothing
- **Package:** `internal/domain/issue`

### 1.6 Dimension Value Object

Define `Dimension` (key–value pair). Key: 1–64 bytes, ASCII printable (0x21–0x7E), no whitespace, at least one alphanumeric. Value: 1–256 bytes, UTF-8, no whitespace, at least one alphanumeric. Key uniqueness is enforced at the collection level. Include a `DimensionSet` collection type.

- **Depends on:** nothing
- **Package:** `internal/domain/issue`

### 1.7 Task State Machine

Define `TaskState` enum (`open`, `claimed`, `closed`, `deferred`, `waiting`) and a transition function that enforces legal transitions per §5.1 and §5.3. `closed` is terminal. Return typed errors (from 1.1) for illegal transitions.

Note: `deleted` is also terminal (§5.3) but is not a state in the state machine — it is a separate flag checked independently by claiming rules (1.15) and soft deletion rules (1.19). The state machine must not accept transitions from or to `deleted`.

- **Depends on:** 1.1
- **Package:** `internal/domain/issue`

### 1.8 Epic State Machine

Define `EpicState` enum (`active`, `claimed`, `deferred`, `waiting`) and a transition function per §5.2 and §5.3. No `closed` state — completion is derived. Return typed errors (from 1.1) for illegal transitions.

Note: as with tasks, `deleted` is terminal but separate from the state machine. See note on 1.7.

- **Depends on:** 1.1
- **Package:** `internal/domain/issue`

### 1.9 Issue Entity — Core Fields

Define the `Issue` struct carrying all common fields from §4.1: ID, role, title, description, acceptance criteria, priority, state, parent reference, dimensions, created-at, idempotency key. All fields are immutable after construction; "mutations" return new `Issue` values. Include factory functions `NewTask` and `NewEpic` with required-field validation (title must contain at least one alphanumeric character).

**Revision** is a derived property — computed as `history entry count − 1` per §4.1. It is not stored on the Issue struct. It is computed at read time from the history.

**Author** (the last person to update the issue) is derived from the most recent history entry. Creation generates a history entry, so every issue has an author from inception.

- **Depends on:** 1.2, 1.4, 1.5, 1.6, 1.7, 1.8
- **Package:** `internal/domain/issue`

### 1.10 Parent Constraints

Implement parent validation rules from §4.1.1: any issue role can be a parent; an issue cannot be its own parent; deleted issues cannot be parents. Cycle detection (A cannot parent B if B is an ancestor of A) is defined as a pure function taking an ancestry-lookup callback — no persistence dependency.

- **Depends on:** 1.9
- **Package:** `internal/domain/issue`

### 1.11 Note Entity

Define `Note` with fields from §4.2: auto-assigned sequential ID (displayed as `note-<int>`), issue ID reference, author, created-at, body (non-empty). Comments are immutable after creation.

- **Depends on:** 1.2, 1.3
- **Package:** `internal/domain/note`

### 1.12 Relationship Value Object

Define `Relationship` with types from §4.3: `blocked_by`/`blocks`, `cites`/`cited_by`. Include the inverse mapping. Enforce: no self-relationships. Idempotent semantics are a storage concern, but the domain type validates structural rules. A relationship cannot reference a deleted issue (validated via callback).

- **Depends on:** 1.2
- **Package:** `internal/domain/issue`

### 1.13 History Entry Entity

Define `HistoryEntry` per §4.4 and §7: entry ID, issue ID, zero-based revision, author, timestamp, event type enum (`created`, `claimed`, `released`, `updated`, `state_changed`, `deleted`, `relationship_added`, `relationship_removed`), structured changes (field name → before/after). Immutable. Comments do not produce history entries.

- **Depends on:** 1.2, 1.3
- **Package:** `internal/domain/history`

### 1.14 Claim Entity

Define `Claim` per §4.5: random unguessable claim ID, issue ID, author, stale threshold (default 2h, max 24h), last-activity timestamp. Include `IsStale(now)` method. Claims are immutable value objects — "extending" produces a new claim.

- **Depends on:** 1.2, 1.3
- **Package:** `internal/domain/claim`

### 1.15 Claiming Rules

Implement claiming business logic from §6.1: only non-terminal issues can be claimed; `closed` and `deleted` issues cannot be claimed (`deleted` is checked as a separate flag, not via the state machine). One-shot claim-update-release as a domain operation. Author attribution rules: gated operations inherit claim author; ungated operations require explicit author.

- **Depends on:** 1.9, 1.14
- **Package:** `internal/domain/claim`

### 1.16 Epic Completion Derivation

Implement §6.2: an epic is complete when it has children AND all children are closed (tasks) or complete (sub-epics). Incomplete otherwise, including when childless. Pure function taking a list of child statuses.

- **Depends on:** 1.7, 1.8
- **Package:** `internal/domain/issue`

### 1.17 Readiness Rules

Implement §6.3: task readiness and epic readiness as pure functions taking lookup callbacks for blockers and ancestors.

Task readiness requires: state `open`; no unresolved `blocked_by` (a `blocked_by` target that has been **closed or deleted** counts as resolved); no ancestor epic in `deferred` or `waiting`.

Epic readiness requires: state `active`; no children (needs decomposition); no unresolved `blocked_by` (same resolution rule — closed or deleted targets are resolved); no ancestor epic in `deferred` or `waiting`.

Readiness propagation: deferred/waiting epics suppress readiness for unclaimed descendants.

- **Depends on:** 1.7, 1.8, 1.12
- **Package:** `internal/domain/issue`

### 1.18 Staleness and Stealing Rules

Implement §6.4: a claim is stale when `now - last_activity > stale_threshold`. Stealing atomicity rules are defined here as preconditions (old claim must be stale, new claim replaces it). Auto-generated note on steal. Threshold extension rules (cannot exceed 24h).

- **Depends on:** 1.14, 1.11
- **Package:** `internal/domain/claim`

### 1.19 Soft Deletion Rules

Implement §6.5: deleted issue's ID is reserved; deleted issues are immutable; cannot be parents or relationship targets; existing relationships retained but invisible. Recursive epic deletion: all unclaimed descendants are deleted; if any descendant is claimed, operation fails with conflict error identifying the claimed issue(s). Pure function returning the set of issues to delete or the conflict error.

- **Depends on:** 1.9, 1.14
- **Package:** `internal/domain/issue`

### 1.20 Agent Name Generator

Implement Docker-style random name generation (e.g., "dashing-storage-glitter"). Pure function, no persistence.

- **Depends on:** nothing
- **Package:** `internal/domain/identity`

### 1.21 Agent Instructions Generator

Implement Markdown block generation per §8.2: core workflow, claim IDs, state transitions, help pointers. Pure string construction.

- **Depends on:** nothing
- **Package:** `internal/domain/identity`

---

## Phase 2 — Port Interfaces

Define the contracts between the core domain and its adapters. No implementations. The exact shape of these interfaces is expected to emerge as the application services are built; the initial design is a starting point, not a commitment.

### 2.1 Driven Port — Persistence Interface

Define the `Repository` interface (or set of interfaces) that the application layer requires from storage. Methods should cover:

- Issue CRUD (create, get by ID, update, soft delete, list, search)
- Note CRUD (create, get by ID, list by issue, search per-issue, search global)
- Claim lifecycle (create, get by issue, invalidate, update last-activity)
- Relationship CRUD (create-if-not-exists, delete-if-exists, list by issue)
- History (append entry, list by issue)
- Ancestry lookup (for cycle detection and readiness propagation)
- GC (physical removal of deleted/closed data)
- Database initialization (prefix storage)
- Issue ID collision check

Pagination parameters: keyset-based, page size default 20, total count.

- **Depends on:** all Phase 1 types (1.1–1.19)
- **Package:** `internal/domain/port`

### 2.2 Driving Port — Application Service Interface

Define the use-case boundary — the set of operations the CLI (or any driving adapter) can invoke. This is a facade over domain logic + persistence. Operations map to §8 commands:

- Init, AgentName, AgentInstructions
- Create, ClaimByID, ClaimNextReady, OneShotUpdate, Update, ExtendStaleThreshold, TransitionState, Delete, Show, List, Search
- AddRelationship, RemoveRelationship
- AddNote, ShowNote, ListNotes, SearchNotes (per-issue), SearchNotes (global)
- ShowHistory
- Doctor, GC

Each operation defines its input DTO and output DTO. Errors use domain error types.

- **Depends on:** 2.1, all Phase 1 types
- **Package:** `internal/app/service` (or `internal/usecase`)

---

## Phase 3 — Application Service Implementation

Implement the driving port. This layer orchestrates domain logic and persistence calls within transactions. Tested with in-memory fakes for the persistence port.

### 3.1 In-Memory Fake Repository

Implement the persistence port interface (§2.1) as an in-memory fake. This is the test double for all application-layer unit tests. Stores issues, notes, claims, relationships, and history in maps/slices. Supports pagination, filtering, and keyset semantics.

- **Depends on:** 2.1
- **Package:** `internal/fake`

### 3.2 Initialize Service

Implement database initialization: validate and store the prefix. In the application layer, this translates to calling the persistence port's init method.

- **Depends on:** 2.2, 3.1
- **Package:** `internal/app/service`

### 3.3 Create Issue Service

Implement issue creation: validate all fields, generate issue ID (with collision retry via persistence port), set defaults (priority P2, state open/active), optionally start as claimed, optionally attach relationships at creation time (per §8.3 Create), handle idempotency key. Produce a `created` history entry.

- **Depends on:** 3.1, 3.2
- **Package:** `internal/app/service`

### 3.4 Claim Services (ClaimByID, ClaimNextReady)

Implement claiming: validate issue is claimable (not terminal, not already claimed unless stale + steal opt-in), generate claim ID, bind author, set stale threshold. `ClaimNextReady` uses readiness rules and priority ordering. Produce `claimed` history entry. Handle steal fallback with auto-comment.

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.5 Update Issue Service

Implement claimed update: verify claim ID matches, apply field changes (title, description, acceptance criteria, priority, parent, dimensions), validate parent constraints including cycle detection, produce `updated` history entry. Update claim's last-activity.

When the parent is changed or removed, recalculate the **old** parent's epic completion status (per §4.1.1).

Optionally add a comment in the same atomic operation (per §8.3 Update). The note follows normal note rules (author, body validation) and updates claim last-activity.

- **Depends on:** 3.4
- **Package:** `internal/app/service`

### 3.6 One-Shot Update Service

Implement atomic claim → update → release for unclaimed issues. Transient claim; caller provides author. Wraps 3.4 + 3.5 + state transition to release.

- **Depends on:** 3.5
- **Package:** `internal/app/service`

### 3.7 State Transition Service

Implement `TransitionState`: verify claim, apply state machine transition (release, close, defer, wait), invalidate claim, produce `state_changed` history entry. Closing recalculates parent epic completion.

- **Depends on:** 3.4
- **Package:** `internal/app/service`

### 3.8 Extend Stale Threshold Service

Implement threshold extension: verify claim, validate new threshold (≤24h), update claim.

- **Depends on:** 3.4
- **Package:** `internal/app/service`

### 3.9 Delete Issue Service

Implement soft deletion: verify claim, recursive epic deletion (collect unclaimed descendants, fail if any are claimed), mark as deleted, produce `deleted` history entries.

- **Depends on:** 3.4
- **Package:** `internal/app/service`

### 3.10 Note Services (Add, Show, List, Search)

Implement note operations: add note (no claim required, validate author and body, update claim last-activity if issue is claimed), show by ID, list by issue with filters (author, created-after, after-note-ID), search per-issue, search global with filters.

Enforcement rules: notes **cannot** be added to deleted issues (deleted issues are immutable per §4.2). Comments **can** be added to closed issues — closure is terminal for state changes, not for commentary.

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.11 Relationship Services

Implement relationship add/remove per §8.4: no claim required, requires explicit author, validate no self-reference, validate target is not deleted, idempotent semantics (create-if-not-exists, delete-if-exists), produce history entry on source issue.

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.12 Show / List / Search Issue Services

Implement read operations: show (full issue state including derived readiness, epic completion, revision, and author — the latter two derived from history), list (with filters: role, state, ready predicate, parent, dimension; ordering: priority, created, modified; keyset pagination), search (FTS on title/description/acceptance criteria, optionally notes).

Dimension filtering supports exact key–value match (`key:value`), wildcard (`key:*`), and negative matching (e.g., exclude issues with a given dimension value).

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.13 History Service

Implement `ShowHistory`: list history entries for an issue with filters (author, date range), ordering, and pagination.

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.14 Doctor Service

Implement diagnostics from §8.7: detect circular `blocked_by` chains, deadlocked state (all issues blocked), stale claims, epics with no task descendants, GC opportunity (40% space savings threshold). Read-only; returns findings.

- **Depends on:** 3.1, 3.3
- **Package:** `internal/app/service`

### 3.15 GC Service

Implement garbage collection: physically remove deleted (and optionally closed) issue data via persistence port.

- **Depends on:** 3.1
- **Package:** `internal/app/service`

---

## Phase 4 — Driven Adapter (SQLite)

Implement the persistence port against SQLite using `modernc.org/sqlite` (pure Go, CGO-free). Integration-tested.

### 4.1 Database Discovery

Implement `.np/` directory walk from §3.3: start at cwd, walk up to filesystem root, skip permission/sandbox errors. Return path or "no database found" error.

- **Depends on:** nothing (standalone utility)
- **Package:** `internal/storage/sqlite`

### 4.2 SQLite Schema and Migrations

Design and implement the SQLite schema: issues (`WITHOUT ROWID`), notes, claims, relationships, history, dimensions, FTS virtual tables for search, prefix storage. Include initialization (create `.np/` dir + database + schema).

- **Depends on:** 2.1, 4.1
- **Package:** `internal/storage/sqlite`

### 4.3 SQLite Repository — Issue CRUD

Implement create, get-by-ID, update, soft-delete, list (with all filters including dimension negative matching and keyset pagination), search (FTS).

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

### 4.4 SQLite Repository — Note CRUD

Implement note create, get-by-ID, list-by-issue, search per-issue (FTS), search global (FTS).

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

### 4.5 SQLite Repository — Claim Lifecycle

Implement claim create, get-by-issue, invalidate, update-last-activity. Atomic compare-and-swap for claiming and stealing.

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

### 4.6 SQLite Repository — Relationships, History, Ancestry

Implement relationship create-if-not-exists, delete-if-exists, list-by-issue. History append and list. Ancestry lookup for cycle detection and readiness propagation.

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

### 4.7 SQLite Repository — GC

Implement physical removal of deleted (and optionally closed) data, including related notes, history, relationships, dimensions, and FTS entries. Run `VACUUM` after.

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

### 4.8 Transaction Management

Implement transaction wrapper ensuring all write operations are atomic. The application service calls persistence methods within a transaction scope provided by the adapter.

- **Depends on:** 4.2
- **Package:** `internal/storage/sqlite`

---

## Phase 5 — Driving Adapter (CLI)

Wire CLI commands to the application service. Each command is a thin adapter: parse flags/args, call the service, format output (JSON or human-readable). Phase 4 must be complete before Phase 5 begins — CLI commands require the SQLite adapter for production wiring, and several commands (e.g., `np init`) depend directly on it.

### 5.1 Exit Code Mapping

Update `exit_codes.go` to match §9 exit codes: 0 success, 1 general error, 2 not found, 3 claim conflict, 4 validation error, 5 database error. Update `classifyError` to map domain errors to these codes.

- **Depends on:** 1.1
- **Package:** `internal/app`

### 5.2 JSON Output Infrastructure

Implement a shared output formatter: `--json` flag on every command, deterministic output shape (empty list → `[]`, missing optional → null, never omitted), self-describing errors in JSON (including structured claim-conflict context from `ErrClaimConflict`).

- **Depends on:** nothing
- **Package:** `internal/cmdutil`

### 5.3 `np init` Command

CLI adapter for §8.2 Initialize: `np init <PREFIX>`. Creates `.np/` directory and database in cwd.

- **Depends on:** 3.2, 4.2, 5.1, 5.2
- **Package:** `internal/cmd/init`

### 5.4 `np agent name` Command

CLI adapter for agent name generation.

- **Depends on:** 1.20, 5.2
- **Package:** `internal/cmd/agent`

### 5.5 `np agent prime` Command

CLI adapter for agent instructions output.

- **Depends on:** 1.21, 5.2
- **Package:** `internal/cmd/agent`

### 5.6 `np create` Command

CLI adapter for issue creation. Flags: `--title`, `--description`, `--acceptance-criteria`, `--priority`, `--role`, `--parent`, `--dimension` (repeatable key=value), `--relationship` (repeatable type:id), `--claim`, `--author`, `--idempotency-key`.

- **Depends on:** 3.3, 5.1, 5.2
- **Package:** `internal/cmd/create`

### 5.7 `np claim id` Subcommand

CLI adapter for `ClaimByID`. Flags: `--steal`, `--author`, `--stale-threshold`.

- **Depends on:** 3.4, 5.1, 5.2
- **Package:** `internal/cmd/claim`

### 5.8 `np claim ready` Subcommand

CLI adapter for `ClaimNextReady`. Flags: `--dimension` (filter), `--role` (filter), `--steal-if-needed`, `--author`, `--stale-threshold`.

- **Depends on:** 3.4, 5.1, 5.2
- **Package:** `internal/cmd/claim`

### 5.9 `np update` Command

CLI adapter for claimed update. Flags for each mutable field, `--claim` required, `--dimension-set`, `--dimension-remove`, `--relationship-add`, `--relationship-remove`, `--note`.

- **Depends on:** 3.5, 5.1, 5.2
- **Package:** `internal/cmd/update`

### 5.10 `np edit` Command

CLI adapter for one-shot update. Same field flags as `np update` but no `--claim`; requires `--author`.

- **Depends on:** 3.6, 5.1, 5.2
- **Package:** `internal/cmd/edit`

### 5.11 `np release`, `np close`, `np defer`, `np wait` Commands

CLI adapters for state transitions. Each requires `--claim`. `np close` is tasks only.

- **Depends on:** 3.7, 5.1, 5.2
- **Package:** `internal/cmd/transition`

### 5.12 `np extend` Command

CLI adapter for stale threshold extension. Requires `--claim` and new duration.

- **Depends on:** 3.8, 5.1, 5.2
- **Package:** `internal/cmd/extend`

### 5.13 `np delete` Command

CLI adapter for soft deletion. Requires `--claim` and `--confirm` flag.

- **Depends on:** 3.9, 5.1, 5.2
- **Package:** `internal/cmd/delete`

### 5.14 `np relate` Command

CLI adapter for ungated relationship management per §8.4. Subcommands or flags for add/remove. Requires `--author`, source issue ID, relationship type (`blocks` or `cites`), and target issue ID. Does not require `--claim`.

- **Depends on:** 3.11, 5.1, 5.2
- **Package:** `internal/cmd/relate`

### 5.15 `np show` Command

CLI adapter for issue detail view. Positional arg: issue ID. Displays all fields, dimensions, relationships, children, derived properties (readiness, completion, revision, author).

- **Depends on:** 3.12, 5.1, 5.2
- **Package:** `internal/cmd/show`

### 5.16 `np list` Command

CLI adapter for issue listing. Flags for all filters (role, state, ready, parent, dimension including negative matching) and ordering. Keyset pagination flags (`--after`, `--page-size`). Optional `--timestamps` flag to include created/modified timestamps in output.

- **Depends on:** 3.12, 5.1, 5.2
- **Package:** `internal/cmd/list`

### 5.17 `np search` Command

CLI adapter for full-text search. Positional arg: query. Flags for filters, ordering, pagination, `--search-notes`.

- **Depends on:** 3.12, 5.1, 5.2
- **Package:** `internal/cmd/search`

### 5.18 `np comment add`, `np comment show`, `np comment list`, `np comment search` Commands

CLI adapters for note operations. `np comment add` requires `--author` and `--body`. Search has both per-issue and global modes.

- **Depends on:** 3.10, 5.1, 5.2
- **Package:** `internal/cmd/note`

### 5.19 `np history` Command

CLI adapter for history listing. Positional arg: issue ID. Filter/pagination flags.

- **Depends on:** 3.13, 5.1, 5.2
- **Package:** `internal/cmd/history`

### 5.20 `np doctor` Command

CLI adapter for diagnostics. No required flags. Outputs findings.

- **Depends on:** 3.14, 5.1, 5.2
- **Package:** `internal/cmd/doctor`

### 5.21 `np gc` Command

CLI adapter for garbage collection. Flags: `--deleted-only` (default), `--include-closed`, `--confirm`.

- **Depends on:** 3.15, 5.1, 5.2
- **Package:** `internal/cmd/gc`

### 5.22 Register All Commands

Wire all subcommands into the root command. Update `NewRootCmd` to register the full command tree. Add database-discovery to the root `Before` hook (commands that need the database get it from context; `init` and `agent` skip it).

- **Depends on:** 5.3–5.21, 4.1
- **Package:** `internal/cmd/root`

---

## Phase 6 — Integration Testing and Polish

### 6.1 SQLite Integration Tests

Integration tests (build tag: `integration`) for the SQLite repository. Verify all CRUD operations, transactions, keyset pagination, FTS, concurrent claiming (via goroutines), and GC. One `TestMain` to manage test database lifecycle.

- **Depends on:** Phase 4

### 6.2 E2E CLI Tests

E2E tests (build tag: `e2e`) that invoke the `np` binary and verify output. Cover the core workflow: init → create → claim → update → close. Verify JSON output shape, exit codes, and error messages.

- **Depends on:** Phase 5

### 6.3 Human-Readable Output Formatting

Polish the human-readable (non-JSON) output for all commands: table formatting for lists, colored status indicators, relative timestamps when stdout is a TTY, structured issue detail view.

- **Depends on:** Phase 5

### 6.4 `--help` Text and Usage Strings

Review and polish all command usage strings, flag descriptions, and examples. Ensure `np --help` and `np <command> --help` are useful for both humans and agents per §9.

- **Depends on:** Phase 5

---

## Dependency Graph (Phases)

```
Phase 1 (Domain) ──→ Phase 2 (Ports) ──→ Phase 3 (App Services + Fakes)
                                              │
                                              ▼
                                         Phase 4 (SQLite Adapter) ──→ 6.1 Integration Tests
                                              │
                                              ▼
                                         Phase 5 (CLI Adapter) ──→ 6.2 E2E Tests
                                                                 ──→ 6.3 Output Polish
                                                                 ──→ 6.4 Help Text
```

Phase 4 and Phase 5 are **sequential**, not parallel — CLI commands require the SQLite adapter for production wiring, and `np init` (5.3) depends directly on the schema (4.2).

Within Phase 1, many value objects (1.2–1.6, 1.11–1.14, 1.20–1.21) are independent and could theoretically be parallelized, but serial execution is assumed. Task 1.1 (Domain Error Types) is first because subsequent tasks depend on its typed errors.

Within Phase 5, commands are independent of each other; the listed order follows a logical workflow progression (init → create → claim → update → transition → relate → query → diagnostics).

---

## Out of Scope

The following items from the product vision are explicitly not planned:

- **Duplicate handling** — not an automated operation. Users resolve duplicates manually using existing commands (close one issue, add a `cites` relationship to the surviving issue). See §11 of the specification.

---

## Estimated Task Count

| Phase | Tasks | Cumulative |
|-------|-------|------------|
| 1 — Core Domain | 21 | 21 |
| 2 — Ports | 2 | 23 |
| 3 — App Services | 15 | 38 |
| 4 — SQLite Adapter | 8 | 46 |
| 5 — CLI Adapter | 22 | 68 |
| 6 — Integration/Polish | 4 | 72 |
| **Total** | **72** | |
