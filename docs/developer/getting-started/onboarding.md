# Onboarding

This is the best starting point if you are new to the Nitpicking codebase.

The goal is simple: get oriented fast, make a safe change, and avoid learning
the repo in the wrong order.

## What This Repo Is

Nitpicking (`np`) is a local-only, CLI-driven issue tracker for AI-agent
workflows. It is intentionally small-scope:

- one machine
- one CLI
- one local workspace database under `.np/`
- no daemon, server, or multi-user coordination layer

That matters because many design choices in this repo only make sense under
those constraints.

## First Reading Order

Read these docs in this order:

1. [Developer Setup](developer-setup.md)
2. [First Change](first-change.md)
3. [Architecture](../architecture/architecture.md)
4. [Package Layout](../architecture/package-layout.md)

Read these only when the task needs them:

- [CLI Implementation Guide](../architecture/cli-implementation.md) if you are
  changing commands, wiring, help output, or error classification
- [JSONL Import Format](../reference/jsonl-import-format.md) if you are
  changing import or backup/restore behavior
- [Claim ID Security Model](../reference/claim-security.md) if you are
  changing claim generation, validation, redaction, or storage

## First Commands To Run

From the repo root:

```bash
make build
make test
make lint
```

If you are touching SQLite behavior or end-to-end CLI behavior, also run the
narrower tier that matches your change:

```bash
make test-boundary
make test-blackbox
```

Use `make ci` before finishing a broad change.

## Mental Model

The repo has a clean center of gravity:

- `internal/domain` holds business vocabulary
- `internal/ports/driving` and `internal/ports/driven` define boundaries
- `internal/core` holds business behavior
- `internal/adapters/driven/...` holds concrete storage and backup adapters
- `internal/cmd/...` is the CLI driving adapter
- `internal/wiring` assembles the app

If you are unsure where code belongs, the shortest answer is:

- CLI parsing and rendering belong in `internal/cmd`
- business rules belong in `internal/core` or `internal/domain`
- persistence details belong in driven adapters

For the authoritative version of that rule, use
[Architecture](../architecture/architecture.md) and
[Package Layout](../architecture/package-layout.md).

## Common Tasks

### Add a new CLI command

Read:

1. [First Change](first-change.md)
2. [CLI Implementation Guide](../architecture/cli-implementation.md)

Then:

1. Add a package under `internal/cmd/`
2. Keep parsing in `NewCmd`
3. Delegate behavior to a testable helper
4. Register the command in the root tree
5. Verify `--help` output
6. If the command tree or flags changed, verify `np agent prime`

### Change a business rule

Read:

1. [Architecture](../architecture/architecture.md)
2. [Package Layout](../architecture/package-layout.md)

Then start in `internal/domain` or `internal/core`, not in the CLI or SQLite
adapter.

### Change persistence or import behavior

Read:

1. [Architecture](../architecture/architecture.md)
2. [Package Layout](../architecture/package-layout.md)
3. [JSONL Import Format](../reference/jsonl-import-format.md) if import is
   involved

If you add or change a driven-port method, update both concrete adapters:
SQLite and in-memory.

## First Safe Change Checklist

Before opening a change, make sure you can answer these questions:

- Which layer owns this behavior?
- Am I changing a CLI concern, a business rule, or a storage concern?
- Which test tier proves the change?
- Does this affect `--help`, `np agent prime`, or workspace discovery docs?

If you cannot answer those yet, stop and read
[Architecture](../architecture/architecture.md) and
[First Change](first-change.md) before editing code.
