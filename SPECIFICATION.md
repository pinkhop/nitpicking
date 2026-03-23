# Nitpicking (`np`) — Software Specification

> Version 1.0 — 2026-03-23

---

## Table of Contents

1. [Overview](#1-overview)
2. [Design Principles](#2-design-principles)
3. [Architecture](#3-architecture)
4. [Data Model](#4-data-model)
   - 4.1 [Ticket](#41-ticket)
   - 4.2 [Note](#42-note)
   - 4.3 [Relationship](#43-relationship)
   - 4.4 [History Entry](#44-history-entry)
   - 4.5 [Claim](#45-claim)
   - 4.6 [Facet](#46-facet)
   - 4.7 [Ticket ID Format](#47-ticket-id-format)
   - 4.8 [Author Validation](#48-author-validation)
5. [State Machines](#5-state-machines)
   - 5.1 [Task States](#51-task-states)
   - 5.2 [Epic States](#52-epic-states)
   - 5.3 [State Transition Rules](#53-state-transition-rules)
6. [Lifecycle Rules](#6-lifecycle-rules)
   - 6.1 [Claiming](#61-claiming)
   - 6.2 [Epic Completion Derivation](#62-epic-completion-derivation)
   - 6.3 [Readiness](#63-readiness)
   - 6.4 [Staleness and Stealing](#64-staleness-and-stealing)
   - 6.5 [Soft Deletion](#65-soft-deletion)
   - 6.6 [Duplicate Handling](#66-duplicate-handling)
7. [History and Auditability](#7-history-and-auditability)
8. [Commands](#8-commands)
   - 8.1 [Cross-Cutting Concerns](#81-cross-cutting-concerns)
   - 8.2 [Global Operations](#82-global-operations)
   - 8.3 [Ticket Operations](#83-ticket-operations)
   - 8.4 [Note Operations](#84-note-operations)
   - 8.5 [History Operations](#85-history-operations)
   - 8.6 [Diagnostics](#86-diagnostics)
9. [Agent Ergonomics](#9-agent-ergonomics)
10. [Concurrency and Atomicity](#10-concurrency-and-atomicity)
11. [Out of Scope](#11-out-of-scope)

---

## 1. Overview

Nitpicking (`np`) is a local-only, CLI-driven issue tracker designed for AI agent workflows. It stores tickets in an embedded SQLite database scoped to a project directory, requires no network access, no background daemons, and no database server. Both humans and AI agents interact with it through the `np` CLI.

---

## 2. Design Principles

- **Local-only** — runs on a single machine; no network, no remote sync.
- **Non-invasive** — no global hooks, no coupling to git lifecycle, no background daemons.
- **No database server** — embedded SQLite only; no separate process.
- **CLI-driven** — the `np` command is the sole interface; designed for AI agents and humans alike.
- **Per-project databases** — each project gets its own ticket database. The developer decides the scope boundary by choosing the directory where the database lives — a single repo, a parent directory spanning multiple repos, etc.
- **No agent orchestration** — the tool tracks issues; it does not coordinate which agent works on what.

---

## 3. Architecture

### 3.1 Hexagonal (Ports and Adapters)

The implementation follows Ports and Adapters architecture with three layers:

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
│     ticket model, state machine, business rules,    │
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

### 3.2 Development Sequencing

Work proceeds inside-out:

1. **Core domain** — ticket model, state machine, lifecycle rules, validation, and business logic. No dependency on CLI structure or storage technology.
2. **Port interfaces** — the driving port (application API) and the driven port (persistence interface). These are contracts, not implementations.
3. **Adapters** — CLI (driving adapter) and SQLite storage (driven adapter), implemented against the port contracts.

CLI command structure and SQLite schema are explicitly deferred until the domain model and ports are solid.

### 3.3 Database Discovery

`np` locates its database by walking up from the current working directory, looking for a `.np/` directory.

- The search starts at `cwd` and proceeds to each parent directory until either a `.np/` directory is found or the filesystem root is reached.
- If a `.np/` directory is found, the database inside it is used.
- If the root is reached without finding `.np/`, the command fails with a "no database found" error.
- **Permission and sandbox errors are silently ignored** during the walk. If a directory cannot be read due to filesystem permissions, kernel sandboxing (e.g., macOS App Sandbox, SELinux), or similar access restrictions, it is skipped.

---

## 4. Data Model

### 4.1 Ticket

There are exactly two ticket types, referred to as **roles**: **Epic** and **Task**.

#### Epic

A ticket that organizes other tickets. Its completion is derived from its children.

- Optionally has an epic as its parent (nesting is allowed).
- Cannot be directly completed — completion is derived (see [6.2](#62-epic-completion-derivation)).
- Has three directly settable planning states: `active`, `deferred`, `waiting` (see [5.2](#52-epic-states)).
- Claimable — an epic must be claimed to edit its metadata or decompose it into children.

#### Task

The actionable work unit.

- Optionally has an epic as its parent.
- Cannot have child tickets (leaf node only).

#### Common Fields

All tickets carry these fields regardless of role:

| Field               | Required | Mutable | Notes |
|---------------------|----------|---------|-------|
| ID                  | Yes      | No      | `<PREFIX>-<random>`, auto-assigned. See [4.7](#47-ticket-id-format). |
| Role                | Yes      | No      | `epic` or `task`. Immutable after creation. |
| Title               | Yes      | Yes     | Must contain at least one alphanumeric character. |
| Description         | No       | Yes     | Free-form text. |
| Acceptance Criteria | No       | Yes     | Free-form text. |
| Priority            | Yes      | Yes     | `P0`–`P4`. Default: `P2`. Lower number = higher urgency. Changing requires claiming. |
| State               | Yes      | Yes     | See [5](#5-state-machines). Tasks start as `open`; epics start as `active`. |
| Revision            | Yes      | No      | Integer; derived from history entry count (`revision = history count − 1`). Starts at `0`. |
| Parent              | No       | Yes     | Reference to a parent epic. Tasks and epics may have a parent epic. |
| Facets              | —        | Yes     | Zero or more key–value pairs. See [4.6](#46-facet). |
| Notes               | —        | —       | Zero or more. See [4.2](#42-note). Managed separately. |
| Relationships       | —        | —       | Zero or more. See [4.3](#43-relationship). |
| Created At          | Yes      | No      | Automatically applied timestamp. |
| Idempotency Key     | No       | No      | Optional opaque string provided at creation. Prevents duplicate creation. |

All mutable fields (except notes and relationships) require claiming to modify.

### 4.2 Note

Notes are comments on a ticket. They can be added at any time by anyone without claiming.

| Field      | Required | Notes |
|------------|----------|-------|
| ID         | Yes      | Auto-assigned sequential integer, displayed as `note-<integer>` (e.g., `note-368`). Global across the database, not scoped per ticket. |
| Ticket ID  | Yes      | The ticket this note belongs to. |
| Author     | Yes      | Explicit parameter. Must pass author validation (see [4.8](#48-author-validation)). |
| Created At | Yes      | Automatically applied timestamp. |
| Body       | Yes      | Free-form text. |

Notes are a flat list — no threading, no `reply_to` field. Conversational context is established by convention (referencing note IDs in body text, topic lines).

### 4.3 Relationship

| Relationship    | Inverse        | Semantics |
|-----------------|----------------|-----------|
| `blocked_by`    | `blocks`       | This ticket cannot make progress until the referenced ticket is closed. |
| `cites`         | `cited_by`     | This ticket references the target ticket as relevant context. |

- Relationships can be added or removed without claiming either ticket.
- Use idempotent semantics: "create if not exists", "delete if exists".
- A relationship cannot reference a deleted ticket.
- Existing relationships pointing to a deleted ticket are retained in storage but invisible to normal operations.

### 4.4 History Entry

Every mutation transaction produces exactly one history entry. See [7](#7-history-and-auditability) for full details.

| Field        | Notes |
|--------------|-------|
| Entry ID     | Auto-assigned. Unique within the ticket's history. |
| Ticket ID    | The ticket this entry belongs to. |
| Revision     | Zero-based index within the ticket's history (`0` = creation). |
| Author       | Inherited from the active claim for gated operations; explicit parameter for ungated operations. |
| Timestamp    | Automatically applied. |
| Event Type   | The kind of mutation — e.g., `created`, `claimed`, `released`, `updated`, `state_changed`, `deleted`. |
| Changes      | Structured delta: which fields changed, with before and after values. Must be sufficient to reconstruct the ticket's state at any revision. |

### 4.5 Claim

A claim represents active ownership of a ticket.

| Field           | Notes |
|-----------------|-------|
| Claim ID        | Random, unguessable token. Bearer authentication for all gated operations. |
| Ticket ID       | The ticket being claimed. |
| Author          | Bound at claim time. All gated mutations inherit this author. |
| Stale Threshold | Duration after which the claim becomes stale. Default: 2 hours. Configurable up to 24 hours. |
| Last Activity   | Timestamp of the most recent gated mutation or note on the claimed ticket. Used for staleness calculation. |

A claim is invalidated when:
- The claimer releases the ticket (close, defer, wait, or release).
- Another agent steals the ticket.

### 4.6 Facet

Facets are key–value pairs on any ticket for filtering and agent coordination.

- Keys and values are short strings. No schema enforcement.
- Keys are unique per ticket — setting an existing key overwrites the previous value.
- Require claiming to add, modify, or remove.
- Queryable: exact key–value match, `key:*` wildcard (matches any value for that key), and optionally negative matching.

**Labels** are a convention on facets, not a first-class field. The recommended facet key for labels is `kind` (e.g., `kind:feat`, `kind:fix`). Users define their own vocabulary.

### 4.7 Ticket ID Format

Ticket IDs have the form `<PREFIX>-<random>` (e.g., `NP-a3bxr`).

- **Prefix**: uppercase, set once at database initialization, immutable. Must be specified by the user — no auto-generation.
- **Random portion**: 5 lowercase Crockford Base32 characters, generated randomly per ticket. ID space: 33,554,432. On collision, regenerate and retry.
- **Case contrast**: uppercase prefix, lowercase random portion, for visual separation.

**SQLite constraint**: the ticket table must be `WITHOUT ROWID` so the declared primary key (the random ID) is the actual B-tree key. This avoids sequential clustering from SQLite's implicit `ROWID`.

### 4.8 Author Validation

All author fields — whether bound to a claim or passed explicitly — must satisfy:

- At least one alphanumeric character.
- Maximum length: 64 Unicode runes (measured after normalization).
- No whitespace — no Unicode whitespace characters.
- **NFC-normalized** — the system normalizes all author strings to Unicode NFC on input.
- Equality and sorting are **case-sensitive**. `"alice"` and `"Alice"` are distinct authors.

---

## 5. State Machines

### 5.1 Task States

| State      | Meaning |
|------------|---------|
| `open`     | Available for work. Default state at creation. |
| `claimed`  | An agent or human has taken ownership; working on it or updating fields. |
| `closed`   | Fully resolved. **Terminal** — cannot be claimed or reopened. |
| `deferred` | Should not be worked on now. |
| `waiting`  | Cannot proceed until something external to the ticket tracker happens. |

### 5.2 Epic States

| State      | Meaning |
|------------|---------|
| `active`   | Live. Children follow their own lifecycles; readiness flows normally. Default state at creation. |
| `claimed`  | An agent or human is editing metadata or decomposing into children. |
| `deferred` | Should not be worked on now. Unclaimed descendants are no longer ready; claimed descendants continue. |
| `waiting`  | Cannot proceed until something external happens. Same readiness propagation as `deferred`. |

Epics have no `closed` state. Epic completion is derived (see [6.2](#62-epic-completion-derivation)).

### 5.3 State Transition Rules

**Claiming is the universal gate for all state changes.** To move a ticket to any new state, you must be its current claimer. The only ungated transition is into `claimed` itself.

```
(any non-terminal state) → claimed    take ownership
claimed → open / active               release without completing
claimed → closed                      complete (tasks only)
claimed → deferred                    shelve
claimed → waiting                     externally blocked
```

`closed` and `deleted` are terminal — they cannot be claimed.

---

## 6. Lifecycle Rules

### 6.1 Claiming

- A ticket must be claimed before its mutable fields can be updated.
- **Exceptions — no claim required:**
  - **Notes** — anyone can add a note to any ticket.
  - **Relationships** — anyone can add or remove relationships.
- `closed` and `deleted` tickets cannot be claimed.
- For quick updates, the CLI supports a one-shot claim → update → release as a single command. The claim ID is generated and immediately invalidated internally.

#### Claim IDs

- Generated randomly and opaque to callers.
- Every command response involving a claimed ticket includes the claim ID.
- Invalidated when the claim ends (release, close, defer, wait, or steal).
- When stolen, a new claim ID is issued to the stealer.

#### Author Attribution

- **Gated operations**: the author is bound to the claim at claim time. All mutations inherit this author; the caller cannot override it per-operation.
- **Ungated operations** (notes, relationships): require an explicit author parameter.

### 6.2 Epic Completion Derivation

Epic completion is derived, never directly set.

- **Complete** when: the epic has children **and** all of them are closed (tasks) or complete (sub-epics).
- **Incomplete** otherwise — including when the epic has no children.

Completion is an observation, not a lock. New children can always be added to a complete epic, which flips it back to incomplete.

### 6.3 Readiness

A task is **ready** when all of the following are true:

1. Its state is `open`.
2. It has no `blocked_by` relationships, **or** every `blocked_by` target has been closed or deleted.
3. No ancestor epic is `deferred` or `waiting`.

Readiness propagates downward — a deferred or waiting epic suppresses readiness for all unclaimed descendants.

### 6.4 Staleness and Stealing

#### Staleness

- A `claimed` ticket becomes **stale** when it has had no updates and no new notes for its stale threshold.
- **Default threshold**: 2 hours.
- **Custom threshold**: configurable at claim time, up to a maximum of 24 hours.
- **Extending**: the claimer can extend the threshold at any time, up to the 24-hour maximum.

#### Stealing

- Stale claimed tickets can be stolen directly by ID, or automatically when no ready tickets are available.
- **Atomic**: the old claim is invalidated and the new claim is created in a single transaction. If two agents race to steal the same stale ticket, exactly one succeeds; the other receives a claim-conflict error.
- When a ticket is stolen, a note is automatically generated using the stealer's claim-bound author (e.g., "Stolen from `<previous-claimer>`.").

### 6.5 Soft Deletion

Deletion is soft — data is retained but invisible to normal operations.

- A deleted ticket's ID is permanently reserved.
- Requesting a deleted ticket by ID returns "not found".
- A deleted ticket is immutable — no further changes.
- A deleted ticket cannot be referenced in new relationships.
- Existing relationships pointing to a deleted ticket are retained in storage but invisible.
- Deleting an epic recursively deletes all its children.
- Deleted tickets cannot be undeleted.
- `gc` can physically remove deleted ticket data.

### 6.6 Duplicate Handling

When a ticket is a duplicate:

- One ticket is closed with a `cites` relationship pointing to the surviving ticket.
- **Preferred heuristic**: keep the ticket with the most complete title, description, and acceptance criteria.
- **Exception**: if the "weaker" ticket has richer interaction history, closing the "stronger" ticket may be more appropriate.
- This is a judgement call, not an automated operation.

---

## 7. History and Auditability

Every mutation transaction produces exactly one history entry. This includes creation, field updates, state transitions, claiming, releasing, and deletion.

History is per-ticket: an ordered, append-only sequence of entries that fully describes the ticket's evolution from creation to its current state.

The data model is event-sourcing compatible. Per-ticket histories can be merged by timestamp to approximate global history, but a first-class global history view is out of scope.

Garbage collection (`gc`) is the only operation that destroys history entries.

---

## 8. Commands

### 8.1 Cross-Cutting Concerns

#### Pagination

All list commands use **keyset pagination**:

- Default page size: **20 items**.
- Response includes the **total count** of matching items.
- Next page: caller passes the last item's sort key and ID. The database seeks directly to that position.
- Stable under concurrent inserts and deletes.

#### Agent Name Generation

A command to generate a readable, random agent name (e.g., "dashing-storage-glitter"). Format follows Docker's auto-generated container name style. Persistence and reuse across sessions are out of scope.

### 8.2 Global Operations

#### Initialize

Create a `.np/` directory and database in the current working directory.

- **Required parameter**: prefix for ticket IDs. Cannot be changed after initialization.

#### Agent Name

Generate a readable, random agent name.

#### Agent Instructions

Generate a concise Markdown block describing how to use `np`, suitable for pasting into agent configuration. The output covers:

- Core workflow: claim → work → transition state.
- How claim IDs work and when to pass them.
- Always move a ticket to an appropriate unclaimed state when done.
- How to discover more: `np --help` and `np <command> --help`.
- Statement that `np` is the exclusive tool for task management — agents must not use built-in task management from their own platform.

The instructions must be brief — enough to get an agent started with pointers for detail.

### 8.3 Ticket Operations

#### Create

Create a ticket. Settable at creation: title, description, acceptance criteria, priority, role (task or epic), parent epic, facets, and relationships.

- Optionally start as claimed (returns a claim ID).
- Optional **idempotency key**: if a ticket with the same key exists, return the existing ticket instead.

#### Claim by ID

Claim a specific ticket. Returns a claim ID.

- If the ticket is already claimed and **stale**, the caller may indicate stealing is allowed. On success, a new claim ID is issued and an auto-generated note is added.
- If claimed and **not stale**, the operation fails.

#### Claim Next Ready

Claim the highest-priority unclaimed ready ticket.

- **Ordering**: lowest `P` number first; ties broken by earliest creation time.
- Filterable by facet.
- **Steal fallback**: if no ready tickets are available, optionally steal the highest-priority stale claimed ticket. Caller must explicitly opt in.

#### Update

Update one or more properties, facets (add/modify/remove), relationships (add/remove), and/or parent assignment. Optionally add a note in the same operation. Requires the claim ID. All changes are a single atomic mutation.

#### Transition State

Change the state of a claimed ticket. Requires the claim ID. Valid transitions from `claimed`:

| Transition | Effect |
|------------|--------|
| Release    | Return to `open` (tasks) or `active` (epics). |
| Close      | Mark as complete. Tasks only. Terminal. |
| Defer      | Shelve. |
| Wait       | Externally blocked. |

All transitions end the claim and invalidate the claim ID.

#### Delete

Soft-delete a claimed ticket. Requires the claim ID. Deleting an epic recursively deletes all children.

#### Show

Display the full current state of a ticket: all fields, facets, relationships, parent, children (epics), and derived properties (readiness, completion status). Notes are excluded — they have their own listing.

#### List

List tickets with high-level information: ID, role, state, priority, title. Optionally include timestamps.

- Filterable by: state, the computed "ready" predicate, facet (`key:value` exact match, `key:*` wildcard, optionally negative matching).
- Orderable by: priority, creation time, modification time.
- Paginated.

#### Search

Full-text search on title, description, and acceptance criteria. Optionally include notes.

- Filterable by: state, facet.
- Orderable and paginated.

### 8.4 Note Operations

#### Add Note

Add a note to a ticket. Does not require claiming. Requires explicit author and body.

#### Show Note

Display a single note by its ID.

#### List Notes

List notes on a specific ticket.

- Filterable by: author, created-after date-time, created-after a specific note ID.
- Orderable and paginated.

#### Search Notes (Per-Ticket)

Full-text search on notes for a specific ticket.

#### Search Notes (Global)

Full-text search across all notes in the database.

- Filterable by: author, created-after date-time, created-after note ID, ticket facets, ticket state.
- Orderable and paginated.

### 8.5 History Operations

#### Show History

Display the change history for a ticket.

- Filterable by: author, date range.
- Orderable and paginated.

### 8.6 Diagnostics

#### Doctor

Diagnostic-only — reports findings without modifying data:

- Circular `blocked_by` relationships (dependency cycles).
- Deadlocked state — all remaining tickets are blocked.
- Stale `claimed` tickets showing no activity.
- Epics with no task descendants (decomposition needed).
- Garbage collection opportunity — if removing deleted (and optionally closed) data would reduce database size by at least **40%**, suggest running `gc`.

#### GC

Physically compact the database by removing deleted (and optionally closed) ticket data.

- Can be targeted: e.g., remove only deleted tickets without discarding closed tickets.
- Not part of normal workflow — available if the database grows unwieldy.

---

## 9. Agent Ergonomics

These requirements apply across all commands.

### Structured Output

Every command supports a **JSON output mode**. JSON is the primary interface for agent callers; human-readable output is a convenience layer.

### Deterministic Output Shape

The JSON structure for a given command is the same regardless of the result:

- Empty list → `[]`, never a missing field or `null`.
- Missing optional field → present with a null or default value, never omitted.

### Predictable Exit Codes

| Code | Meaning |
|------|---------|
| 0    | Success. |
| 1    | General / unexpected error. |
| 2    | Not found (ticket, note, etc.). |
| 3    | Claim conflict (ticket is claimed and not stale, or claim ID mismatch). |
| 4    | Validation error (bad input). |
| 5    | Database error (corruption, locked, etc.). |

### Self-Describing Errors

In JSON mode, error responses include structured context — not just a message string. When a claim fails, the response includes who holds the claim and when it becomes stale. When validation fails, the response identifies which fields are invalid and why.

### No Interactive Prompts

Every operation is completable in a single invocation. No confirmations, no pagers, no editors. Destructive operations use a required flag (e.g., `--confirm`).

### Idempotent Where Natural

- **Relationships**: create-if-not-exists, delete-if-exists.
- **Ticket creation**: optional idempotency key.
- Inherently non-idempotent operations (claiming, adding notes) remain so.

---

## 10. Concurrency and Atomicity

### Concurrency Control

Claiming is the concurrency control mechanism. All mutable ticket fields are gated by exclusive claiming; there is no concurrent modification to handle.

Ungated operations:
- **Notes**: append-only, no contention.
- **Relationships**: idempotent semantics (create-if-not-exists, delete-if-exists).

Claiming and stealing are atomic compare-and-swap operations. The claim succeeds only if the ticket is unclaimed or stale; the loser gets a claim-conflict error. SQLite's transaction isolation handles the CAS naturally.

### Atomicity Guarantees

| Operation Type | Guarantee |
|----------------|-----------|
| **Writes** | A CLI execution that changes the database is atomic. The entire mutation succeeds or fails as a unit (e.g., recursive epic deletion is one transaction). |
| **Single-ticket reads** | Atomic — the ticket is always in an internally coherent state. |
| **Multi-ticket reads** (list, search) | Atomic per ticket, not across the result set. Each ticket is coherent, but cross-ticket relationships may reflect different points in time. |

---

## 11. Out of Scope

- Note threading (no reply chains or nested conversations).
- Multi-machine sync or remote access.
- Agent orchestration or coordination.
- Global history view (data model supports it; feature is deferred).
- Cross-database references or federation.
- Undelete.
