# Package Layout

This document is the package-level companion to
[Architecture](architecture.md).

Use it when you already understand the layering model and need help deciding
where new code belongs in this repository.

## Package Map

```
cmd/
  np/                          CLI entry point - calls wiring.NewCore(), builds
                               the root command, executes it, classifies errors,
                               owns os.Exit().

internal/
  wiring/                      Configurator - constructs Factory and wires
                               dependencies. Does not own CLI execution.

  core/                        Business logic - implements the driving port and
                               orchestrates domain operations through driven
                               ports.

  cmd/                         Driving adapter - CLI commands and command groups.
  ├─ root/                     Root command assembly, signal handling, help
  │                            template
  ├─ create/                   np create
  ├─ show/                     np show
  ├─ list/                     np list
  ├─ claim/                    np claim
  ├─ closecmd/                 np close
  ├─ issuecmd/                 np issue ...
  ├─ epiccmd/                  np epic ...
  ├─ comment/                  np comment ...
  ├─ relcmd/                   np rel ...
  ├─ labelcmd/                 np label ...
  ├─ blocked/                  np blocked
  ├─ ready/                    np ready
  ├─ jsoncmd/                  np json ...
  ├─ formcmd/                  np form ...
  ├─ importcmd/                np import ...
  ├─ admincmd/                 np admin ...
  ├─ agent/                    np agent ...
  ├─ init/                     np init
  └─ version/                  np version

  cmdutil/                     Shared CLI infrastructure - Factory, typed errors,
                               parsing helpers, output formatting, tracker
                               construction.

  domain/                      Business vocabulary and value types. Mostly flat,
                               with `history/` as a focused subpackage.

  ports/
  ├─ driving/                  Core API exposed to the CLI
  └─ driven/                   Contracts the core uses for storage and other
                               external capabilities

  adapters/
  └─ driven/
     ├─ backup/jsonl/          JSONL backup/restore adapter
     └─ storage/
        ├─ memory/             In-memory storage adapter used heavily in tests
        └─ sqlite/             Production SQLite adapter

  iostreams/                   Terminal I/O abstraction and TTY-aware behavior

test/
  blackbox/                    Compiled-binary component tests
```

## What Goes Where

### Types and values

| You are adding... | Package | Why |
|---|---|---|
| A business enum or value type | `internal/domain` | It is domain vocabulary |
| A domain error other layers need to match on | `internal/domain` | Error types are part of the business vocabulary |
| A driving-port input or output DTO | `internal/ports/driving` | It crosses the CLI/core boundary |
| A driven-port query or repository contract type | `internal/ports/driven` | It is part of the core/storage contract |

### Behavior

| You are adding... | Package | Why |
|---|---|---|
| A new use case or business rule | `internal/core` or `internal/domain` | This is business behavior |
| A new repository/storage method | `internal/ports/driven` plus each adapter implementation | The contract lives in the port; behavior lives in adapters |
| A new CLI command | `internal/cmd/<name>` | Commands are driving-adapter code |
| Shared flag/output/error plumbing | `internal/cmdutil` | It is CLI infrastructure, not domain logic |
| Presentation-only rendering code | The command package that uses it, or `cmdutil` if shared | Rendering belongs with the adapter |

### Packages and modules

| You are adding... | Package | Why |
|---|---|---|
| A new driven adapter | `internal/adapters/driven/...` | Adapters are leaf packages |
| A new driving adapter | A new entry point under `cmd/` plus adapter package(s) | Each driving adapter gets its own executable surface |
| A new domain subpackage | Only if the concept is large enough to justify it | Avoid fragmenting the domain package without need |

## Decision Guide

When placement is ambiguous, walk these checks in order:

1. If it depends on a database driver, terminal device, filesystem specifics,
   or another external capability, it is adapter code.
2. If it defines a boundary between the core and another layer, it is a port.
3. If it expresses business vocabulary or invariants with no adapter
   dependency, it belongs in the domain.
4. If it orchestrates business behavior across entities or transactions, it
   belongs in the core.
5. If it only exists because of CLI parsing, formatting, or terminal behavior,
   it belongs in `internal/cmd` or `internal/cmdutil`.

## Notes For Contributors

- Prefer the existing package pattern over inventing a new one for a single
  change.
- Update both SQLite and in-memory adapters when a driven port changes.
- Keep the domain package cohesive; do not split it just to make files shorter.
- Keep `cmdutil` boring. It should help commands talk to the app, not own
  business behavior.
