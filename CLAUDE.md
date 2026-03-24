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

1. **Core Domain** — issue model, state machine, business rules, validation, history, readiness, deletion logic. No dependencies on CLI or storage.
2. **Ports** — driving port (application API / use-case boundary exposed to adapters) and driven port (persistence interface the core requires).
3. **Adapters** — driving adapter (CLI `np` command) and driven adapter (SQLite storage).

### Development sequencing

Work proceeds inside-out: core domain first, then port interfaces, then adapters. CLI command structure and SQLite schema are explicitly deferred until the domain model and ports are solid.

### Testing strategy

The core domain is unit-tested with in-memory fakes for the persistence port — no SQLite required for domain tests.

## Domain Model (key concepts)

- **Two issue types:** Epic (organizes children; completion derived) and Task (leaf node; directly stateful). See state machines, claiming, readiness, and all other domain details in `SPECIFICATION.md`.
- **Claiming gates all mutations.** Bearer-authenticated via random claim IDs. Comments and relationships can be added without claiming.
- **Issue IDs:** `<PREFIX>-<random>` (e.g., `NP-a3bxr`). Prefix set at db init; random part is 5 lowercase Crockford Base32 characters.
- **Database discovery:** `np` walks up from `cwd` looking for a `.np/` directory.

See `SPECIFICATION.md` for the full specification and `PRODUCT_VISION.md` for product context and resolved design decisions.

## Gotchas

- **No golangci-lint.** Linting uses six individual tools invoked separately via `make lint`. All are managed as Go tool dependencies in `go.mod`.
- **Integration/E2E tests use build tags.** They won't run with `make test`; use `make test-integration` or `make test-e2e` explicitly.
- **Version injection via ldflags.** `make build` injects the version string into `internal/app.version`; pass `VERSION=x.y.z` to override the default `"dev"`.

# np — Issue Tracker

np is the **exclusive** tool for task management in this project. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

np is local-only — no network, no remote sync, no background daemons. It stores issues in an embedded SQLite database under the `.np/` directory.

## Choosing an Author Name

Every mutation requires an `--author` flag identifying who is acting. Pick a stable name and reuse it for your entire session. Generate one with `np agent name` if you need a fresh identifier.

## Core Workflow

### 1. Find work

```bash
np claim ready --author <your-name>   # claim the highest-priority ready issue
np ready                              # browse all ready issues without claiming
np blocked                            # list issues blocked by unresolved dependencies
np status                             # dashboard with counts by state
np list --ready --dimension kind:fix  # filter ready issues by dimension
np list --include-closed              # include closed issues (hidden by default)
np list --state closed                # show only closed issues
np search "login timeout"             # full-text search across titles and descriptions
```

`np list` hides closed issues by default since they are typically resolved. Use `--include-closed` to show them, or `--state closed` to list only closed issues.

An issue is **ready** when it is `open` with no children (for epics, needing decomposition), has no unresolved `blocked_by` relationships, and no ancestor epic is `deferred` .

### 2. Claim before mutating

Claiming is mandatory before updating fields or transitioning state. Claiming returns a **claim ID** — a token you must pass to all subsequent operations on that issue.

```bash
np claim id <ISSUE-ID> --author <your-name>
# → returns claim ID, e.g. a1b2c3d4e5f6...
```

Persist the claim ID for the duration of your work on that issue. If you lose it, you cannot continue — another agent (or you) must wait for the claim to go stale and then steal it.

### 3. Update fields

```bash
np update <ISSUE-ID> --claim <CLAIM-ID> --title "Revised title"
np update <ISSUE-ID> --claim <CLAIM-ID> --description "More detail"
np update <ISSUE-ID> --claim <CLAIM-ID> --priority 1
np update <ISSUE-ID> --claim <CLAIM-ID> --dimension kind:fix
```

For a quick one-shot edit that does not require holding a claim, use `np edit`:

```bash
np edit <ISSUE-ID> --author <your-name> --title "Quick fix"
```

### 4. Document your work with comments

Before transitioning state, add a comment capturing your reasoning, trade-offs, findings, or anything a future reader would find useful.

```bash
np comment add --issue <ISSUE-ID> --body "Approach taken: ..." --author <your-name>
```

### 5. Transition state when done

Use `np done` (alias: `close`) for the common workflow of closing an issue with a reason. It adds a comment and closes in one step:

```bash
np done <ISSUE-ID> --claim <CLAIM-ID> --author <your-name> --reason "Completed: all tests pass."
```

For other transitions, use the explicit commands:

```bash
np release <ISSUE-ID> --claim <CLAIM-ID>   # return to open/active without completing
np state defer <ISSUE-ID> --claim <CLAIM-ID>   # shelve for later
np state close <ISSUE-ID> --claim <CLAIM-ID>   # close without a reason (prefer 'done' instead)
```

**Always transition state when you are done.** Abandoned claims block other agents until the stale threshold expires.

## Issue Types

| Role | Purpose | Closed directly? |
|------|---------|-----------------|
| **Task** | Leaf-node work item | Yes — `np close` is terminal |
| **Epic** | Organizes children; completion is derived | No — an epic is complete when all its children are closed or complete |

Create issues with:

```bash
np create --role task --title "Implement retry logic" --author <your-name>
np create --role epic --title "Authentication overhaul" --author <your-name>
np create --role task --title "Write tests" --author <your-name> --parent <EPIC-ID>
```

