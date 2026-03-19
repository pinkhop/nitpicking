# Nitpicking (`np`) — Product Ideas

> A local-only, non-invasive, single-machine, CLI-driven, no-database-server issue tracker
> designed for AI agent workflows.

Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project.

---

## Table of Contents

1. [Design Principles](#1-design-principles)
2. [Motivation: Deficiencies of Beads](#2-motivation-deficiencies-of-beads)
3. [Ticket Model](#3-ticket-model)
   - 3.1 [Ticket Types](#31-ticket-types)
   - 3.2 [Objective Labels](#32-objective-labels)
   - 3.3 [Common Fields](#33-common-fields)
   - 3.4 [States](#34-states)
   - 3.5 [Relationships](#35-relationships)
   - 3.6 [Notes](#36-notes)
4. [Ticket Lifecycle](#4-ticket-lifecycle)
   - 4.1 [Claiming & Updating](#41-claiming--updating)
   - 4.2 [Objective State Derivation](#42-objective-state-derivation)
   - 4.3 [Readiness](#43-readiness)
   - 4.4 [Duplicate Handling](#44-duplicate-handling)
   - 4.5 [Stale Tickets & Stealing](#45-stale-tickets--stealing)
   - 4.6 [Soft Deletion](#46-soft-deletion)
5. [History & Auditability](#5-history--auditability)
6. [Architecture](#6-architecture)
7. [Commands](#7-commands)
   - 7.1 [Doctor](#71-doctor)
8. [Out of Scope](#8-out-of-scope)
9. [Conflicts & Tensions](#9-conflicts--tensions)
10. [Open Questions for Next Session](#10-open-questions-for-next-session)

---

## 1. Design Principles

- **Local-only** — runs on a single machine; no network, no remote sync.
- **Non-invasive** — no global hooks, no coupling to git lifecycle, no background daemons.
- **No database server** — embedded storage only; no Dolt, no Postgres, no separate process.
- **CLI-driven** — first-class CLI (`np`); designed to be called by AI agents and humans alike.
- **Per-project databases** — each project/product gets its own ticket database.

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

## 3. Ticket Model

### 3.1 Ticket Types

There are exactly two ticket types (referred to as "roles"):

#### Objective

The high-level "what & why". Analogous to an epic or story in agile.

- Optionally has an objective ticket as its parent (nesting is allowed).
- **Cannot** be directly opened or closed — state is derived from children
  (see [4.2](#42-objective-state-derivation)).
- Has a **priority** (P0–P4).
- Optionally carries a **label** (see [3.2](#32-objective-labels)).

#### Task

The low-level "what & how". Describes a step or sequence of steps that progresses the
project — or its parent objective — closer to completion.

- Optionally has an objective ticket as its parent.
- **Cannot** have child tickets of its own (leaf node only).

### 3.2 Objective Labels

Labels classify the nature of an objective. They are optional.

| Label           | Definition |
|-----------------|------------|
| `bug`           | A defect or failure to meet standards: user-facing issues, failing tests, dependencies with known security vulnerabilities, etc. |
| `chore`         | Work that delivers no direct user-visible change but keeps the product healthy or enables future work. |
| `enhancement`   | An improvement to an existing capability — the verb already exists, but it gets better. E.g., "export is now 3× faster", "you can filter the collaborator list by role". |
| `feature`       | A new capability — something the product couldn't do before. Introduces a new verb for the user. E.g., "now you can export reports", "now you can invite collaborators". |
| `spike`         | A time-boxed research or investigation to answer a question or reduce uncertainty. The deliverable is knowledge, not code. E.g., "can we integrate with their API?", "what's causing the intermittent timeout?" |

### 3.3 Common Fields

All tickets — regardless of type — carry these fields:

| Field               | Required | Notes |
|---------------------|----------|-------|
| ID                  | Yes      | Auto-assigned, short alphanumeric, **immutable**. |
| Project ID          | Yes      | **Immutable.** Defaults to the parent ticket's value. Encoding TBD (see [Open Questions](#9-open-questions-for-next-session)). |
| Title               | Yes      | Must contain at least one alphanumeric character. |
| Description         | No       | |
| Acceptance Criteria | No       | |
| Revision            | Yes      | Integer; starts at `0`, increments on every update. |
| State               | Yes      | Starts as `open`. See [3.4](#34-states). |
| Notes               | —        | Zero or more. See [3.6](#36-notes). |
| Relationships       | —        | Zero or more. See [3.5](#35-relationships). |

### 3.4 States

| State      | Meaning |
|------------|---------|
| `open`     | Available for work. |
| `claimed`  | An agent or human has claimed the ticket to work on it or update its fields. |
| `closed`   | Fully resolved. |
| `deferred` | Should not be worked on now. For objectives, may represent a stretch goal or a future roadmap item. |
| `waiting`  | Cannot proceed until something **external to the ticket tracker** happens — e.g., a human decision, a permission grant. (Name is provisional; "blocked" is explicitly rejected.) |

### 3.5 Relationships

| Relationship  | Semantics |
|---------------|-----------|
| `blocked_by`  | This ticket cannot make progress until the referenced ticket is closed. |
| `see_also`    | This ticket may be related to the referenced ticket. (Name is provisional; "relates_to" is explicitly rejected.) |

### 3.6 Notes

Notes are comments on a ticket. They can be added **at any time by anyone** without
claiming the ticket.

| Field      | Required | Notes |
|------------|----------|-------|
| ID         | Yes      | Auto-assigned. Tentative format: `<ticket-id>-<random-6-alphanumeric>`. Exists so notes can reference other notes. |
| Author     | Yes      | Must contain at least one alphanumeric character. |
| Created At | Yes      | Automatically applied timestamp. |
| Body       | Yes      | Free-form text. |

---

## 4. Ticket Lifecycle

### 4.1 Claiming & Updating

- A ticket **must be claimed** before its fields (other than notes and relationships)
  can be updated.
- Notes and relationships can be added **without** claiming.

### 4.2 Objective State Derivation

Objective tickets do not have directly settable open/closed states. Instead:

- **Open** when: it has no child tickets (needs to be "tasked out"), **or** any of its
  children are open.
- **Closed** when: all of its child tickets are closed.

### 4.3 Readiness

A ticket is **ready** when:

1. Its state is `open`, **and**
2. It has no `blocked_by` relationships, **or** every `blocked_by` target has been
   closed or deleted.

### 4.4 Duplicate Handling

When a ticket is determined to be a duplicate:

- One ticket is closed with a `see_also` relationship pointing to the surviving ticket.
- **Preferred heuristic:** keep the ticket with the most complete and useful title,
  description, and acceptance criteria.
- **Exception:** if the "weaker" ticket has the richer interaction history (notes,
  relationships, etc.), closing the "stronger" ticket may be more appropriate.
- This is a judgement call.

### 4.5 Stale Tickets & Stealing

- A `claimed` ticket can become **stale** (criteria TBD — tentatively: no updates and
  no new notes for 3 hours).
- Stale claimed tickets can be **stolen**:
  - Directly by ID, or
  - Automatically when there are no ready tickets available.

### 4.6 Soft Deletion

Deletion is a soft removal. A deleted ticket is treated as though it does not exist,
with the following exceptions and rules:

- A deleted ticket's **ID is permanently reserved** — it cannot be reused.
- Deleted tickets appear in list/detail views **only** when the request explicitly opts
  in to showing deleted tickets. Without the opt-in flag, requesting a deleted ticket by
  ID returns a "not found" error.
- A deleted ticket is **immutable** — no further changes of any kind.
- A deleted ticket **cannot be referenced** in new relationships.
- Existing relationships pointing to a deleted ticket are **ignored / not displayed**.
  (Whether to physically remove these relationships or merely hide them is an
  implementation decision — see [Open Questions](#9-open-questions-for-next-session).)
- Deleting an objective ticket **deletes all its children recursively**.

---

## 5. History & Auditability

- **All changes** to a ticket are recorded, including ticket creation.
- All changes are **atomic**.
- Each change captures: **author** and **timestamp**.

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

#### Development Strategy

This architecture enables a deliberate sequencing of work:

1. **First: Core domain** — implement the ticket model, state machine, lifecycle rules,
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

### 7.1 Doctor

A `doctor` command that diagnoses problems in the ticket database. Examples:

- **Circular `blocked_by` relationships** — dependency cycles.
- **Deadlocked state** — all remaining tickets are blocked (e.g., on `claimed`,
  `deferred`, or `waiting` tickets).
- **Stale `claimed` tickets** — tickets that have been claimed but show no activity
  (threshold TBD; tentatively 3 hours with no updates or new notes).

---

## 8. Out of Scope

- **Note threading** — notes are a flat list; no reply chains or nested conversations.

---

## 9. Conflicts & Tensions

The following areas contain ideas that may conflict with each other or with the stated
design principles. They should be resolved before implementation.

### 9.1 Objective State: Derived vs. Explicit

Objectives derive their open/closed state from children, yet objectives also participate
in the full state model (`open`, `claimed`, `deferred`, `waiting`). This raises
questions:

- Can an objective be `deferred` or `waiting` independently of its children's states?
- If an objective is `deferred`, what happens to its open children — are they implicitly
  deferred too, or can they still be worked?
- If an objective is `waiting`, does that block its children from being claimed?
- "Cannot be directly opened or closed" needs to be reconciled with the claim that state
  "starts as `open`" — who or what sets an objective to `deferred` or `waiting` if not a
  direct state change?

### 9.2 Claiming Objectives

If objectives cannot be directly opened or closed, can they be `claimed`? The claiming
rule says a ticket must be claimed to update its fields, but objectives have fields
(title, description, acceptance criteria, priority, label) that might need updating.
Either objectives need to be claimable despite not being directly openable/closable, or
there needs to be an exception for objectives.

### 9.3 Per-Project Database vs. Cross-Project Tickets

The design principle says "each project/product gets its own ticket database", but the
open question about multi-project support describes objectives whose tasks span multiple
repos. These two ideas are in tension:

- If each repo has its own database, a cross-cutting objective can't have children in
  other databases.
- If a single database covers multiple projects, the "per-project" principle is relaxed.

### 9.4 Project ID Semantics

Project ID is described as "required; immutable; defaults to parent's value" but also
"needs more thinking through". The tentative encoding (`<project-id>-<ticket-id>`) would
bake the project identity into the ticket ID itself — which is clean for single-project
use but complicates the multi-project scenario.

### 9.5 Soft Deletion: Relationship Cleanup

The spec says existing relationships to deleted tickets are "ignored / not shown" but
acknowledges uncertainty about whether to physically remove them ("removing them might
require a lot of locks and a big transaction"). This is an implementation concern, but it
also has semantic implications: if relationships are kept but hidden, `doctor` might need
to account for phantom edges when detecting cycles or deadlocks.

### 9.6 Task State vs. Objective Derivation

Tasks can be `deferred` or `waiting`, but these states affect the parent objective's
derived state. If all children of an objective are `deferred`, is the objective open
(because no child is closed, so "not all children are closed") or effectively deferred
itself? The derivation rules only address open vs. closed — they don't account for
`deferred` or `waiting` children.

---

## 10. Open Questions for Next Session

### Ticket Identity & Project Scoping

1. **ID format** — what specific alphanumeric scheme? Length? Case sensitivity?
   Collision avoidance strategy?
2. **Project ID encoding** — separate field vs. baked into the ticket ID
   (`<project-id>-<ticket-id>`)? How does the system discover which
   directory/repo/worktree maps to a given project?
3. **Single vs. multi-project databases** — should the design commit to one model, or
   support both? What are the trade-offs for the "parent directory covering multiple
   repos" use case?

### State Machine

4. **Full state transitions** — what are the legal state transitions for tasks?
   E.g., can a `closed` ticket be reopened? Can a `deferred` ticket go directly to
   `claimed`?
5. **Objective states beyond open/closed** — how do `deferred`, `waiting`, and `claimed`
   interact with the derived-state model for objectives? (See
   [Conflict 9.1](#91-objective-state-derived-vs-explicit).)
6. **Claiming objectives** — are objectives claimable? If so, what does that mean given
   their state is derived? (See [Conflict 9.2](#92-claiming-objectives).)

### Deletion

7. **Relationship cleanup on deletion** — hide or physically remove? What are the
   implications for `doctor` and for storage growth over time?
8. **Undelete** — should soft-deleted tickets be restorable, or is deletion final
   (just soft for ID reservation and optional visibility)?

### Staleness & Stealing

9. **Stale threshold** — 3 hours is tentative. Should this be configurable? Should it
   differ for human vs. agent authors?
10. **Steal semantics** — when a ticket is stolen, what happens to the previous
    claimant's in-progress work? Is a note automatically added? Is the history updated?

### Metadata / Key-Value Pairs

11. **Structured metadata** — the idea of short key-value pairs
    (e.g., `jira:FOO-152`, `repo:https://...`, `lang:typescript`) is appealing but risks
    scope creep. Should this be a first-class feature, a free-form tag bag, or deferred
    entirely?
12. **Well-known keys** — if metadata is supported, should certain keys
    (`repo`, `dir`, `git:worktree`) have defined semantics, or is everything free-form?

### Labels

13. **Additional objective labels** — are there other labels beyond `bug`, `chore`,
    `enhancement`, `feature`, and `spike` that would be valuable? Candidates to
    consider: `debt` (technical debt), `security`, `documentation`, `deprecation`.
14. **Labels on tasks** — should tasks also carry labels, or are labels strictly an
    objective concern?

### Architecture & Ports (Deferred Adapter Decisions)

Per the [hexagonal architecture](#6-architecture) decision, CLI structure and SQLite
schema are deferred until the core domain and port interfaces are solid. The questions
below apply once adapter work begins.

15. **Driven port contract** — what operations must the persistence port expose? This
    falls out of the core domain implementation but is worth enumerating early: CRUD for
    tickets/notes/relationships, history queries, readiness queries, soft-deletion
    semantics, etc.
16. **Concurrency model** — if multiple agents or terminals operate on the same database
    simultaneously, what locking or conflict-resolution strategy applies? This is partly
    a core concern (claiming semantics) and partly an adapter concern (SQLite locking).
17. **Atomicity guarantees** — "all changes are atomic" — what defines the transaction
    boundary? Single field update? Entire ticket mutation? Recursive deletion? The core
    defines the logical boundary; the adapter must honour it.

### CLI Design (Deferred)

18. **Command vocabulary** — beyond `doctor`, what commands are needed? Likely
    candidates: `create`, `show`, `list`, `claim`, `update`, `close`, `delete`,
    `note`, `link`, `steal`, `search`. Deferred until the driving port is defined.
19. **Output formats** — should the CLI support structured output (JSON) for agent
    consumption alongside human-readable output?
20. **Agent ergonomics** — what makes a CLI "designed for AI agent workflows"
    specifically? Predictable exit codes? Machine-readable output? Idempotent
    operations?

### SQLite Schema (Deferred)

21. **Schema design** — deferred until the driven port interface is defined. The schema
    should be derived from the port contract, not the other way around.
