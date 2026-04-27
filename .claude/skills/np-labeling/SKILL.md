---
name: np-labeling
description: Use when the agent needs to add or remove labels on an existing `np` (nitpicking) issue (often across multiple issues at once), or inspect the workspace's label vocabulary for drift or to ground a new label key. Triggers on prompts like "add foo:bar to all issues about X", "remove the kind:bug label from FOO-a3bxr", "what labels are in use", "tag these issues with sprint:2026-w17". Does not cover label-based filtering of the ready queue (use `np-finding-work`) or label-based filtering during issue lookup (use `np-reading-issues`).
---

# np-labeling

## Overview

Labels are `key:value` metadata attached to issues. They make backlog-scanning faster, enable filtering and routing, and can capture lightweight workflow markers. This skill covers modifying labels on existing issues and inspecting the workspace vocabulary.

## Author identity and claim

Adding or removing a label is a mutation. It requires the active claim ID for the issue, and `--author` is implicit through the claim:

```bash
$ np label add kind:bug --claim <CID>
$ np label remove kind:bug --claim <CID>
```

If the agent does not already hold a claim, claim the issue first (see `np-finding-work`), then label, then transition (see `np-finishing-work`). For a labeling-only task, releasing the claim immediately after with `np issue release --claim <CID>` keeps the issue's state unchanged.

The claim ID is a bearer credential. Never paste it into a comment, commit message, or shared log.

`np label list-all` is read-only and needs no claim or author flag.

## Adding a label

```bash
$ np label add kind:bug --claim a4dace30e46eb1ec14019c79a59c6b27
$ np label add area:auth --claim a4dace30e46eb1ec14019c79a59c6b27
```

A label is a single `key:value` argument. Add multiple labels with multiple invocations.

## Removing a label

`np label remove` takes the **key**, not the full `key:value`:

```bash
$ np label remove kind --claim a4dace30e46eb1ec14019c79a59c6b27
```

That removes whatever value was stored under `kind` on this issue. If the exact remove syntax is unfamiliar, run `np label remove --help` to confirm.

## Applying a label across multiple issues

The user often asks for labeling across a set ("add `task-group:auth-overhaul` to all issues about auth"). Find the issues first (`np-reading-issues`), then for each one: claim, label, release.

```bash
# 1. Find the targets (np-reading-issues)
$ np list --label area:auth --state open --json

# 2. For each target ID:
$ np claim FOO-a3bxr --author blue-seal-echo
# capture the returned claim ID
$ np label add task-group:auth-overhaul --claim <CID>
$ np issue release --claim <CID>
```

Be deliberate about scope. Confirm with the user before sweeping a label across many issues.

## Inspecting the workspace vocabulary

Before introducing a new key, see what is already in use:

```bash
$ np label list-all
KEY       POPULAR VALUES
area      auth (12), cli (8), storage (5)
kind      bug (19), feature (11), docs (8)
risk      low (10), med (4), high (2)
```

This is also the best drift detector. If both `kind:bug` and `kind:Bug` show up, the convention has split.

## Naming conventions

**Defer to project rules and persistent memory** for label naming standards (key vocabulary, value formatting). The user often documents these in `CLAUDE.md`, a workspace conventions issue, or saved memory.

If no guidance is present, default to **lowercase, hyphenated** keys and values:

- `kind:bug`, `kind:feature`, `kind:enhancement`
- `area:auth`, `area:billing-api`
- `task-group:auth-overhaul`, `sprint:2026-w17`

A key must be 1–64 ASCII printable bytes; the first character must be an ASCII letter or underscore. Values are also ASCII printable, no whitespace.

## What this skill does not cover

- **Filtering the ready queue by label** (`np ready --label …`, `np claim ready --label …`) — use `np-finding-work`.
- **Filtering issue lookups by label** (`np list --label …`, `np issue search --label …`) — use `np-reading-issues`.
- **Attaching labels at issue-creation time** — use `np-creating-issues` (the `labels` field on `np json create`).

## Common mistakes

- **Passing `key:value` to `np label remove`.** Remove takes the key only.
- **Forgetting `--claim` on add or remove.** Both require an active claim on the issue.
- **Inventing a new key when one exists.** Run `np label list-all` first; reuse existing keys to keep filtering reliable.
- **Casing drift.** `kind:Bug` and `kind:bug` are different labels. Stick to one convention per workspace.