Use `--claim` on create to atomically create and claim in one step.

Use `--from-json` to provide issue fields as JSON (compatible with `show --json` output):

```bash
np create --from-json '{"role":"task","title":"Fix bug","priority":"P0"}' --author <your-name>
np show <ISSUE-ID> --json | np create --from-json - --author <your-name>   # clone an issue
```

Precedence: explicit flags > JSON values > env vars. Dimensions with different keys from all sources are merged; for the same key, the higher-precedence source wins.

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
np relate add <ISSUE-ID> blocked_by <BLOCKER-ID> --author <your-name>
np relate add <ISSUE-ID> cites <REFERENCE-ID> --author <your-name>
np relate remove <ISSUE-ID> blocked_by <BLOCKER-ID> --author <your-name>
```

- `blocked_by` / `blocks` — the issue cannot progress until the blocker is closed.
- `cites` / `cited_by` — informational reference; does not block.

The `rel` command (alias: `r`) provides a structured namespace for relationships:

```bash
np rel blocks add --source <ID> --target <BLOCKER-ID> --author <name>    # add blocker
np rel blocks remove --source <ID> --target <BLOCKER-ID> --author <name> # remove blocker
np rel blocks list --issue <ID>                                           # list blocking rels
np rel cites add --source <ID> --target <REF-ID> --author <name>         # add citation
np rel cites list --issue <ID>                                            # list citations
np rel parent detach --issue <ID> --claim <CLAIM-ID>                     # remove parent
np rel parent children --issue <ID>                                       # list children
np rel parent tree --issue <ID>                                           # show descendant tree
np rel list --issue <ID>                                                  # all relationships
np rel cycles                                                             # detect cycles
```

## Comments

Comments do **not** require claiming and can be added to closed issues.

```bash
np comment add <ISSUE-ID> --body "Found the root cause in auth.go:142" --author <your-name>
np comment list <ISSUE-ID>
np comment search "root cause"
```

## Stale Claims and Stealing

Claims expire after their stale threshold (default 2 hours). If no ready issues exist, steal a stale one:

```bash
np claim ready --steal-if-needed --author <your-name>
np claim id <ISSUE-ID> --author <your-name> --steal
```

Extend your own claim's threshold if you need more time:

```bash
np extend <ISSUE-ID> --claim <CLAIM-ID> --threshold 4h
```

## Epic Subcommand Group

The `epic` command provides epic-specific operations:

```bash
np epic status                         # completion breakdown for all open epics
np epic status --issue <EPIC-ID>       # status for a specific epic
np epic status --eligible-only         # show only epics ready to close
np epic close-eligible --author <name> # batch-close all fully-resolved epics
np epic close-eligible --dry-run --author <name>  # preview without closing
np epic children <EPIC-ID>             # list all children of an epic
```

## Dimension Subcommand Group

The `dimension` (alias: `dim`) command manages key-value metadata on issues:

```bash
np dimension add --issue <ID> --claim <CLAIM-ID> --key kind --value bug     # set dimension
np dimension remove --issue <ID> --claim <CLAIM-ID> --key kind              # remove dimension
np dimension list --issue <ID>                                               # list for issue
np dimension list-all                                                        # all unique dims
np dimension propagate --issue <ID> --author <name> --key kind              # propagate to descendants
```

## Issue Subcommand Group

The `issue` (alias: `i`) command groups issue management operations under a single namespace:

```bash
np issue list                                          # list issues (same as top-level 'list')
np issue query "search text"                           # search issues (same as top-level 'search')
np issue update <ID> --claim <CLAIM-ID> --title "New"  # update a claimed issue
np issue close <ID> --claim <CLAIM-ID> --author <name> --reason "Done."  # close with reason
np issue reopen <ID> --author <name>                   # reopen a deferred issue (alias: undefer)
np issue defer <ID> --claim <CLAIM-ID>                 # defer a claimed issue
np issue delete <ID> --claim <CLAIM-ID> --confirm      # delete a claimed issue
np issue note <ID> --author <name> --body "Note text"  # add a comment (alias: comment)
np issue orphans                                       # list issues with no parent epic
```

## Admin Commands

The `admin` command groups maintenance operations:

```bash
np admin doctor                        # detect cycles, deadlocks, stale claims
np admin gc --confirm                  # garbage-collect deleted issues
np admin gc --confirm --include-closed # also remove closed issues
np admin reset --confirm               # delete .np/ database (destructive)
np admin upgrade                       # check for schema upgrades
```

## Diagnostics

```bash
np doctor       # detect cycles, deadlocks, stale claims, epics needing decomposition
np show <ID>    # full issue detail including readiness, relationships, completion
np history <ID> # audit trail of all changes
np graph        # generate Graphviz DOT of all issues and relationships
np graph -o issues.dot   # write to file instead of stdout
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
- **Document your work.** Add a comment before transitioning state — capture reasoning, trade-offs, and findings.
- **Always transition state when done.** Do not abandon claims — release, close, or defer.
- **Closed issues can be reopened.** Claim a closed issue and release it to reopen, or use `np issue reopen`.
- **Epics are never closed directly.** An epic is complete when all its children are resolved.
- **Use `np` exclusively.** Do not track work outside of `np`.
