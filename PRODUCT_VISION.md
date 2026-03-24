# Nitpicking (`np`) — Product Ideas

> A local-only, non-invasive, single-machine, CLI-driven, no-database-server issue tracker
> designed for AI agent workflows.

Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project.

---

## Table of Contents

1. [Design Principles](#1-design-principles)
2. [Motivation: Deficiencies of Beads](#2-motivation-deficiencies-of-beads)
3. [Issue Model](#3-issue-model)
   - 3.1 [Issue Types](#31-issue-types)
   - 3.2 [Labels (Convention on Facets)](#32-labels-convention-on-facets)
   - 3.3 [Common Fields](#33-common-fields)
   - 3.4 [States](#34-states)
   - 3.5 [Relationships](#35-relationships)
   - 3.6 [Notes](#36-notes)
   - 3.7 [Issue ID Format](#37-issue-id-format)
   - 3.8 [Facets](#38-facets)
4. [Issue Lifecycle](#4-issue-lifecycle)
   - 4.1 [Claiming & Updating](#41-claiming--updating)
   - 4.2 [Epic Completion Derivation](#42-epic-completion-derivation)
   - 4.3 [Readiness](#43-readiness)
   - 4.4 [Duplicate Handling](#44-duplicate-handling)
   - 4.5 [Stale Issues & Stealing](#45-stale-issues--stealing)
   - 4.6 [Soft Deletion](#46-soft-deletion)
5. [History & Auditability](#5-history--auditability)
6. [Architecture](#6-architecture)
7. [Commands](#7-commands)
   - 7.0 [Cross-Cutting Concerns](#70-cross-cutting-concerns)
   - 7.1 [Global Operations](#71-global-operations)
   - 7.2 [Issue Operations](#72-issue-operations)
   - 7.3 [Note Operations](#73-note-operations)
   - 7.4 [History Operations](#74-history-operations)
   - 7.5 [Diagnostics](#75-diagnostics)
   - 7.6 [Agent Ergonomics](#76-agent-ergonomics)
8. [Out of Scope](#8-out-of-scope)
9. [Conflicts & Tensions](#9-conflicts--tensions)
10. [Open Questions for Next Session](#10-open-questions-for-next-session)

---

## 1. Design Principles

- **Local-only** — runs on a single machine; no network, no remote sync.
- **Non-invasive** — no global hooks, no coupling to git lifecycle, no background daemons.
- **No database server** — embedded storage only; no Dolt, no Postgres, no separate process.
- **CLI-driven** — first-class CLI (`np`); designed to be called by AI agents and humans alike.
- **Per-project databases** — each project gets its own issue database. The developer decides the scope boundary by choosing the directory where the database lives — a single repo, a parent directory spanning multiple repos, etc.
- **No agent orchestration** — the tool tracks issues; it does not coordinate which agent works on what. Developers manage agent coordination externally.

## 2. Motivation: Deficiencies of Beads

### Invasiveness

- Beads installs itself as a **global pre-hook** for Claude Code.
- Beads ties itself to the project's **git lifecycle** in opaque ways.
- Beads **spawns a database server** (Dolt).

### Excessive Complexity

- Beads targets **multi-project, multi-machine agent collaboration** — far beyond
  single-machine needs.
- Beads has expanded beyond issue tracking into "molecules" and other constructs,
  possibly for agent swarms — scope creep that adds cognitive overhead.

---

## 3. Issue Model

### 3.1 Issue Types

There are exactly two issue types (referred to as "roles"):

#### Epic *(name is provisional — "container" and "task group" are under consideration)*

An issue that organizes other issues. Its completion derives from its children.

- Optionally has an epic as its parent (nesting is allowed).
- **Cannot** be directly completed — completion is derived from children
  (see [4.2](#42-epic-completion-derivation)).
- Has three directly settable **planning states**: `active`, `deferred`, `waiting`
  (see [3.4](#34-states)).
- **Claimable** — an epic must be claimed to edit its metadata or decompose it into
  children. This prevents two agents from racing to break down the same epic.
- Has a **priority** (P0–P4; see [3.3](#33-common-fields)).

#### Task

The actionable work. Describes a step or sequence of steps that progresses the
project — or its parent epic — closer to completion.

- Optionally has an epic as its parent.
- **Cannot** have child issues of its own (leaf node only).

### 3.2 Labels (Convention on Facets)

Labels are not a first-class field — they are a **convention** using facets. The
recommended facet key is `kind` (e.g., `kind:feat`, `kind:fix`). This applies to
any issue, not just epics.

Users are free to define whatever label vocabulary fits their workflow — corporate Jira
categories, team-specific conventions, or anything else. For users who don't have an
existing convention, the recommended default is the
[Conventional Commits](https://www.conventionalcommits.org/) type vocabulary. Most
developers already know these types and their boundaries:

| Value      | Definition |
|------------|------------|
| `feat`     | A new feature — something the project couldn't do before. |
| `fix`      | A bug fix or defect correction. |
| `refactor` | Restructures code without changing external behavior. |
| `perf`     | A performance improvement. |
| `test`     | Adds or corrects tests. |
| `docs`     | Documentation only. |
| `style`    | Formatting, whitespace, or cosmetic changes — no behavior change. |
| `build`    | Changes to the build system, dependencies, or tooling. |
| `ci`       | Changes to CI/CD configuration or automation. |
| `chore`    | Maintenance and housekeeping that doesn't fit other categories. |

These are suggestions, not enforced values. The system does not validate facet values.

### 3.3 Common Fields

All issues — regardless of type — carry these fields:

| Field               | Required | Notes |
|---------------------|----------|-------|
| ID                  | Yes      | `<PREFIX>-<random>`, auto-assigned, **immutable**. See [3.7](#37-issue-id-format). |
| Title               | Yes      | Must contain at least one alphanumeric character. |
| Description         | No       | |
| Acceptance Criteria | No       | |
| Priority            | Yes      | `P0`–`P4`. Default: `P2`. Lower number = higher urgency. Changing priority requires claiming. |
| Revision            | Yes      | Integer; derived from history entry count (`revision = history count − 1`). Starts at `0` for a newly created issue. See [5](#5-history--auditability). |
| State               | Yes      | See [3.4](#34-states). Tasks start as `open`; epics start as `active`. |
| Facets              | —        | Zero or more key-value pairs. See [3.8](#38-facets). |
| Notes               | —        | Zero or more. See [3.6](#36-notes). |
| Relationships       | —        | Zero or more. See [3.5](#35-relationships). |

### 3.4 States

Epics and tasks share the same state vocabulary but differ in which states apply and
how completion works.

#### State Definitions

| State      | Applies to  | Meaning |
|------------|-------------|---------|
| `open`     | Tasks       | Available for work. Default state at creation. |
| `active`   | Epics       | This epic is live. Children follow their own lifecycles; readiness flows normally. Default state at creation. |
| `claimed`  | Both        | An agent or human has taken ownership. For tasks: working on it or updating fields. For epics: editing metadata or decomposing into children. |
| `closed`   | Tasks       | Fully resolved. **Terminal** — cannot be claimed or reopened. Create a new issue and cite the old one instead. |
| `deferred` | Both        | Should not be worked on now. For epics: unclaimed descendants are no longer ready (see [4.3](#43-readiness)); claimed descendants continue, but nothing new starts. |
| `waiting`  | Both        | Cannot proceed until something **external to the issue tracker** happens — e.g., a human decision, a permission grant. Same readiness propagation as `deferred` for epics. (Name is provisional; "blocked" is explicitly rejected.) |

#### State Transitions

**Claiming is the universal gate for all state changes.** To move an issue to any new
state, you must be its current claimer. The only ungated transition is into `claimed`
itself (from any non-terminal state).

```
(any non-terminal state) → claimed    take ownership
claimed → open / active               release without completing
claimed → closed                      complete (tasks only)
claimed → deferred                    shelve
claimed → waiting                     externally blocked
```

`closed` and `deleted` are terminal — they cannot be claimed.

#### Epic Completion

Epic **completion** is not a state — it is a derived observation
(see [4.2](#42-epic-completion-derivation)). Critically, completion is **not a lock** —
new children can always be added to an epic regardless of whether it is currently
complete. Adding a child flips the epic back to incomplete. "Complete" means "everything
currently under here is closed", not "this epic is finished forever."

### 3.5 Relationships

| Relationship    | Inverse        | Semantics |
|-----------------|----------------|-----------|
| `blocked_by`    | `blocks`       | This issue cannot make progress until the referenced issue is closed. |
| `cites`         | `cited_by`     | This issue references the target issue as relevant context. |

### 3.6 Notes

Notes are comments on an issue. They can be added **at any time by anyone** without
claiming the issue.

| Field      | Required | Notes |
|------------|----------|-------|
| ID         | Yes      | Auto-assigned sequential integer, displayed as `note-<integer>` (e.g., `note-368`). Global across the database, not scoped per issue. |
| Author     | Yes      | Must contain at least one alphanumeric character. |
| Created At | Yes      | Automatically applied timestamp. |
| Body       | Yes      | Free-form text. |

#### Why Flat Notes Are Sufficient

Notes are a flat list — there is no structural threading, no `reply_to` field. This is
deliberate. Conversational context can be established by convention:

- **Reference by ID** — "I researched the idea in note-368 and found that..." reads
  naturally in prose.
- **Topic line** — starting a note with "Re: Using Bolt to cache latest dashboard state"
  signals what the note responds to without structural coupling.

These conventions are free — agents and humans can use them immediately without any
tooling support. If note-to-note references become frequent enough that tracing a
conversation thread is painful, the CLI can add query commands that pattern-match on
note IDs in body text, build the reference graph at query time, and output threaded
views. If that proves too slow, a non-canonical index can accelerate the queries without
changing the data model. The point is that flat notes with conventions scale surprisingly
far before structural threading is needed — and structural threading, once added, cannot
be removed.

### 3.7 Issue ID Format

Issue IDs have the form `<PREFIX>-<random>` (e.g., `NP-a3bxr`, `NP-k7m2e`).

- The **prefix** is uppercase, set once at database initialization, and immutable. It is
  expected to be short (e.g., `NP` for "nitpicking", `PLAT` for "platform") but can be
  longer if the user wants.
- The **random portion** is 5 lowercase [Crockford Base32](https://www.crockford.com/base32.html)
  characters, generated randomly on each issue creation. This gives an ID space of
  33,554,432 — sufficient for <0.000041% triple-collision probability at 250,000 issues.
  On collision, regenerate and retry.
- The **case contrast** between uppercase prefix and lowercase random portion provides
  clear visual separation: `NP-a3bxr` is easy to glance-parse.
- The prefix gives humans a way to identify which database an issue belongs to without
  needing to know the directory path — "NP-a3bxr" is unambiguous in conversation even
  when multiple databases exist on the same machine.

#### Why Random, Not Sequential

Random IDs distribute issues evenly across SQLite B-tree pages. Sequential integers
cluster recent (and therefore most active) issues on the same leaf pages, which is
exactly where write contention is worst when multiple agents operate concurrently. The
trade-off is losing "higher = newer" ordering, which is acceptable — queries can sort
by creation timestamp instead.

**SQLite implementation note:** the issue table must be created `WITHOUT ROWID` so the
declared primary key (the random ID) is the actual B-tree key. Otherwise, SQLite's
implicit `ROWID` reintroduces sequential clustering under the hood.

#### Prefix Is Required

The prefix **must be specified** by the user at database initialization. There is no
auto-generation. This keeps initialization explicit and avoids heuristic edge cases
(short directory names, profanity filtering, collisions with other databases).

### 3.8 Facets

Facets are key-value pairs on any issue. They are the primary mechanism for
**filtering** — particularly for coordinating agents that work in different scopes under
a shared database. The name "facets" is deliberate: it implies a small number of
meaningful dimensions for filtering and classification, not a dumping ground for
arbitrary metadata.

- Keys and values are short strings. No schema enforcement — any key is valid.
- An issue can have multiple facets. Keys are unique per issue (setting a key that
  already exists overwrites the previous value).
- Facets require **claiming** to add, modify, or remove (same as other issue fields).
- Facets are queryable: `np claim --ready --filter=repo:auth-service`,
  `np list --filter=lang:go`, etc. Exact filter syntax is a CLI design decision (deferred).

#### Motivating Use Case

A project database lives in a parent directory containing multiple repos. One agent
operates per repo. Each repo's tasks are faceted with `repo:<name>`. An agent claims
work by filtering for its repo: `np claim --ready --filter=repo:auth-service`. This
enables parallel agents under one database without an orchestration layer — each agent
self-selects relevant work via facet filters.

#### Design Notes

- No well-known keys are defined. All keys are free-form. Conventions will emerge from
  usage; premature standardization risks encoding the wrong abstractions.
- Facets are not a replacement for first-class fields (state, priority, labels). If
  something has defined semantics and affects the lifecycle or readiness model, it
  belongs in the issue model, not in facets.

---

## 4. Issue Lifecycle

### 4.1 Claiming & Updating

- An issue **must be claimed** before its fields can be updated. This includes all state
  changes — claiming is the universal gate (see [3.4](#34-states)).
- **Exceptions — no claim required:**
  - **Notes** — append-only; anyone can comment on any issue. An agent working on
    issue A should be able to leave a note on issue B without owning B.
  - **Relationships** — adding a relationship between two issues should not require
    claiming either issue.
- `closed` and `deleted` issues **cannot be claimed**.
- For quick updates to unclaimed issues (setting facets, fixing a title, etc.), the CLI
  should support a one-shot claim → update → release as a single command per issue.

#### Claim IDs

When an issue is claimed, the operation returns a **claim ID** — a random, unguessable
token that is valid for the duration of the claim. The claim ID serves as bearer
authentication for the claim: every operation that requires an active claim must present
the claim ID, and the operation fails if it does not match.

- Claim IDs are generated randomly and are opaque to callers.
- Every command response that involves a claimed issue includes the claim ID, so the
  caller always has it available.
- A claim ID is **invalidated** when the claim ends — whether by the claimer releasing
  the issue (closing, deferring, unclaiming, waiting) or by another agent stealing it.
- When an issue is stolen, a **new** claim ID is issued to the stealer.
- The one-shot claim → update → release pattern generates and immediately invalidates
  a claim ID internally; the caller never needs to see or manage it.

#### Author Attribution

The **author** is bound to the claim at claim time. All gated mutations (updates, state
transitions, deletion) inherit the author from the claim — the caller does not need to
pass the author on every operation, and cannot misrepresent authorship while holding a
claim.

**Ungated operations** (adding notes, adding/removing relationships) have no claim to
bind to, so they require an **explicit author** parameter. Callers may use a name
generated by the agent name command or any name of their choosing.

#### Author Validation

The same rules apply to all author fields — whether bound to a claim or passed
explicitly:

- Must contain at least one alphanumeric character.
- Maximum length: **64 Unicode runes** (measured after normalization).
- **No whitespace** — the string must not contain any Unicode whitespace characters.
- **NFC-normalized** — the system normalizes all author strings to Unicode NFC
  (Canonical Decomposition followed by Canonical Composition) on input. NFC is the
  most widely produced form, preserves visual intent, and avoids equivalent-but-
  different byte sequences for the same logical name.
- Normalized author strings are comparable for **equality** and **sorting**,
  **case-sensitive**. `"alice"` and `"Alice"` are distinct authors.

### 4.2 Epic Completion Derivation

Epic completion is derived from children, never directly set.

- **Complete** when: it has children **and** all of them are closed (for tasks) or
  complete (for sub-epics).
- **Incomplete** otherwise — including when the epic has no children (it needs
  decomposition).

Completion is an observation, not a lock. New children can always be added to a
complete epic, which flips it back to incomplete. This avoids a race condition where
closing the last task would lock out adding newly discovered work, and supports organic
groupings where "complete" just means "nothing pending right now."

An epic with no children is a structural observation, not an error state. It means the
epic has been identified but not yet broken down. `doctor` can flag epics with no
descendants that have tasks (see [7.1](#71-doctor)).

### 4.3 Readiness

A task is **ready** when all of the following are true:

1. Its state is `open`.
2. It has no `blocked_by` relationships, **or** every `blocked_by` target has been
   closed or deleted.
3. No ancestor epic is `deferred` or `waiting`. (Readiness propagates downward — a
   deferred or waiting epic suppresses readiness for all unclaimed descendants.)

### 4.4 Duplicate Handling

When an issue is determined to be a duplicate:

- One issue is closed with a `cites` relationship pointing to the surviving issue.
- **Preferred heuristic:** keep the issue with the most complete and useful title,
  description, and acceptance criteria.
- **Exception:** if the "weaker" issue has the richer interaction history (notes,
  relationships, etc.), closing the "stronger" issue may be more appropriate.
- This is a judgement call.

### 4.5 Stale Issues & Stealing

#### Staleness

- A `claimed` issue becomes **stale** when it has had no updates and no new notes for
  its **stale threshold**.
- **Default threshold:** 2 hours.
- **Custom threshold:** when claiming an issue, the claimer can optionally set a custom
  threshold, up to a maximum of 24 hours.
- **Extending:** the claimer can extend the threshold at any time, up to the 24-hour
  maximum. An agent that keeps extending is fine for the tracker's purposes — the goal
  is to detect abandoned claims, not to police how long work takes.

#### Stealing

- Stale claimed issues can be **stolen**:
  - Directly by ID, or
  - Automatically when there are no ready issues available.
- Stealing is **atomic**: the old claim is invalidated and the new claim is created in
  a single transaction. If two agents race to steal the same stale issue, exactly one
  succeeds; the other receives a claim-conflict error.
- When an issue is stolen, a note is **automatically generated** using the stealer's
  claim-bound author (e.g., "Stolen from `<previous-claimer>`."). The stealer inherits the issue
  content in whatever state it was left in — how they deal with potentially incomplete
  work is their concern, not the tracker's.

### 4.6 Soft Deletion

Deletion is implemented as a soft removal but **treated as a hard delete by everything
unless specifically told otherwise.** The data is retained in the database for history
reconstruction and potential garbage collection, but from the perspective of all normal
operations, the issue does not exist.

- A deleted issue's **ID is permanently reserved** — it cannot be reused.
- Requesting a deleted issue by ID returns a "not found" error. No opt-in visibility
  for deleted issues in normal list/detail views.
- A deleted issue is **immutable** — no further changes of any kind.
- A deleted issue **cannot be referenced** in new relationships.
- Existing relationships pointing to a deleted issue are **ignored** — treated as
  though the relationship does not exist. The relationship data is retained in storage
  but invisible to all normal operations.
- Deleting an epic **deletes all its children recursively**.
- Deleted issues **cannot be undeleted.**

The `gc` command (see [7.2](#72-gc)) can physically remove deleted issue data. `gc`
can be targeted — e.g., remove deleted issues without discarding closed issues.
`doctor` may recommend a `gc` run if significant space can be reclaimed.

---

## 5. History & Auditability

Every mutation transaction produces exactly one **history entry**. This includes issue
creation, field updates, state transitions, claiming, releasing, deletion — every
operation that changes the issue's state in the database.

### History Entry Model

| Field        | Notes |
|--------------|-------|
| Entry ID     | Auto-assigned. Unique within the issue's history. |
| Issue ID    | The issue this entry belongs to. |
| Revision     | Zero-based index within the issue's history (`0` = creation). The issue's current revision equals its latest history entry's revision. |
| Author       | Inherited from the active claim for gated operations; explicit parameter for ungated operations. |
| Timestamp    | Automatically applied. |
| Event Type   | The kind of mutation — e.g., `created`, `claimed`, `released`, `updated`, `state_changed`, `deleted`. |
| Changes      | Structured delta: which fields changed, with before and after values. The exact representation is an implementation decision, but it must be sufficient to reconstruct the issue's state at any revision. |

### Design Intent

History is per-issue. Each issue's history is an ordered, append-only sequence of
entries that fully describes the issue's evolution from creation to its current state.

Because issues are soft-deleted (not physically removed) and history is append-only,
it is possible to reconstruct a close approximation of global history by merging
per-issue histories ordered by timestamp. A first-class global history view is **out
of scope** for now, but the data model does not preclude it — this is, in effect, an
event-sourcing system. If real-world usage demonstrates the need, global history
reconstruction can be pulled into scope without schema changes.

Garbage collection (`gc`) is the only operation that destroys history entries. Once
`gc` removes an issue's data, that issue's history is gone.

---

## 6. Architecture

### Ports & Adapters (Hexagonal)

The implementation follows **Ports & Adapters (Hexagonal) Architecture**. This
separates the system into three distinct layers:

```
┌─────────────────────────────────────────────────────┐
│                  Driving Adapter                    │
│              (CLI — main() entry point)             │
│       command parsing, flags, output formatting     │
├─────────────────────────────────────────────────────┤
│                  Driving Port                       │
│       (Application API / Use-Case Boundary)         │
│  the interface exposed to main() / driving adapters │
├─────────────────────────────────────────────────────┤
│                    Core Domain                      │
│     issue model, state machine, business rules,    │
│     validation, history, readiness, deletion logic  │
├─────────────────────────────────────────────────────┤
│                  Driven Port                        │
│            (Persistence Port Interface)             │
│   the interface the core requires of its storage    │
├─────────────────────────────────────────────────────┤
│                  Driven Adapter                     │
│          (SQLite adapter, or future alternatives)   │
│            schema, queries, transactions            │
└─────────────────────────────────────────────────────┘
```

#### Development Strategy

This architecture enables a deliberate sequencing of work:

1. **First: Core domain** — implement the issue model, state machine, lifecycle rules,
   validation, and business logic in isolation, with no dependency on CLI structure or
   storage technology.
2. **Second: Port interfaces** — define the **driving port** (the application API that
   `main()` or any driving adapter calls into) and the **driven port** (the persistence
   interface the core requires). These are contracts, not implementations.
3. **Third: Adapters** — implement the CLI (driving adapter) and SQLite storage (driven
   adapter) against the established port contracts.

This means decisions about **CLI command/subcommand/flag structure** and **SQLite
schema** are explicitly **deferred** until the core logic and port interfaces are solid.
The ports define what the adapters must do; the adapters decide how.

#### Database Discovery

`np` locates its database by walking **up from the current working directory**, looking
for a `.np/` directory. The search starts at `cwd` and proceeds to each parent
directory until either a `.np/` directory is found or the filesystem root is reached.

- If a `.np/` directory is found, the database inside it is used.
- If the root is reached without finding `.np/`, the command fails with a "no database
  found" error.
- **Permission and sandbox errors are silently ignored** during the walk. If a
  directory cannot be read due to filesystem permissions, kernel sandboxing (e.g.,
  macOS App Sandbox, SELinux), or similar access restrictions, it is skipped. The only
  discovery error is failing to find a `.np/` directory — which may be *caused* by
  permission errors, but the permission errors themselves are not surfaced.

This design means `np` commands work from any subdirectory of the project without
requiring the user to specify a path or set an environment variable.

#### Benefits for This Project

- **Testability** — the core domain can be exhaustively unit-tested with in-memory fakes
  for the persistence port, no SQLite required.
- **Deferred decisions** — CLI ergonomics and storage schema are important but secondary
  to getting the domain model right. Hexagonal architecture makes this sequencing
  natural rather than forced.
- **Swappable storage** — if SQLite proves wrong (or a flat-file format is wanted for
  certain use cases), only the driven adapter changes.
- **Agent-friendliness** — the driving port is a clean programmatic API; the CLI adapter
  is just one consumer of it. Future adapters (MCP server, library API, etc.) plug in
  at the same boundary.

---

## 7. Commands

### 7.0 Cross-Cutting Concerns

#### Pagination

All commands that return lists (issues, notes, history entries) are **paginated** using
keyset pagination. The default page size is **20 items**.

- The response includes the **total count** of matching items so the caller knows whether
  more pages exist.
- To fetch the next page, the caller passes the **last item's sort key and ID** from the
  current page. The database seeks directly to that position — no scanning past skipped
  rows.
- Keyset pagination is stable under concurrent inserts and deletes: no missed or
  duplicated entries between pages.

#### Agent Name Generation

A command to generate a readable, random agent name for a session — e.g.,
"dashing-storage-glitter", "slow-green-scissors". The format follows the same style as
Docker's auto-generated container names.

Persisting the name or reusing it across sessions is out of scope — the user handles that
through instructions to their agents.

### 7.1 Global Operations

#### Initialize

Initialize the database by creating a `.np/` directory in the current working directory.
The user must specify a **prefix** for issue IDs (see [3.7](#37-issue-id-format));
the prefix is required and cannot be changed after initialization.

#### Agent Name

Generate a readable, random agent name for a session (see [7.0](#70-cross-cutting-concerns)).

#### Agent Instructions

Generate a concise Markdown block describing how to use `np`, suitable for pasting into
`CLAUDE.md`, a rule file, or a system prompt. The output covers:

- Core workflow: claim → work → transition state.
- How claim IDs work and when to pass them.
- Always move an issue to an appropriate unclaimed state when done (close, defer, wait,
  or release).
- How to discover more: `np --help` and `np <command> --help`.

The instructions must state that `np` is the **exclusive** tool for task management,
to-dos, and planning — agents must not use built-in task management, to-do lists, or
planning features from their own platform. If the user wants softer language, they can
edit the generated text before pasting it.

The instructions must be **brief** — enough to get an agent started and know where to
find details. An agent's context window is precious; a wall of documentation defeats the
purpose.

### 7.2 Issue Operations

#### Create

Create an issue. All properties on the issue may be set at creation: title, description,
acceptance criteria, priority, type (task or epic), parent epic, facets, and relationships.
The caller may optionally have the issue start as **claimed** by them, which returns a
claim ID (see [4.1](#41-claiming--updating)).

The caller may optionally provide an **idempotency key** — an opaque string. If an issue
with the same idempotency key already exists, the operation returns the existing issue
instead of creating a duplicate. This prevents double-creation when an agent crashes
after the write succeeds but before it reads the response.

#### Claim by ID

Claim a specific issue by its ID. Returns a claim ID.

- If the issue is already claimed and **stale**, the caller may optionally indicate that
  stealing is allowed. If stealing succeeds, a new claim ID is issued and an auto-generated
  note is added (see [4.5](#45-stale-issues--stealing)).
- If the issue is claimed and **not stale**, the operation fails — the caller cannot
  steal a non-stale issue.

#### Claim Next Ready

Claim the highest-priority unclaimed ready issue (lowest `P` number first; ties broken
by earliest creation time). Filterable by facet. Returns the claimed issue and a
claim ID.

- Variant: if no ready issues are available, optionally **steal** the highest-priority
  stale claimed issue instead. The caller must explicitly opt into this fallback.

#### Update

Update one or more properties, facets (full CRUD), relationships (create/delete), and/or
parent assignment on a claimed issue. Optionally add a note as part of the same
operation. Requires the claim ID. All changes are applied as a **single atomic mutation**.

#### Transition State

Change the state of a claimed issue. Requires the claim ID. Valid transitions from
`claimed`:

- **Release** — return to `open` (tasks) or `active` (epics) without completing.
- **Close** — mark as complete (tasks only). Terminal.
- **Defer** — shelve.
- **Wait** — externally blocked.

All of these transitions **end the claim** and invalidate the claim ID.

#### Delete

Soft-delete an issue (see [4.6](#46-soft-deletion)). Requires claiming the issue first.
Deleting an epic **recursively deletes** all its children.

#### Show

Display the full current state of an issue: all fields, facets, relationships, parent,
children (for epics), and derived properties (readiness, completion status). Notes are
**excluded** — they have their own paginated listing.

#### List

List issues. Displays high-level information by default: ID, type, state, priority, and
title. Optionally include creation and/or modification timestamps.

- **Filterable** by state, or by the computed "ready" predicate (see [4.3](#43-readiness)).
- **Filterable** by facet. `key:*` matches all issues with that key regardless of value.
  Negative facet matching (exclude issues with a facet) may be supported.
- **Orderable** by priority, creation time, modification time, or other criteria.
- **Paginated** (see [7.0](#70-cross-cutting-concerns)).

#### Search

Search issues using full-text search on title, description, and acceptance criteria.
Optionally include notes in the FTS scope. Also filterable by facet (including `key:*`
and possibly negative matching).

- Same high-level display as **List**.
- **Filterable** by state.
- **Orderable** and **paginated**.

### 7.3 Note Operations

#### Add Note

Add a note to an issue. Does **not** require claiming. Requires an **explicit author**
and body (see [4.1 Author Attribution](#41-claiming--updating)).

#### Show Note

Display a single note by its ID.

#### List Notes

List notes on a specific issue.

- **Filterable** by author, by created-after date-time, or by created-after a specific
  note ID.
- **Orderable** and **paginated**.

#### Search Notes (Per-Issue)

Search notes on a specific issue using full-text search.

#### Search Notes (Global)

Search all notes across all issues using full-text search.

- **Filterable** by author, created-after date-time, created-after a specific note ID,
  issue facets, and issue state.
- **Orderable** and **paginated**.

### 7.4 History Operations

#### Show History

Display the change history for an issue.

- **Filterable** by author and/or date range.
- **Orderable** and **paginated**.

### 7.5 Diagnostics

#### Doctor

A **diagnostic-only** command that identifies problems in the issue database. It
reports findings but does not modify any data. Examples:

- **Circular `blocked_by` relationships** — dependency cycles.
- **Deadlocked state** — all remaining issues are blocked (e.g., on `claimed`,
  `deferred`, or `waiting` issues).
- **Stale `claimed` issues** — issues that have been claimed but show no activity.
- **Epics with no task descendants** — epic subtrees that have no tasks at any leaf,
  indicating decomposition work is needed.
- **Garbage collection opportunity** — if physically removing deleted (and optionally
  closed) issue data would reduce the database size by at least **40%** (heuristic
  estimate), `doctor` notes this and suggests running `gc`.

A future enhancement may add recommended next steps or an auto-fix mode, but the
initial version is purely informational.

#### GC

Physically compact the database by removing deleted (and optionally closed) issue data.
Analogous to `git gc` — not part of normal workflow, available if the database grows
unwieldy toward the end of a long project. Can be targeted — e.g., remove only deleted
issues without discarding closed issues. `doctor` may recommend a `gc` run if
significant space can be reclaimed.

### 7.6 Agent Ergonomics

These principles apply across all commands and define what makes the CLI "designed for
AI agent workflows."

#### Structured Output

Every command supports a **JSON output mode**. When enabled, the command emits
structured JSON instead of human-readable text. The JSON mode is the primary interface
for agent callers; human-readable output is a convenience layer on top.

#### Deterministic Output Shape

The JSON structure for a given command is the **same regardless of the result**. An
empty list is `[]`, not a missing field or `null`. A missing optional field is always
present with a null or default value — never omitted. This lets agents use rigid
parsing without defensive coding.

#### Predictable Exit Codes

Exit codes distinguish failure classes so agents can branch without parsing error text.
At minimum:

| Code | Meaning |
|------|---------|
| 0    | Success. |
| 1    | General / unexpected error. |
| 2    | Not found (issue, note, etc.). |
| 3    | Claim conflict (issue is claimed and not stale, or claim ID mismatch). |
| 4    | Validation error (bad input). |
| 5    | Database error (corruption, locked, etc.). |

The exact codes will be finalized during implementation; the principle is that common,
actionable failure modes have distinct codes.

#### Self-Describing Errors

In JSON mode, error responses include **structured context** — not just a message
string. When a claim fails, the response includes who holds the claim and when it
becomes stale. When validation fails, the response identifies which fields are invalid
and why. The goal is to give the agent enough information to decide its next action
without guessing.

#### No Interactive Prompts

Every operation must be completable in a **single invocation**. No "are you sure?"
confirmations, no pagers, no editors. If a destructive operation needs a safety gate,
it uses a required flag (e.g., `--confirm`), not an interactive prompt.

#### Idempotent Where Natural

Operations that can be idempotent should be:

- **Relationships** — "create if not exists", "delete if exists" (already defined in
  the concurrency model).
- **Issue creation** — optional idempotency key (see [7.2 Create](#72-issue-operations)).

Operations that are inherently non-idempotent (claiming, adding notes) remain so —
idempotency is not forced where it would distort the semantics.

---

## 8. Out of Scope

- **Note threading** — notes are a flat list; no reply chains or nested conversations.

---

## 9. Conflicts & Tensions

The following areas contain ideas that may conflict with each other or with the stated
design principles. They should be resolved before implementation.

### ~~9.1 Objective State: Derived vs. Explicit~~ — RESOLVED

Resolved: epics have a separate state model from tasks. Epics have three directly
settable planning states (`active`, `deferred`, `waiting`) plus `claimed`. Completion is
derived from children, never directly set. `deferred` and `waiting` propagate downward
as readiness constraints — unclaimed descendants are no longer ready, but claimed
descendants continue. See [3.4](#34-states) and [4.2](#42-epic-completion-derivation).

### ~~9.2 Claiming Objectives~~ — RESOLVED

Resolved: epics are claimable. Claiming an epic means "I am editing this epic's metadata
or decomposing it into children." This is the mechanism that prevents two agents from
racing to break down the same epic. See [3.1](#31-issue-types).

### ~~9.3 Per-Project Database vs. Cross-Project Issues~~ — RESOLVED

Resolved: the developer decides the scope boundary by choosing where the database lives.
A project is not necessarily a repo — it might be a parent directory containing multiple
repos (e.g., all microservices and front-ends for a SaaS platform). Placing the database
at that level gives all repos a shared issue space. The tool does not manage cross-database
references or multi-database federation; if work spans a boundary, the developer moves the
boundary.

Agent coordination (parallel agents, orchestration) is explicitly **out of scope**.
The tool tracks issues; it does not orchestrate who works on them. Developers coordinate
agents externally (e.g., serial execution via Ralph Loop, or manual assignment). This is
intentionally limiting — orchestration complexity is the path to insurmountable adoption
friction.

### ~~9.4 Project ID Semantics~~ — RESOLVED

Resolved by 9.3: since the database scope is determined by directory placement rather
than a project ID encoding scheme, Project ID as an issue field is no longer needed.
The database *is* the project boundary. Issue IDs only need to be unique within their
database.

### ~~9.5 Soft Deletion: Relationship Cleanup~~ — RESOLVED

Resolved: soft deletion keeps everything in place — the issue, its relationships, its
history. Nothing is physically removed. This is the simplest approach and preserves full
reconstructability. `doctor` accounts for deleted issues when analyzing the graph (e.g.,
a `blocked_by` pointing to a deleted issue is not a live blocker).

If the database grows unwieldy toward the end of a long project, a `gc` command (analogous
to `git gc`) can physically compact closed and deleted data. This is not expected to be
normal operation — it exists "in case", not "as part of the workflow."

### ~~9.6 Task State vs. Objective Derivation~~ — RESOLVED

Resolved: epic completion is defined strictly as "has children and all children are
closed/complete." Tasks that are `deferred` or `waiting` are not closed, so they keep
the parent epic incomplete — which is accurate. The epic's own planning state
(`active`, `deferred`, `waiting`) is an independent, directly settable concern. If all
children are deferred, the epic is incomplete and likely should itself be marked
`deferred` — but that is a judgment call, not an automatic derivation.

---

## 10. Open Questions for Next Session

### Issue Identity & Project Scoping

1. ~~**ID format**~~ — RESOLVED. See [3.7](#37-issue-id-format).
2. ~~**Project ID encoding**~~ — RESOLVED. See [9.4](#94-project-id-semantics).
3. ~~**Single vs. multi-project databases**~~ — RESOLVED. See [9.3](#93-per-project-database-vs-cross-project-issues).

### State Machine

4. ~~**Full state transitions**~~ — RESOLVED. See [3.4](#34-states).
5. ~~**Objective states beyond open/closed**~~ — RESOLVED. See [9.1](#91-objective-state-derived-vs-explicit).
6. ~~**Claiming objectives**~~ — RESOLVED. See [9.2](#92-claiming-objectives).

### Deletion

7. ~~**Relationship cleanup on deletion**~~ — RESOLVED. See [9.5](#95-soft-deletion-relationship-cleanup).
8. ~~**Undelete**~~ — RESOLVED. No. Deleted issues cannot be undeleted. See [4.6](#46-soft-deletion).

### Staleness & Stealing

9. ~~**Stale threshold**~~ — RESOLVED. See [4.5](#45-stale-issues--stealing).
10. ~~**Steal semantics**~~ — RESOLVED. See [4.5](#45-stale-issues--stealing).

### Metadata / Key-Value Pairs

11. ~~**Structured metadata**~~ — RESOLVED. Free-form key-value facets are a first-class feature. See [3.8](#38-facets).
12. ~~**Well-known keys**~~ — RESOLVED. No well-known keys. All keys are free-form; conventions emerge from usage.

### Labels

13. ~~**Additional epic labels**~~ — RESOLVED. Labels are now a convention on facets (`kind:<value>`), not a first-class field. Users define their own vocabulary. See [3.2](#32-labels-convention-on-facets).
14. ~~**Labels on tasks**~~ — RESOLVED. Facets apply to any issue, so labels (via the `kind` facet) work on both epics and tasks.

### Architecture & Ports

15. ~~**Driven port contract**~~ — RESOLVED. Not an open question — the driven port
    interface will emerge as the core domain is implemented. The core discovers what
    persistence operations it needs; the port interface is defined to match. Transaction
    boundaries, query patterns, and operation granularity will become obvious as
    operations like recursive deletion and readiness queries are built.
16. ~~**Concurrency model**~~ — RESOLVED. Claiming is the concurrency control mechanism.
    All mutable issue fields are gated by exclusive claiming, so there is no concurrent
    modification to handle. The remaining ungated operations have simple solutions:
    notes are append-only (no contention); relationships use idempotent semantics
    ("create if not exists", "delete if exists" — both succeed from the caller's
    perspective). Claiming and stealing are atomic compare-and-swap operations (claim
    succeeds only if issue is unclaimed or stale; loser gets an error and picks another
    issue). SQLite's transaction isolation handles the CAS naturally.
17. ~~**Atomicity guarantees**~~ — RESOLVED. Three levels:
    - **Writes:** a CLI execution that changes the database is atomic. The entire
      mutation succeeds or fails as a unit (e.g., recursive epic deletion is one
      transaction, not many).
    - **Single-issue reads:** atomic — the issue is always in an internally coherent
      state.
    - **Multi-issue reads** (list, search): atomic **per issue**, not across the
      result set. Each issue is coherent, but cross-issue relationships may reflect
      different points in time. E.g., issue A may list B as a child, but B's state
      may have changed between when A and B were each loaded. This avoids locking the
      entire database for read operations. If a cross-issue consistent read proves
      necessary, it can be enabled per-query (e.g., a flag) — but not until there is a
      demonstrated need.

### ~~CLI Design~~ — RESOLVED

18. ~~**Command vocabulary**~~ — RESOLVED. See [7. Commands](#7-commands).
19. ~~**Output formats**~~ — RESOLVED. Every command supports JSON output mode.
    See [7.6 Agent Ergonomics](#76-agent-ergonomics).
20. ~~**Agent ergonomics**~~ — RESOLVED. See [7.6 Agent Ergonomics](#76-agent-ergonomics).

### SQLite Schema (Deferred)

21. **Schema design** — deferred until the driven port interface is defined. The schema
    should be derived from the port contract, not the other way around.
