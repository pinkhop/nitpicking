---
name: np-help-discipline
description: Use when about to invoke an `np` (nitpicking) command, subcommand, or flag that has not been verified in the current session, or when unsure whether a flag or subcommand exists. Also use as the escape hatch for any `np` activity not covered by another skill (e.g., `np issue reopen`, `np issue undefer`, `np epic close-completed`, `np admin backup`, `np agent name`, `np json update`).
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np admin backup:*) Bash(np admin doctor:*) Bash(np agent:*) Bash(np claim:*) Bash(np close:*) Bash(np epic:*) Bash(np issue:*) Bash(np json:*) Bash(np label:*) Bash(np list:*) Bash(np ready:*) Bash(np rel:*) Bash(np show:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-help-discipline

## Overview

`np` exposes a large command tree, and only the most common activities have dedicated skills. The single rule that keeps the rest safe is: **never fabricate an `np` command, subcommand, or flag — always run `--help` first.**

## The rule

Before invoking any `np` command, subcommand, or flag whose exact shape is not already known, run `--help` on the most specific level available. Read the output. Then invoke the real command.

```bash
$ np --help                     # top-level commands
$ np rel --help                 # rel subcommands
$ np rel add --help             # rel add usage and flags
$ np json create --help         # required and optional fields, flags
$ np claim --help               # claim modes and flag shapes
```

`--help` is the authoritative reference for the CLI surface. Hidden commands and hidden flags are forbidden by project policy, so anything real shows up there.

## When this skill applies

- The other `np-*` skills cover finding work, finishing work, reading issues, creating issues, commenting, labeling, and managing relationships. For anything outside those activities — reopening a closed issue, undeferring a deferred one, batch-closing completed epics, taking a backup, generating an agent name, updating an issue's fields after creation — start here.
- If a covered skill mentions a flag or subcommand without spelling out its full syntax, run `--help` on that subcommand before guessing.

## Examples of activities that route through `--help`

| Activity | Where to start |
|---|---|
| Reopen a closed issue | `np issue reopen --help` |
| Undefer a deferred issue | `np issue undefer --help` |
| Batch-close completed epics | `np epic close-completed --help` |
| Take a backup before destructive work | `np admin backup --help` |
| Generate a stable agent name | `np agent name --help` |
| Update fields on a claimed issue | `np json update --help` |
| List children of an epic | `np epic children --help` |

## What this skill does not do

It does not enumerate every `np` command (that is what `--help` is for). It does not replace the dedicated activity skills when one of them applies — prefer the targeted skill, then fall back here.

## Common mistakes

- **Fabricating a flag from memory.** `np` evolves; flags rename. Verify every time the command is unfamiliar.
- **Skipping `--help` because the command "looks obvious."** Subcommands often have non-obvious required flags (e.g., `--author`, `--claim`).
- **Reading the project rules file as a substitute for `--help`.** The rules file documents conventions; `--help` documents the current binary.
