---
name: np-creating-issues
description: Use when the agent needs to file a new `np` (nitpicking) issue — an incidental discovery while working on something else, a child task or sub-epic when decomposing a parent epic, a deferred follow-up, or any new task or epic the user asks to be created. Triggers on prompts like "create an issue for ...", "file a follow-up", "decompose this epic into ...", "add a child task under FOO-c4npt".
---

# np-creating-issues

## Overview

`np json create` is the agent entry point for new issues. It accepts a JSON object on stdin and returns the created issue as JSON. Identity is on the command line; content is in the JSON body.

## Author identity

Every creation requires `--author <name>`. If no name has been chosen for this session, generate one:

```bash
$ np agent name --seed=$PPID
blue-seal-echo
```

Reuse that name for the rest of the session.

## Creating a task

The minimal create:

```bash
$ np json create --author blue-seal-echo <<'JSONEND'
{
  "title": "Fix retry helper to honour context cancellation",
  "priority": "P1"
}
JSONEND
```

`role` defaults to `task` when omitted. Useful optional fields:

| Field | Notes |
|---|---|
| `description` | Free-text Markdown. Use it for problem context. |
| `acceptance_criteria` | **String**, not array. Markdown text. Make criteria falsifiable. |
| `priority` | `P0` (highest) through `P4`; default `P2`. |
| `parent` | An existing issue ID to attach as a child. |
| `labels` | Array of `key:value` strings, e.g. `["kind:bug", "area:auth"]`. |

Unknown fields are rejected.

## Creating an epic

Epics are containers for grouped work. Set `role` explicitly:

```bash
$ np json create --author blue-seal-echo <<'JSONEND'
{
  "role": "epic",
  "title": "Authentication overhaul",
  "description": "Replace legacy session-based auth with JWT tokens.",
  "priority": "P1"
}
JSONEND
```

`np` enforces a maximum hierarchy depth of three (epic → epic → task). If decomposition wants more depth, that is a sign the hierarchy is doing too much; use `blocked_by` between peers instead.

## CLI flags

| Flag | Effect |
|---|---|
| `--author <name>` | Required. Identity for the mutation. |
| `--with-claim` | Immediately claims the new issue and returns the claim ID. Mutually exclusive with `--deferred`. |
| `--deferred` | Creates the issue in the deferred state so it does not appear as ready work. Mutually exclusive with `--with-claim`. |

## Decomposing an epic

When the agent has claimed an epic and is breaking it into children:

```bash
$ np json create --author blue-seal-echo <<'JSONEND'
{
  "title": "Implement JWT token generation service",
  "priority": "P1",
  "parent": "FOO-c4npt"
}
JSONEND

$ np json create --author blue-seal-echo <<'JSONEND'
{
  "title": "Replace session middleware with JWT validation",
  "priority": "P1",
  "parent": "FOO-c4npt"
}
JSONEND
```

When a sub-area is large enough to need its own breakdown, create a child epic with `"role": "epic"` and a `parent`. Express ordering between siblings using `blocked_by` relationships (see `np-managing-relationships`).

After decomposing, release the epic claim — the epic itself stays open until all children are resolved.

## Deferred follow-ups

When a discovery should be tracked but not picked up immediately:

```bash
$ np json create --deferred --author blue-seal-echo <<'JSONEND'
{
  "title": "Audit retry helpers for context handling",
  "description": "Spotted while working on FOO-a3bxr; not blocking, but should be reviewed.",
  "labels": ["kind:chore", "area:reliability"]
}
JSONEND
```

## Capturing the returned ID

`np json create` returns the new issue's ID as JSON. Capture it before the output scrolls — subsequent steps (adding relationships, claiming, attaching to a parent) need it. With `--with-claim`, also capture the returned claim ID and treat it as a bearer credential (never paste into shared logs or comments).

## What this skill does not cover

- **Updating fields after creation** — escape-hatch via `np-help-discipline` for `np json update --help`.
- **Adding labels to an existing issue** — use `np-labeling`.
- **Adding non-parent relationships** (`blocked_by`, `blocks`, `refs`) — use `np-managing-relationships`. Parent attachment is the only relationship set via `np json create`.
- **Claiming an existing issue** — use `np-finding-work`.

## Common mistakes

- **Passing `acceptance_criteria` as a JSON array.** It is a single Markdown string. An array will be rejected.
- **Setting `parent` via `np rel add`.** Parent is set via the `parent` field on `np json create`. `np rel add` is for `blocked_by`, `blocks`, and `refs`.
- **Omitting `role` when creating an epic.** `role` defaults to `task`. Set it explicitly when an epic is intended.
- **Over-deep hierarchy.** Three levels is the cap. Reach for `blocked_by` to express ordering instead.
