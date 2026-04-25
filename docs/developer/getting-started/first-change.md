# First Change

This guide is the shortest path from "I just cloned the repo" to "I can make a
safe change without fighting the architecture."

Read this before the deeper architecture documents unless your task is broad or
structural.

## What A Safe Change Looks Like

A safe first change has four properties:

- you know which layer owns the behavior
- you know which test tier proves it
- you are changing one thing at a time
- you can tell whether docs or agent-facing output must move with the code

If you cannot answer those yet, stop and read
[Architecture](../architecture/architecture.md) and
[Package Layout](../architecture/package-layout.md).

## Pick The Right Starting Point

### If you are changing a command

Start in `internal/cmd/...`.

Read:

1. [CLI Implementation Guide](../architecture/cli-implementation.md)
2. [Package Layout](../architecture/package-layout.md)

Then verify:

- parsing stays in `NewCmd`
- business behavior moves into a helper or the core
- `--help` stays accurate
- `np agent prime` still emits valid commands if the CLI surface changed

### If you are changing a business rule

Start in `internal/domain` or `internal/core`.

Read:

1. [Architecture](../architecture/architecture.md)
2. [Package Layout](../architecture/package-layout.md)

Then verify:

- the CLI is not deciding the rule
- SQLite is not deciding the rule
- the in-memory adapter still satisfies the same driven-port contract

### If you are changing storage or import behavior

Start at the driven port and adapter boundary.

Read:

1. [Architecture](../architecture/architecture.md)
2. [Package Layout](../architecture/package-layout.md)
3. [JSONL Import Format](../reference/jsonl-import-format.md) if import is involved

Then verify:

- the port change is justified at the core boundary
- both SQLite and memory adapters are updated
- boundary tests cover the storage-specific behavior

## Minimal Working Loop

From the repo root:

```bash
make build
make test
make lint
```

Add broader coverage only when the change needs it:

```bash
make test-boundary
make test-blackbox
```

Use `make ci` before finishing a broad or risky change.

## Questions To Ask Before Editing

- Is this a CLI concern, a business-rule concern, or a storage concern?
- Which package should own the change?
- Which tests fail if I put the logic in the wrong layer?
- Does this change the public CLI surface, workspace discovery, or agent-prime output?

## Questions To Ask Before Sending A Change

- Did I keep command parsing and rendering in the CLI layer?
- Did I keep business rules in the core or domain?
- Did I update both driven adapters if the driven port changed?
- Did I update docs that describe the changed behavior?

## Read More Only As Needed

- [Onboarding](onboarding.md) for the broader mental model
- [Developer Setup](developer-setup.md) for the full command catalog
- [CLI Implementation Guide](../architecture/cli-implementation.md) for command,
  wiring, and testing patterns
- [Reference docs](../reference/README.md) for specialized formats and security
  rationale
