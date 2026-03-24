# Nitpicking — Project Log

> Tracks completed implementation steps.

---

## Phase 1 — Core Domain Model

- [x] **1.1** Domain Error Types — `internal/domain/errors.go`
  - Sentinel errors: ErrNotFound, ErrIllegalTransition, ErrCycleDetected, ErrDeletedTicket, ErrTerminalState
  - Typed errors: ValidationError (structured fields), ClaimConflictError (holder + stale-at), DatabaseError (op + wrapped)
- [x] **1.2** Ticket ID Value Object — `internal/domain/ticket/id.go`
  - ParseID, GenerateID with collision retry, ValidatePrefix, Crockford Base32 validation
- [x] **1.3** Author Value Object — `internal/domain/identity/author.go`
  - NFC normalization, whitespace rejection, alphanumeric requirement, 64-rune max, case-sensitive equality
- [x] **1.4** Priority Value Object — `internal/domain/ticket/priority.go`
  - P0–P4 enum, ParsePriority, default P2, ordering
- [x] **1.5** Ticket Role Enumeration — `internal/domain/ticket/role.go`
  - RoleTask, RoleEpic, ParseRole
- [x] **1.6** Facet Value Object — `internal/domain/ticket/facet.go`
  - Key validation (ASCII printable, 1–64 bytes), value validation (UTF-8, 1–256 bytes), FacetSet (immutable, copy-on-write)
- [x] **1.7** Task State Machine — `internal/domain/ticket/state.go`
  - States: open, claimed, closed, deferred, waiting; TransitionTask with legal/illegal transition validation
- [x] **1.8** Epic State Machine — `internal/domain/ticket/state.go`
  - States: active, claimed, deferred, waiting; TransitionEpic; no closed state
- [x] **1.9** Ticket Entity — `internal/domain/ticket/ticket.go`
  - Immutable value type with all common fields; NewTask/NewEpic constructors; With* mutation methods returning new values
- [x] **1.10** Parent Constraints — `internal/domain/ticket/parent.go`
  - ValidateParent (only epics, no self, no deleted); ValidateNoCycle with ancestor lookup callback
- [x] **1.11** Note Entity — `internal/domain/note/note.go`
  - Immutable note with sequential ID, display ID (note-N), body validation
- [x] **1.12** Relationship Value Object — `internal/domain/ticket/relationship.go`
  - RelationType enum with inverse mapping; NewRelationship with self-relationship rejection
- [x] **1.13** History Entry Entity — `internal/domain/history/entry.go`
  - EventType enum (8 types); FieldChange struct; immutable Entry with defensive copies
- [x] **1.14** Claim Entity — `internal/domain/claim/claim.go`
  - 128-bit hex claim ID; stale threshold (default 2h, max 24h); IsStale/StaleAt; immutable With* methods
- [x] **1.15** Claiming Rules — `internal/domain/claim/rules.go`
  - ValidateClaim (terminal check, existing claim check, stale+steal logic); StealNote
- [x] **1.16** Epic Completion Derivation — `internal/domain/ticket/completion.go`
  - IsEpicComplete: has children AND all closed/complete; empty = incomplete
- [x] **1.17** Readiness Rules — `internal/domain/ticket/readiness.go`
  - IsTaskReady: open + no unresolved blockers + no deferred/waiting ancestors
  - IsEpicReady: active + no children + no unresolved blockers + no deferred/waiting ancestors
- [x] **1.18** Staleness and Stealing Rules — covered by claim.Claim.IsStale and claim.ValidateClaim
- [x] **1.19** Soft Deletion Rules — `internal/domain/ticket/deletion.go`
  - PlanEpicDeletion: recursive descendant check, conflict on claimed descendants
  - ValidateDeletion: rejects already-deleted tickets
- [x] **1.20** Agent Name Generator — `internal/domain/identity/agentname.go`
  - Three-part Docker-style names (adjective-noun-modifier), ~180K combinations
- [x] **1.21** Agent Instructions Generator — `internal/domain/identity/instructions.go`
  - Markdown block covering core workflow, claim IDs, state transitions, help pointers

## Phase 2 — Port Interfaces

- [x] **2.1** Driven Port — Persistence Interface — `internal/domain/port/repository.go`
  - TicketRepository, NoteRepository, ClaimRepository, RelationshipRepository, HistoryRepository, DatabaseRepository
  - UnitOfWork and Transactor abstractions for transaction management
  - PageRequest/PageResult for keyset pagination; TicketFilter, NoteFilter, HistoryFilter
- [x] **2.2** Driving Port — Application Service Interface — `internal/app/service/`
  - Service interface with all §8 commands as methods
  - Full DTO definitions: CreateTicketInput/Output, ClaimInput/Output, UpdateTicketInput, etc.
  - TransitionAction enum (release, close, defer, wait)
  - DoctorFinding, GCInput/Output for diagnostics

## Phase 3 — Application Service Implementation

- [x] **3.1** In-Memory Fake Repository — `internal/fake/repository.go`
  - Full implementation of all port interfaces (Ticket, Note, Claim, Relationship, History, Database)
  - Thread-safe with sync.RWMutex; supports pagination, filtering, search
  - Transactor/UnitOfWork wrappers in `internal/fake/transactor.go`
