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
