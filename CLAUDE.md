# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Nitpicking (`np`) is a local-only, non-invasive, single-machine, CLI-driven issue tracker designed for AI agent workflows. Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project, it deliberately avoids beads' invasiveness (global hooks, git lifecycle coupling, database servers) and complexity (multi-machine agent collaboration, scope creep).

## Language & Tooling

- **Module:** `github.com/pinkhop/nitpicking`
- **Language:** Go 1.26+
- **CLI framework:** `github.com/urfave/cli/v3`
- **Build output:** `dist/` (created by `make build`); static binary, `CGO_ENABLED=0`
- **Coverage reports:** `coverage/` (created by `make coverage`)
- **Editor config:** `.editorconfig` — Go files use tabs, indent size 4; most other files use spaces, indent size 2

## Commands

```bash
# Build
make build              # Build static binary to dist/np (VERSION=x.y.z to set version)
make clean              # Remove dist/, coverage/, and test cache

# Test
make test               # Run unit tests (alias for test-units)
make test-integration   # Integration tests (requires external systems; build tag: integration)
make test-e2e           # E2E tests (requires full environment; build tag: e2e)
make coverage           # Unit tests with HTML coverage report to coverage/

# Format
make fmt                # Run all formatters (gofumpt + goimports)

# Lint (make lint runs all)
make lint               # go vet, gofumpt, goimports, ineffassign, errcheck, staticcheck

# Security
make sec                # gosec (static security scan) + govulncheck (CVE check)

# CI
make ci                 # Full pipeline: build → lint → sec → test-units
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

- **Two ticket types:** Epic (organizes children; completion derived) and Task (leaf node; directly stateful). See state machines, claiming, readiness, and all other domain details in `SPECIFICATION.md`.
- **Claiming gates all mutations.** Bearer-authenticated via random claim IDs. Notes and relationships can be added without claiming.
- **Ticket IDs:** `<PREFIX>-<random>` (e.g., `NP-a3bxr`). Prefix set at db init; random part is 5 lowercase Crockford Base32 characters.
- **Database discovery:** `np` walks up from `cwd` looking for a `.np/` directory.

See `SPECIFICATION.md` for the full specification and `PRODUCT_VISION.md` for product context and resolved design decisions.

## Gotchas

- **No golangci-lint.** Linting uses six individual tools invoked separately via `make lint`. All are managed as Go tool dependencies in `go.mod`.
- **Integration/E2E tests use build tags.** They won't run with `make test`; use `make test-integration` or `make test-e2e` explicitly.
- **Version injection via ldflags.** `make build` injects the version string into `internal/app.version`; pass `VERSION=x.y.z` to override the default `"dev"`.

# np — Issue Tracker

np is the **exclusive** tool for task management in this project. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

np is local-only — no network, no remote sync, no background daemons. It stores tickets in an embedded SQLite database under the `.np/` directory.

## Choosing an Author Name

Every mutation requires an `--author` flag identifying who is acting. Pick a stable name and reuse it for your entire session. Generate one with `np agent-name` if you need a fresh identifier.

## Core Workflow

### 1. Find work

```bash
np next --author <your-name>          # claim the highest-priority ready ticket
np list --ready                       # browse all ready tickets without claiming
np list --ready --facet kind:fix      # filter ready tickets by facet
np search "login timeout"             # full-text search across titles and descriptions
```

A ticket is **ready** when it is `open` (task) or `active` with no children (epic needing decomposition), has no unresolved `blocked_by` relationships, and no ancestor epic is `deferred` or `waiting`.

### 2. Claim before mutating

Claiming is mandatory before updating fields or transitioning state. Claiming returns a **claim ID** — a token you must pass to all subsequent operations on that ticket.

```bash
np claim <TICKET-ID> --author <your-name>
# → returns claim ID, e.g. a1b2c3d4e5f6...
```

Persist the claim ID for the duration of your work on that ticket. If you lose it, you cannot continue — another agent (or you) must wait for the claim to go stale and then steal it.

### 3. Update fields

```bash
np update <TICKET-ID> --claim-id <CLAIM-ID> --title "Revised title"
np update <TICKET-ID> --claim-id <CLAIM-ID> --description "More detail"
np update <TICKET-ID> --claim-id <CLAIM-ID> --priority 1
np update <TICKET-ID> --claim-id <CLAIM-ID> --facet-set kind:fix
```

For a quick one-shot edit that does not require holding a claim, use `np edit`:

```bash
np edit <TICKET-ID> --author <your-name> --title "Quick fix"
```

### 4. Transition state when done

Every transition requires the claim ID and ends the claim.

```bash
np close   <TICKET-ID> --claim-id <CLAIM-ID>   # complete the task (terminal — cannot reopen)
np release <TICKET-ID> --claim-id <CLAIM-ID>   # return to open/active without completing
np defer   <TICKET-ID> --claim-id <CLAIM-ID>   # shelve for later
np wait    <TICKET-ID> --claim-id <CLAIM-ID>   # blocked on an external dependency
```

**Always transition state when you are done.** Abandoned claims block other agents until the stale threshold expires.

## Ticket Types

| Role | Purpose | Closed directly? |
|------|---------|-----------------|
| **Task** | Leaf-node work item | Yes — `np close` is terminal |
| **Epic** | Organizes children; completion is derived | No — an epic is complete when all its children are closed or complete |

Create tickets with:

```bash
np create --role task --title "Implement retry logic" --author <your-name>
np create --role epic --title "Authentication overhaul" --author <your-name>
np create --role task --title "Write tests" --author <your-name> --parent <EPIC-ID>
```

Use `--claim` on create to atomically create and claim in one step.

## Priorities

| Level | Meaning |
|-------|---------|
| P0 | Critical — security, data loss, broken builds |
| P1 | High — major features, important bugs |
| P2 | Medium (default) |
| P3 | Low — polish, optimization |
| P4 | Backlog — future ideas |

## Relationships

Relationships do **not** require claiming.

```bash
np relate add <TICKET-ID> blocked_by <BLOCKER-ID> --author <your-name>
np relate add <TICKET-ID> cites <REFERENCE-ID> --author <your-name>
np relate remove <TICKET-ID> blocked_by <BLOCKER-ID> --author <your-name>
```

- `blocked_by` / `blocks` — the ticket cannot progress until the blocker is closed.
- `cites` / `cited_by` — informational reference; does not block.

## Notes

Notes do **not** require claiming and can be added to closed tickets.

```bash
np note add <TICKET-ID> --body "Found the root cause in auth.go:142" --author <your-name>
np note list <TICKET-ID>
np note search "root cause"
```

## Stale Claims and Stealing

Claims expire after their stale threshold (default 2 hours). If no ready tickets exist, steal a stale one:

```bash
np next --steal-if-needed --author <your-name>
np claim <TICKET-ID> --author <your-name> --steal
```

Extend your own claim's threshold if you need more time:

```bash
np extend <TICKET-ID> --claim-id <CLAIM-ID> --threshold 4h
```

## Diagnostics

```bash
np doctor       # detect cycles, deadlocks, stale claims, epics needing decomposition
np show <ID>    # full ticket detail including readiness, relationships, completion
np history <ID> # audit trail of all changes
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Not found |
| 3 | Claim conflict (already claimed and not stale, or wrong claim ID) |
| 4 | Validation error |
| 5 | Database error |

Use exit codes to branch your workflow — e.g., exit code 3 means you should wait or steal.

## JSON Output

Append `--json` to any command for structured, machine-readable output. JSON is the primary agent interface; prefer it over human-readable output when parsing results programmatically.

## Key Rules

- **Claim before mutating.** Field updates and state transitions are gated by claiming.
- **Always transition state when done.** Do not abandon claims — release, close, defer, or wait.
- **Close is terminal.** Closed tasks cannot be reopened, reclaimed, or modified (notes can still be added).
- **Epics are never closed directly.** An epic is complete when all its children are resolved.
- **Use `np` exclusively.** Do not track work outside of `np`.
