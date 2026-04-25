# Architecture

This document is the authoritative reference for Nitpicking's architectural
layering and dependency rules.

Use this file for the conceptual model and hard boundaries.

Use [Package Layout](package-layout.md) for package placement and
the sections below for the "what to avoid" version.

## Overview

Nitpicking uses a ports-and-adapters style architecture tailored to a
local-only, single-machine, CLI-driven application.

This is not presented as a universal reference architecture. It is the shape
that fits this repo's constraints.

When this document conflicts with another project document on architectural
matters, this file governs.

## Mental Model

The codebase has seven conceptual layers. The numbering implies dependency
direction, not call direction.

### 1. Domain entities

Pure business vocabulary such as issues, claims, comments, priorities, roles,
states, and value types.

No I/O, no persistence knowledge, no presentation concerns.

### 2. Driving ports

The API that the driving adapter calls.

In this repo, the CLI programs against the driving port instead of reaching
into the core or storage directly.

### 3. Core

Business logic and use-case orchestration.

This layer implements the driving port and depends on driven ports for
capabilities it does not own.

### 4. Driven ports

Interfaces the core uses for persistence and other external capabilities.

These ports exist because the core benefits from being decoupled from concrete
storage or serialization details.

### 5. Driven adapters

Concrete implementations of driven ports.

In this repo, SQLite is the production storage adapter and the in-memory
adapter is the main test adapter.

### 6. Driving adapters

User-facing layers that translate input into driving-port calls and translate
responses into output.

Today, the CLI is the only driving adapter.

### 7. Configurator

Cross-layer wiring that assembles the object graph.

This is allowed to know about every layer, but it is not where executable
behavior belongs.

## Dependency Rules

Dependencies always point inward.

| Layer | May import |
|---|---|
| Domain entities | Standard library and domain packages only |
| Driving ports | Domain entities |
| Core | Domain, driving ports, driven ports |
| Driven ports | Domain and standard library types |
| Driven adapters | Domain, driven ports, adapter-specific external libraries |
| Driving adapters | Driving ports and domain value types used by the API |
| Configurator | Everything |

### Hard constraints

- The core never imports adapters.
- Driving adapters never import driven ports or driven adapters for normal
  application behavior.
- Driving adapters importing domain value types is correct when those types are
  part of the driving-port API.
- Adapters do not import each other.
- Nothing imports the configurator.

## The Library Test

The driving port, core, and driven ports should be extractable as a standalone
module without dragging in CLI or SQLite-specific types.

You do not need to perform that extraction. Use it as a thought experiment:

- if inward-facing code needs CLI flags or SQLite handles, the boundary is
  wrong
- if the import graph would prevent extraction, the layering is wrong

## The In-Memory Adapter Test

The in-memory adapter is a design check on the driven-port abstraction.

If a driven-port method cannot be implemented cleanly in memory without
SQLite-specific assumptions, the port has probably leaked implementation
details.

That is why the in-memory adapter matters. It is not just a convenient test
double; it keeps the contract honest.

## Visual Summary

```
main() -> configurator -> driving adapter -> driving port -> core -> driven port -> driven adapter
                              |
                              `-> domain vocabulary flows inward across those boundaries
```

The important rule is simple: dependencies point inward toward the domain and
core.

## CLI Discoverability Rule

Hidden functional commands and hidden functional flags are forbidden.

`--help` is the authoritative description of the CLI surface for both humans
and AI agents. If a feature exists, it must be visible there unless the project
owner explicitly approves a named exception.

## Common Mistakes To Avoid

### Flattening domain enums to strings at the driving-port boundary

Domain enums are business vocabulary, not implementation details. Converting
`domain.Role`, `domain.Priority`, or `domain.State` to plain strings at the
CLI/core boundary throws away type safety without improving layering.

Keep domain vocabulary typed across inward-facing boundaries.

### Driving adapters importing driven ports or driven adapters

CLI commands should talk to the driving port, not directly to SQLite or
repository interfaces.

When a command reaches around the core:

- business logic gets duplicated or skipped
- tests become more adapter-specific
- future driving adapters get harder to add

### Core importing CLI-specific types

The core must not depend on command-package types such as output modes, flag
values, or rendering choices.

If the core needs a concept, define it in an inner layer. The CLI should
translate from command-line concerns into core-facing DTOs.

### Business logic in driving adapters

If a command is deciding whether an issue may transition state, whether an item
is ready, or how child state affects a parent, that is the wrong layer.

The CLI should parse input, call the service, and render output.

### Business logic in driven adapters

SQLite and in-memory adapters should translate storage operations, not decide
business behavior.

If adapter code has to understand why the core wants something, rather than
what it wants, the contract is probably in the wrong place.

### Configurator owning executable-specific behavior

`internal/wiring` assembles the object graph. It should not:

- parse CLI arguments
- own signal handling policy
- format terminal errors
- call `os.Exit`

Those belong to the executable entry point and the CLI adapter.

## Related Documents

- [Package Layout](package-layout.md)
- [CLI Implementation Guide](cli-implementation.md)