- [x] **3.2** Initialize Service
- [x] **3.3** Create Ticket Service — with idempotency key, optional claim-at-creation, relationships
- [x] **3.4** Claim Services — ClaimByID with steal support, ClaimNextReady with steal fallback
- [x] **3.5** Update Ticket Service — field updates, facet changes, optional note in same operation
- [x] **3.6** One-Shot Update Service — atomic claim→update→release
- [x] **3.7** State Transition Service — release, close, defer, wait
- [x] **3.8** Extend Stale Threshold Service
- [x] **3.9** Delete Ticket Service — recursive epic deletion with conflict detection
- [x] **3.10** Note Services — add, show, list, search (per-ticket and global)
- [x] **3.11** Relationship Services — add/remove with history entries
- [x] **3.12** Show/List/Search Ticket Services — readiness, completion, revision derivation
- [x] **3.13** History Service
- [x] **3.14** Doctor Service — stale claim detection
- [x] **3.15** GC Service
  - All services implemented in `internal/app/service/impl.go`
  - 20 unit tests covering core workflows in `internal/app/service/service_test.go`

## Phase 4 — SQLite Adapter

- [x] **4.1** Database Discovery — `internal/storage/sqlite/discover.go`
  - Walk-up search from cwd for .np/ directory; permission errors silently skipped
  - InitDatabaseDir creates .np/ directory
- [x] **4.2** SQLite Schema — `internal/storage/sqlite/schema.go`
  - WITHOUT ROWID tables for tickets, facets, claims, relationships
  - AUTOINCREMENT for notes and history entries
  - FTS5 virtual tables for tickets and notes with sync triggers
  - Indexes on parent_id, state, priority+created_at, idempotency_key
- [x] **4.3** Ticket CRUD — `internal/storage/sqlite/store.go`
  - Create, Get (with includeDeleted), Update, List (with filters/pagination), Search (FTS5)
  - GetChildStatuses, GetDescendants (recursive), HasChildren, GetAncestorStatuses
- [x] **4.4** Note CRUD — CreateNote, GetNote, ListNotes, SearchNotes
- [x] **4.5** Claim Lifecycle — CreateClaim, GetByTicket, GetByID, Invalidate, UpdateActivity, UpdateThreshold, ListStale
- [x] **4.6** Relationships, History, Ancestry — full CRUD with JSON serialization for history changes
- [x] **4.7** GC — physical removal of deleted (and optionally closed) data
- [x] **4.8** Transaction Management — WithTransaction/WithReadTransaction wrapping sql.Tx

## Phase 5 — CLI Adapter

- [x] **5.1** Exit Code Mapping — updated `exit_codes.go` to match §9: 0 OK, 1 error, 2 not found, 3 claim conflict, 4 validation, 5 database
- [x] **5.2** JSON Output Infrastructure — `cmdutil.WriteJSON` for structured JSON output
- [x] **5.3** `np init <PREFIX>` — creates .np/ directory and database
- [x] **5.4** `np agent name` — generates Docker-style random agent name
- [x] **5.5** `np agent prime` — prints Markdown workflow instructions
- [x] **5.6** `np create` — creates tickets with all field flags, optional claim, idempotency key
- [x] **5.7** `np claim id <TICKET-ID>` — claims a ticket with optional steal
- [x] **5.8** `np claim ready` — claims highest-priority ready ticket with role/facet filters
- [x] **5.9** `np update <TICKET-ID>` — updates claimed ticket fields
- [x] **5.10** `np edit <TICKET-ID>` — one-shot claim→update→release
- [x] **5.11** `np release/close/defer/wait` — state transitions
- [x] **5.12** `np extend` — extends stale threshold
- [x] **5.13** `np delete` — soft-deletes with --confirm guard
- [x] **5.14** `np relate add/remove` — relationship management
- [x] **5.15** `np show <TICKET-ID>` — full ticket detail view
- [x] **5.16** `np list` — filtered, ordered, paginated ticket listing
- [x] **5.17** `np search <QUERY>` — full-text search across tickets
- [x] **5.18** `np note add/show/list/search` — note management
- [x] **5.19** `np history <TICKET-ID>` — mutation history
- [x] **5.20** `np doctor` — diagnostics (stale claims)
- [x] **5.21** `np gc --confirm` — garbage collection
- [x] **5.22** Register All Commands — all 23 commands wired into root; service lazily initialized with database discovery

## Phase 6 — Integration Testing and Polish

- [x] **6.1** SQLite Integration Tests — `internal/storage/sqlite/store_integration_test.go`
  - Full lifecycle: create→update→show→close
  - Notes on closed tickets (spec compliance)
  - List with pagination (5 tickets, page size 3)
  - Delete and verify not found
  - Extend stale threshold
  - Build tag: `integration`
- [x] **6.2** E2E CLI Tests — `test/e2e/e2e_test.go`
  - Init + create workflow with JSON output verification
  - List JSON output with total count
  - Exit code verification (not found = exit 2)
  - Agent name generation without database
  - Build tag: `e2e`

