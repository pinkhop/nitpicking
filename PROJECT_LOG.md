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

