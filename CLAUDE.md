# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nitpicking (`np`) is a local-only, non-invasive, single-machine, CLI-driven issue tracker designed for AI agent workflows. Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project, it deliberately avoids beads' invasiveness (global hooks, git lifecycle coupling, database servers) and complexity (multi-machine agent collaboration, scope creep).

## Language & Tooling

- **Language:** Go (1.24+)
- **Build output:** `dist/` (created by `make build`)
- **Coverage reports:** `coverage/` (created by `make coverage`)
- **Editor config:** `.editorconfig` — Go files use tabs, indent size 4; most other files use spaces, indent size 2

## Commands

```bash
make build      # Build binary to dist/
make test       # Run unit tests
make coverage   # Run tests with coverage report to coverage/
make lint       # Run linter
```

## Architecture

The project follows **Hexagonal (Ports & Adapters) Architecture** with three layers:

1. **Core Domain** — ticket model, state machine, business rules, validation, history, readiness, deletion logic. No dependencies on CLI or storage.
2. **Ports** — driving port (application API / use-case boundary exposed to adapters) and driven port (persistence interface the core requires).
3. **Adapters** — driving adapter (CLI `np` command) and driven adapter (SQLite storage).

### Development sequencing

Work proceeds inside-out: core domain first, then port interfaces, then adapters. CLI command structure and SQLite schema are explicitly deferred until the domain model and ports are solid.

### Testing strategy

The core domain is unit-tested with in-memory fakes for the persistence port — no SQLite required for domain tests.

## Domain Model (key concepts)

- **Two ticket types ("roles"):** Epic (organizes other tickets; completion derived from children) and Task (leaf node; directly stateful).
- **Priority:** P0–P4 on all tickets. Default P2. Lower number = higher urgency. Drives "claim next ready" ordering (P0 first, ties broken by creation time).
- **Claiming gates all mutations.** To change a ticket's state, you must be its current claimer. The only ungated transition is into `claimed` itself. Notes and relationships can be added without claiming. Claims are bearer-authenticated via random **claim IDs**.
- **Task states:** `open`, `claimed`, `closed`, `deferred`, `waiting`. `closed` is terminal.
- **Epic states:** `active`, `claimed`, `deferred`, `waiting`. Completion is derived (all children closed/complete), not a state — and not a lock (new children can always be added).
- **Readiness:** Tasks are ready when `open`, unblocked, and no ancestor epic is `deferred`/`waiting`. Epics are ready when `active`, have no children (need decomposition), and are unblocked.
- **Facets:** key-value pairs for filtering and agent coordination (e.g., `repo:auth-service`). Keys: 1–64 bytes ASCII printable. Values: 1–256 bytes UTF-8. No whitespace in either.
- **Relationships:** `blocked_by`/`blocks` and `cites`/`cited_by`. No self-referential relationships. Cycles detected by `doctor`, not prevented at write time.
- **Parent constraints:** Only epics can be parents. No self-parenting. No ancestor cycles. Cannot parent to a deleted ticket.
- **Soft deletion:** treated as hard delete by all normal operations; data retained for history reconstruction and `gc`. No undelete. Epic deletion fails if any descendant is currently claimed.
- **Ticket IDs:** `<PREFIX>-<random>` (e.g., `NP-a3bxr`). Prefix: uppercase ASCII letters, 1–10 chars, set at db init. Random: 5 lowercase Crockford Base32 characters. `WITHOUT ROWID` table.
- **History:** every ticket mutation produces a history entry (event-sourcing compatible). Notes do not produce history entries. Revision = history count − 1.
- **Database discovery:** `np` walks up from `cwd` looking for a `.np/` directory. Permission/sandbox errors during the walk are silently ignored.
- **All changes are atomic and auditable** (author + timestamp, NFC-normalized, case-sensitive).

See `SPECIFICATION.md` for the full specification and `PRODUCT_IDEAS.md` for product context and resolved design decisions.
