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
make test-boundary      # Boundary tests (requires external systems; build tag: boundary)
make test-blackbox      # Blackbox component tests (requires full environment; build tag: blackbox)
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

The project follows **Hexagonal (Ports & Adapters) Architecture**. The authoritative reference for architectural layering, dependency rules, package placement, and anti-patterns is [`docs/developer/architecture.md`](docs/developer/architecture.md). When this section and that document conflict, the architecture document governs.

**Key points** (see the architecture doc for full detail):

- Seven conceptual layers: domain entities → driving ports → core → driven ports → driven adapters → driving adapters → configurator.
- Dependencies always point inward; driving adapters importing domain value types is correct, not a violation.
- Development proceeds inside-out: domain first, then ports, then adapters.
- The core is unit-tested with in-memory fakes for the driven ports — no SQLite required for domain tests.
- The in-memory adapter is a real implementation (not a mock) that keeps the driven port abstraction honest.

## Domain Model (key concepts)

- **Two issue types:** Epic (organizes children; acquires a `completed` display badge when all children are closed, but remains open until explicitly closed via `np epic close-completed`) and Task (leaf node; directly stateful). Both epics and tasks may have children; `np epic close-completed --include-tasks` handles parent tasks in the completed-by-children condition the same way.
- **Claiming gates all mutations.** Bearer-authenticated via random claim IDs. Comments and relationships can be added without claiming.
- **Issue IDs:** `<PREFIX>-<random>` (e.g., `PKHP-a3bxr`). Prefix set at db init; random part is 5 lowercase Crockford Base32 characters.
- **Database discovery:** `np` walks up from `cwd` looking for a `.np/` directory.
- **JSONL import:** `np import` reads a JSONL file and bulk-creates issues with relationships, comments, labels, and state transitions. The import pipeline is two-phase: validation (domain layer, `internal/domain/`) runs before any mutations, then the import pass (service layer, `ImportIssues`) creates issues. Import is idempotent via `idempotency_key`.

## Gotchas

- **No golangci-lint.** Linting uses six individual tools invoked separately via `make lint`. All are managed as Go tool dependencies in `go.mod`.
- **Boundary/blackbox component tests use build tags.** They won't run with `make test`; use `make test-boundary` or `make test-blackbox` explicitly.
- **Version injection via ldflags.** `make build` injects the version string into `internal/wiring.version`; pass `VERSION=x.y.z` to override the default `"dev"`.
- **Hidden commands and hidden flags are forbidden.** Every functional command and flag must be visible in `--help` output. `--help` is the authoritative reference for the CLI surface; hiding elements breaks discoverability for AI agents. This prohibition may only be lifted by explicit direction from the project owner that specifically names the command or flag being hidden.
- **ANSI escape sequences are invisible but not zero-width to naive code.** Any TTY layout — tables, aligned columns, padded fields, progress bars — that measures string width to compute padding will break if it counts ANSI SGR bytes as visible characters. Go's `text/tabwriter`, `fmt` width verbs, and `len()`/`utf8.RuneCountInString()` all make this mistake. Use `cmdutil.TableWriter` (which strips `\x1b\[[0-9;]*m` before measuring) for tabular output, and `utf8.RuneCountInString(cmdutil.StripANSI(s))` anywhere else alignment depends on display width. This has caused header-vs-data misalignment in list output twice; the symptom is columns that look correct without color but shift dramatically when ANSI colors are active.
- **Agent prime must stay in sync with the CLI.** When changing the command tree (adding, removing, or renaming commands) or modifying flags, verify that every `np` invocation in `np agent prime` output is still valid — correct command path and correct flags. Issues that change the command tree or flags must include an acceptance criterion requiring this verification. The agent prime source is `internal/cmd/agent/instructions.go`; the rule file `.claude/rules/issue-tracking.md` must be regenerated from `np agent prime` after any change.
