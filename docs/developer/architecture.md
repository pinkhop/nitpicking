# Architecture

## 1. Overview

This document describes how Nitpicking applies hexagonal architecture
(Ports & Adapters) to its codebase. It is the authoritative reference
for architectural layering, dependency direction, and package placement
decisions in this project.

**This is not a canonical reference architecture.** The model described
here is tailored to the constraints and priorities of a local-only,
single-machine, CLI-driven issue tracker. Other projects applying
hexagonal architecture may make different trade-offs — that does not
make either approach wrong.

When CLAUDE.md or any other project document conflicts with this file
on architectural matters, this file governs.

## 2. Mental Model

The architecture has seven conceptual layers. The numbering implies
dependency direction (lower-numbered layers never depend on
higher-numbered layers), but it does not imply call direction — calls
flow inward from the driving adapter through the driving port to the
core, and outward from the core through driven ports to driven adapters.

### The layers

**(a) Domain entities** — pure business vocabulary. Issue, Claim,
Comment, Priority, Role, State, and their associated value types. No
I/O, no side effects, no knowledge of persistence or presentation. The
domain defines what the system *is*; everything else defines what the
system *does*.

**(b) Driving ports** — the public API of the core, expressed as an
interface whose methods accept and return domain entities and DTOs. This
is the boundary that driving adapters (CLI, TUI, REST) program against.
A driving port declares *what the application can do* without revealing
*how* it does it.

**(c) Core** — implements the driving port interface. This is where
business logic lives: use-case orchestration, transaction scoping,
state-machine enforcement, claim validation, readiness computation. The
core depends on driven ports for persistence but never on a specific
adapter.

**(d) Driven ports** — abstraction boundaries that the core programs
against for capabilities it does not own. Two motivations justify a
driven port:

1. **External dependencies the project does not control.** If the
   project relied on a third-party API, that dependency would be modeled
   as a driven port so the core is not coupled to the provider's SDK or
   wire format.

2. **Natural component boundaries where substitutability is valuable.**
   Storage in this project is *not* an external dependency — we control
   the SQLite schema, the queries, and the data model end to end.
   Nevertheless, we model it as a driven port because doing so yields
   concrete benefits: the core can be tested with an in-memory adapter
   (fast, no I/O), the storage adapter can be swapped without touching
   business logic, and the port interface itself documents the
   persistence contract explicitly rather than leaving it implicit in
   scattered SQL.

Both motivations are equally valid. The key question is not "do we
control it?" but "does the core benefit from being decoupled from it?"

**(e) Driven adapters** — concrete implementations of driven ports.
Each driven port should have at least two adapters: one for production
use and one for testing. In this project, SQLite is the production
storage adapter and an in-memory fake serves as the test adapter. The
JSONL adapter implements the backup/restore port. A driven adapter
imports the driven port interface and the domain types it needs for
translation; it never imports the core or any driving adapter.

**(f) Driving adapters** — the user-facing layer. In this project, the
CLI (`np`) is the sole driving adapter today. A driving adapter imports
the driving port interface and invokes its methods. It does *not* import
driven ports or driven adapters. Driving adapters *do* import domain
value types — enums, identifiers, and vocabulary types — because those
types are part of the driving port's method signatures. This is a
correct inward dependency, not a layer violation.

**(g) Configurator** — the cross-layer wiring that assembles the
application from its parts. The configurator is the one place allowed to
know about every layer: it constructs driven adapters, injects them into
the core, and hands the resulting driving port implementation to the
driving adapter. It is not a layer in the architectural sense — it is a
pragmatic necessity that exists outside the dependency rules.

The configurator exposes assembly (e.g., a factory or a `New` function),
not `Main()`. Each executable owns its own entry point; the
configurator's job ends once the object graph is constructed.

### The library test

The driving port, core, and driven ports could be extracted as a
standalone Go module with the driving port as its public API. If that
extraction would require pulling in CLI flags, SQLite imports, or other
adapter-specific types, the abstraction boundary has been violated.

This thought experiment — "could I ship the core as a library?" — is
the litmus test for whether inward-facing code is truly adapter-free.
You do not need to actually extract a module; just verify that the
import graph would permit it.

### The in-memory adapter test

If the in-memory adapter cannot satisfy the driven port interface
without importing implementation-specific types (SQLite connection
handles, query builders, etc.), the port abstraction is broken. A
well-designed driven port is defined entirely in terms of domain entities
and standard library types.

The in-memory adapter is not a mock — it is a fully functional,
behaviorally correct implementation of the driven port that happens to
use maps and slices instead of SQL. Its existence keeps the port
abstraction honest: every time a new method is added to a driven port
interface, the in-memory adapter must implement it. If that
implementation feels unnatural or requires importing adapter-specific
types, the port's method signature needs rethinking.

## 3. Dependency Rules

The dependency rules form a directed acyclic graph. Each row lists
what a layer is allowed to import. Anything not listed is prohibited.

| Layer | May import |
|---|---|
| **(a) Domain entities** | Standard library only. No other project packages. |
| **(b) Driving ports** | Domain entities. |
| **(c) Core** | Domain entities, driving ports (the interface it implements), driven ports (the interfaces it calls). |
| **(d) Driven ports** | Domain entities, standard library types. |
| **(e) Driven adapters** | Domain entities, driven ports (the interfaces they implement). External libraries specific to the adapter (e.g., SQLite driver). |
| **(f) Driving adapters** | Driving ports (the interface they call), domain entities (value types that appear in the driving port's API). |
| **(g) Configurator** | Everything. This is the sole exception to the dependency rules. |

### Key constraints

- **Domain imports nothing outside itself.** Domain packages may import
  other domain packages and the standard library, but never packages
  from any other layer.

- **The core never imports adapters.** The core depends on driven port
  *interfaces*, not on their implementations. Wiring a concrete adapter
  to the core is the configurator's job.

- **Driving adapters never import driven ports or driven adapters.**
  A CLI command has no business knowing which database engine backs the
  application. It calls the driving port; the core handles the rest.

- **Driving adapters importing domain value types is correct.** When a
  driving port method accepts `domain.Priority` or returns
  `domain.State`, the driving adapter must import those types to call the
  method and interpret the result. The dependency rule is about
  direction (always inward), not about total isolation from domain
  vocabulary. Forbidding this import would force the driving port to
  flatten all domain types to strings, losing type safety for no
  architectural benefit.

- **Nothing imports the configurator.** The configurator is called from
  `main()` and hands off a fully assembled object graph. No layer
  depends on it.

- **Adapters do not import each other.** The SQLite adapter does not
  know the in-memory adapter exists, and vice versa. Driving adapters do
  not import driven adapters. Each adapter is an independent leaf in the
  dependency graph.

### Visual summary

```
                ┌─────────────┐
                │   main()    │
                └──────┬──────┘
                       │ calls
                       ▼
              ┌─────────────────┐
              │  Configurator   │──────── imports everything
              └────────┬────────┘
          ┌────────────┼────────────┐
          ▼            ▼            ▼
   ┌─────────────┐ ┌──────┐ ┌─────────────┐
   │   Driving   │ │ Core │ │   Driven    │
   │   Adapter   │ │      │ │   Adapter   │
   └──────┬──────┘ └──┬───┘ └──────┬──────┘
          │            │            │
          │    ┌───────┴───────┐    │
          │    ▼               ▼    │
          │ ┌──────────┐ ┌────────┐│
          │ │ Driving  │ │ Driven ││
          │ │  Port    │ │  Port  ││
          │ └────┬─────┘ └───┬────┘│
          │      │           │     │
          │      ▼           ▼     │
          │   ┌─────────────────┐  │
          └──▶│ Domain Entities │◀─┘
              └─────────────────┘
```

Arrows point in the direction of dependency (from importer to imported).
All dependencies point inward — toward the domain at the center.

## 4. Package Map

This section describes the package layout.

```
cmd/
  np/                          CLI entry point — calls wiring.NewCore(), builds the
                               root command, executes it, classifies errors into
                               exit codes, owns os.Exit().

internal/
  wiring/                      Configurator — constructs Factory, wires dependencies.
                               Does not build CLI commands or classify errors.

  core/                        Core implementation — serviceImpl and all business
                               logic. Orchestrates domain operations through the
                               driven port (Transactor). Implements driving.Service.
  ├─ claim_rules.go            Claim validation rules (ValidateClaim, IssueClaimStatus)
  │                            and steal-comment generation (StealComment).
  ├─ parent_rules.go           Hierarchy validation rules (ValidateParent,
  │                            ValidateDepth, ValidateEpicDepth, ValidateNoCycle,
  │                            AncestorLookup, MaxDepth).

  cmd/                         Driving adapter — CLI commands. Each subpackage owns
  │                            one command or command group.
  ├─ root/                     Root command assembly, signal handling, help template
  ├─ create/                   np create
  ├─ show/                     np show
  ├─ list/                     np list
  ├─ search/                   np issue search
  ├─ claim/                    np claim <ISSUE-ID|ready>
  ├─ done/                     np close
  ├─ issuecmd/                 np issue (release, defer, undefer, reopen, delete, history, orphans)
  ├─ epiccmd/                  np epic (status, close-completed, children)
  ├─ comment/                  np comment (list, search)
  ├─ relcmd/                   np rel (add, list, tree, cycles, blocks, refs, parent, graph)
  ├─ labelcmd/                 np label (add, remove, list, list-all, propagate)
  ├─ blocked/                  np blocked
  ├─ ready/                    np ready
  ├─ jsoncmd/                  np json (structured stdin-based interface)
  ├─ formcmd/                  np form (interactive create, update, comment)
  ├─ importcmd/                np import (currently: jsonl)
  ├─ backupcmd/                np admin backup
  ├─ restorecmd/               np admin restore
  ├─ graphcmd/                 np rel graph
  │  ├─ graph_dot.go           Graph rendering (Graphviz DOT) from GraphNode/GraphEdge.
  │  └─ graph_json.go          Graph rendering (JSON) from GraphNode/GraphEdge.
  ├─ admincmd/                 np admin (backup, completion, doctor, gc, reset, restore, tally, upgrade, where)
  ├─ doctor/                   np admin doctor (shared with admincmd)
  ├─ gc/                       np admin gc
  ├─ tally/                    np admin tally
  ├─ agent/                    np agent (name, prime)
  │  └─ instructions.go        Agent instructions (prime output).
  ├─ init/                     np init
  ├─ where/                    np admin where
  ├─ version/                  np version
  ├─ completion/               np admin completion
  ├─ delete/                   np issue delete
  └─ historyview/              np issue history (rendering)

  cmdutil/                     CLI infrastructure — Factory (DI container),
                               typed errors (ErrSilent, FlagError), flag helpers,
                               output formatting, tracker construction.

  domain/                      Domain entities — pure business vocabulary.
  │                            Single flat package except history/.
  ├─ issue.go                  Issue entity — the central aggregate.
  ├─ id.go                     Issue ID (prefix + Crockford Base32 random).
  ├─ role.go                   Role enum (task, epic).
  ├─ state.go                  State enum and transition rules.
  ├─ secondary_state.go        Secondary state (active, completed, deferred).
  ├─ priority.go               Priority enum (P0–P4).
  ├─ label.go                  Label and LabelSet value types.
  ├─ relationship.go           Relationship and RelationType (blocked_by, blocks, refs).
  ├─ readiness.go              BlockerStatus, AncestorStatus (value types for
  │                            driven port readiness contracts).
  ├─ completion.go             ChildStatus (value type for driven port completion
  │                            contracts).
  ├─ deletion.go               DescendantInfo (value type for driven port deletion
  │                            contracts).
  ├─ claim.go                  Claim entity — bearer-authenticated issue ownership.
  ├─ comment.go                Comment entity — immutable, no claiming required.
  ├─ author.go                 Author value type (NFC-normalized).
  ├─ agentname.go              Agent name generation (Docker-style random names).
  ├─ backup.go                 Backup data types (Header, IssueRecord, etc.) — the
  │                            contract between service logic and backup adapters.
  ├─ import_*.go               JSONL import types and functions — RawLine,
  │                            ValidatedRecord, ValidationResult, ParseError,
  │                            LineError (value types), plus Parse and Validate
  │                            (pure transformations of domain vocabulary).
  ├─ resetkey.go               Database reset key — 128-bit Crockford Base32 token
  │                            generation and SHA-512 verification.
  ├─ errors.go                 Typed domain errors (ValidationError,
  │                            ClaimConflictError, DatabaseError) and sentinel
  │                            errors (ErrNotFound, ErrIllegalTransition, etc.).
  └─ history/                  History Entry — append-only audit trail. Retains its
                               own package because its EventType/Detail model is a
                               self-contained subsystem with distinct lifecycle
                               semantics.

  adapters/
  └─ driven/
     ├─ backup/
     │  └─ jsonl/              JSONL driven adapter — backup/restore serialization.
     │                         Reader/Writer for JSON Lines format.
     └─ storage/
        ├─ memory/             In-memory driven adapter — implements all driven port
        │                      interfaces with maps, slices, and sync.RWMutex.
        │                      Behaviorally correct, not a mock.
        └─ sqlite/             SQLite driven adapter — implements all port interfaces.
                               Pure-Go SQLite (zombiezen), WAL mode, connection pool,
                               BEGIN IMMEDIATE for writes. Handles schema creation,
                               FTS indexing, database discovery (.np/ walk-up).

  iostreams/                   Terminal I/O abstraction — stdin/stdout/stderr with
                               TTY detection, ANSI color control, terminal width.

test/
  blackbox/                    Blackbox component tests — exercise the compiled binary.
  └─ testdata/                 Fixture data for blackbox component tests.
```

## 5. What Goes Where — Decision Guide

Use this table to determine the correct package for new code.

### Types and values

| You are adding… | Package | Why |
|---|---|---|
| A new enum with a closed set of business values (e.g., a new state, role, or priority level) | `domain/` | Domain vocabulary; no I/O, no dependencies outside domain. |
| A new value type that identifies or describes a business concept (e.g., a tag type, a duration policy) | `domain/` | Same rationale as enums — pure business vocabulary. |
| A new entity with its own lifecycle (creation, validation, state transitions) | `domain/` | Entities are domain concepts. If the entity has a rich enough internal model (like `history/`), it may warrant its own subpackage. |
| A struct that crosses the driving port boundary as input or output (e.g., `CreateIssueInput`, `IssueListItemDTO`) | `ports/driving/` | DTOs belong to the port that defines them. Driving adapters and the core both import these types. |
| A query/filter type consumed by a driven port method (e.g., `IssueFilter`, `IssueOrderBy`) | `ports/driven/` | Filter types are part of the persistence contract, not business vocabulary. |
| An error type that adapters need to match on for control flow (e.g., `ClaimConflictError`, `ValidationError`) | `domain/` | Typed errors are domain vocabulary. Adapters use `errors.Is` / `errors.As` to classify them. |

### Functions and methods

| You are adding… | Package | Why |
|---|---|---|
| A new use-case that orchestrates multiple domain operations within a transaction | `core/` | Use-case orchestration is the core's job. The driving port interface gets a new method; the core implements it. |
| A new repository method (CRUD, query, aggregate) | Driven port interface in `ports/driven/`; implementation in each adapter (`sqlite/`, `memory/`) | The port declares the contract; adapters fulfill it. Both adapters must be updated together. |
| A new CLI command or subcommand | `cmd/<name>/` under the driving adapter tree | CLI commands are driving adapter code. They call the driving port; they never import driven ports or adapters. |
| Shared CLI infrastructure (flag parsing, output formatting, error types for the CLI layer) | `cmdutil/` | Cross-cutting CLI concerns that multiple commands need but that are not business logic. |
| Validation logic that enforces a business rule (e.g., "an epic cannot be closed directly") | `domain/` or `core/` | If the rule is intrinsic to the entity, it belongs in the domain. If it requires cross-entity coordination (checking children, blockers), it belongs in the core. |
| A pure function that transforms domain data for presentation (e.g., graph rendering, Markdown formatting) | The driving adapter that consumes it (e.g., `cmd/graphcmd/`); `cmdutil/` if shared across multiple commands | Presentation-specific rendering belongs with its consumer — a web adapter would use D3.js, a TUI would use box-drawing characters. Only truly adapter-agnostic transformations belong in `domain/`. |

### Packages and modules

| You are adding… | Package | Why |
|---|---|---|
| A new driven adapter (e.g., a PostgreSQL storage backend, an S3 backup adapter) | `adapters/driven/<category>/<implementation>/` | Each adapter is an independent leaf. It imports the driven port interface and domain types; nothing imports it except the configurator. |
| A new driving adapter (e.g., a TUI, a REST API) | `cmd/<adapter>/` with its own entry point under `cmd/<binary>/` | Each driving adapter programs against the driving port. It gets its own `main()` under `cmd/`. |
| A new domain subpackage | Only when the concept has a rich internal model that would clutter the main `domain/` package (like `history/`). Otherwise, add to `domain/` directly. | Minimize package proliferation. One large, cohesive domain package is better than many tiny ones. |
| Test helpers and fakes shared across test packages | `internal/adapters/driven/storage/memory/` (for the in-memory driven adapter) or `internal/testutil/` (for general test helpers) | Centralized test infrastructure prevents duplication and keeps test files focused on scenarios. |

### Decision tree for ambiguous cases

When the table above does not clearly answer the question, walk through
these checks in order:

1. **Does it depend on I/O, a database driver, or an external library?**
   → It is an adapter. Place it in the appropriate adapter package.

2. **Does it define or implement a contract between the core and an
   adapter?** → It is a port. Place it in `ports/driving/` or
   `ports/driven/` depending on which direction the dependency flows.

3. **Does it express business vocabulary, rules, or invariants with no
   dependency on adapters or ports?** → It is domain. Place it in
   `domain/`.

4. **Does it orchestrate multiple domain operations, enforce
   cross-entity rules, or manage transactions?** → It is core logic.
   Place it in `core/`.

5. **Does it serve the CLI specifically (flag parsing, output
   formatting, terminal interaction)?** → Place it in `cmdutil/` or the
   specific command package.

6. **None of the above?** → Start with the domain. It is easier to move
   code outward (domain → core → adapter) than inward. If the code
   later acquires adapter dependencies, that is the signal to relocate
   it.

## 6. Anti-Patterns

Each entry below describes a concrete mistake that has occurred in this
project or that the architecture is specifically designed to prevent.
The goal is not a checklist to memorize but an understanding of *why*
each pattern is harmful — so you can recognize novel variations.

### Flattening domain enums to strings at the driving port boundary

**What happened.** Early in the project, driving port DTOs used domain
enum types (`domain.Role`, `domain.Priority`, `domain.State`,
`domain.RelationType`, `domain.SecondaryState`) directly in their field
signatures. When the project decided to decouple driving adapters
from driven port types, one proposed approach was to flatten
*all* enum types to strings at the driving port boundary — including
domain enums that driving adapters legitimately need.

**Why it is wrong.** Domain enums are business vocabulary, not
implementation details. A `Role` is "task" or "epic" regardless of
whether the storage backend is SQLite, PostgreSQL, or an in-memory map.
Flattening domain enums to strings at the driving port boundary destroys
type safety without any architectural benefit. The driving adapter loses
compile-time validation of its inputs, and the service must parse
strings that were already well-typed — adding runtime failure modes that
did not previously exist.

**The principle.** The goal of hexagonal architecture is to hide *driven
port* types from the driving adapter — not to hide *domain* vocabulary.
Domain types flow freely across all inward-facing boundaries. When you
find yourself converting a domain type to a string (or vice versa) at
the driving port, ask: "Am I hiding an implementation detail, or am I
discarding business semantics?" If the latter, keep the domain type.

### Driven ports under domain/

**What happened.** The driven port interfaces (`port.Repository`,
`port.Transactor`, etc.) were placed under `internal/domain/port/`.
This made `port` a subpackage of `domain`, implying that persistence
contracts are business vocabulary.

**Why it is wrong.** Driven ports are *not* domain concepts. They define
the contracts that the core needs from its infrastructure — repository
methods, transaction boundaries, query filters. These contracts exist to
*serve* the core, but they are not part of the business vocabulary that
the domain layer defines. Placing them under `domain/` conflates two
responsibilities: "what the business is" and "what the core needs from
its environment".

**The principle.** A package's location in the tree communicates its
architectural role. When a persistence interface lives under `domain/`,
every reader — human and AI — must override that signal with special
knowledge that "this one is different". Moving driven ports to their own
top-level location (`ports/driven/`) makes the architecture
self-documenting. If you are unsure whether a type belongs in the domain
or in a port, apply the library test: if the type would need to exist
even in a project that had no persistence layer at all, it is domain. If
it exists because the core needs to talk to storage, it is a port.

### Driving adapters importing driven port types or adapter packages

**What it looks like.** A CLI command importing the SQLite package to
call a repository method directly, or importing a driven port interface
to pass it around outside the core.

**Why it is wrong.** The driving adapter's job is to translate user
intent into driving port calls and translate driving port responses into
user-visible output. If a CLI command reaches past the driving port to
touch driven ports or driven adapters, it bypasses the core — which
means business logic is either duplicated in the adapter or skipped
entirely. The driving port exists precisely to be the only thing the
driving adapter talks to.

**The principle.** The driving adapter should be replaceable without
touching the core, and the driven adapter should be replaceable without
touching the driving adapter. Any import from a driving adapter to a
driven port or driven adapter violates both properties.

### Core importing driving adapter types

**What it looks like.** The core (service implementation) importing a
type defined in a CLI command package — for example, a flag enum, an
output format, or a rendering option.

**Why it is wrong.** The core must be adapter-agnostic. If the core
imports a CLI type, it cannot be used from a different driving adapter
(TUI, REST, gRPC) without pulling in CLI dependencies. This inverts the
dependency direction: the core should define *what* information it
needs (via the driving port's DTOs), and the driving adapter should
translate its adapter-specific inputs into those DTOs.

**The principle.** Dependencies always point inward. If you find
yourself importing from an outer layer into an inner one, the type
you need either already exists in an inner layer, or it should be
created there.

### Business logic in driving adapters

**What it looks like.** A CLI command that checks whether an issue can
be closed based on its children's states, applies validation rules, or
computes readiness — instead of calling a driving port method that
encapsulates those rules.

**Why it is wrong.** Business logic in a driving adapter is invisible
to other driving adapters. If a future TUI or REST API needs the same
behavior, it must re-implement it — or, more likely, get it subtly
wrong. The logic cannot be unit-tested without the CLI framework, and
changes to business rules require touching adapter code.

**The principle.** The driving adapter translates between the user's
world and the driving port. It does not make business decisions. If a
CLI command contains an `if` statement that encodes a business rule,
that rule belongs in the core.

### Business logic in driven adapters

**What it looks like.** A SQLite repository method that filters results
based on business rules (e.g., "only return issues that are ready"),
computes derived state, or enforces invariants — beyond what the port
interface's contract specifies.

**Why it is wrong.** Driven adapters are mechanical translators: they
convert port method calls into adapter-specific operations and convert
adapter-specific results back into domain types. If a SQLite adapter
embeds a business rule, the in-memory adapter must independently
re-implement it — and the two implementations may diverge. The core
becomes dependent on adapter behavior it cannot see or control.

**The principle.** If a driven adapter's implementation requires
understanding *why* the core wants something (not just *what* it
wants), the logic belongs in the core. The adapter should translate
faithfully; the core should decide what to ask for.

### Configurator owning executable-specific logic

**What it looks like.** The configurator (wiring layer) parsing
command-line arguments, handling OS signals, formatting error messages
for the terminal, or calling `os.Exit()`.

**Why it is wrong.** The configurator's job is assembly: construct
driven adapters, construct the core, hand the result to the driving
adapter. If it also owns argument parsing or signal handling, it
becomes coupled to a specific executable's concerns — which means a
second executable (e.g., `np-tui`) cannot reuse the same assembly
logic without inheriting unwanted behavior.

**The principle.** Each executable owns its own entry point and its own
executable-specific concerns. The configurator exposes a factory or
constructor that returns an assembled object graph. `main()` calls the
configurator; the configurator does not call `main()`-level concerns.

**Current state.** The codebase follows this principle.
`wiring.NewCore(appName)` returns the assembled Factory without
executing anything. `cmd/np/main.go` owns the CLI-specific lifecycle:
building the root command, running it, classifying errors, and calling
`os.Exit`. A hypothetical `cmd/np-tui/main.go` would call the same
`wiring.NewCore(appName)` but wire the Factory into a TUI driving
adapter instead of the CLI command tree.

## 7. The In-Memory Adapter Test

Section 2 introduced the in-memory adapter test as a litmus test: if
the in-memory adapter cannot satisfy the driven port interface without
importing implementation-specific types, the port abstraction is broken.
This section explains *why* maintaining two driven adapters matters and
how the discipline plays out in practice.

### Why two adapters

A driven port interface is an abstraction. Abstractions are only as
honest as the implementations that exercise them. With a single adapter
(SQLite), the port's method signatures can silently accrete
implementation-specific assumptions — a parameter type that only makes
sense with SQL, a return shape that mirrors a specific query's result
set, a method that conflates two operations because they happen to be
one SQL statement.

The in-memory adapter is the counterweight. It implements the same
interface using maps, slices, and in-process data structures. Every
time a new method is added to a driven port, the in-memory adapter
must implement it too. If that implementation feels forced — if it
requires importing the SQLite driver, replicating SQL semantics, or
working around a method signature that assumes relational storage — the
port's design needs attention.

### What the in-memory adapter is not

The in-memory adapter is not a mock. It does not record calls or verify
interaction sequences. It is a fully functional, behaviorally correct
implementation of the persistence contract that happens to use different
storage mechanics.

This distinction matters. A mock encodes assumptions about *how* the
core will call the port; a real implementation validates that the port's
contract is *satisfiable*. Mocks can pass while the real adapter fails;
the in-memory adapter surfaces contract violations at compile time.

### The port honesty feedback loop

Adding a new driven port method follows a natural cycle:

1. Define the method signature on the port interface.
2. Implement it in the SQLite adapter.
3. Implement it in the in-memory adapter.

Step 3 is the architectural check. If the in-memory implementation
requires importing `database/sql`, a SQLite-specific type, or any
adapter-level dependency, the method signature is leaking
implementation details. The fix is to redesign the method to operate
in terms of domain entities and standard library types.

This feedback loop keeps the port honest over time. Without it, port
interfaces gradually drift toward the shape of their primary adapter —
and by the time a second adapter is needed, the port is too entangled
to be implemented independently.

### When the test fails

Symptoms that the in-memory adapter test is failing (conceptually, not
literally):

- The in-memory adapter imports packages outside of `domain/`,
  `ports/driven/`, and the standard library.
- Implementing a new port method in the in-memory adapter requires
  understanding SQLite-specific behavior (transaction isolation levels,
  FTS syntax, JSON functions).
- The in-memory adapter's implementation for a method is substantially
  more complex than the domain logic it represents — suggesting the port
  method is doing too much.
- A test using the in-memory adapter behaves differently from the same
  test against SQLite, for reasons unrelated to storage mechanics (e.g.,
  different business-rule outcomes).

Each of these is a signal to revisit the port's method signature, not
to patch the in-memory adapter.

## 8. CLI Discoverability Rule

**Hidden commands and hidden flags are forbidden.** Every functional
command and every functional flag must appear in `--help` output.
Nothing in the CLI surface may be invisible to the user.

### Rationale

`--help` is the authoritative reference for the CLI surface. AI agents
discover available commands and flags by reading help text — a hidden
command is, for all practical purposes, a nonexistent command. Hiding
a command or flag creates a discoverability gap: the feature exists and
can be invoked, but no automated consumer can learn about it through
the CLI's own self-description.

This rule applies to both `urfave/cli/v3`'s `Hidden` field on commands
and flags, and to any other mechanism that suppresses a functional
element from help output.

### Exception process

This prohibition may only be lifted by explicit, written direction
from the project owner that specifically names the command or flag
being hidden and states the reason. A general "hide things that aren't
ready" policy is not sufficient — each hidden element requires
individual approval.
