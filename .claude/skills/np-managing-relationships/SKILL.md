---
name: np-managing-relationships
description: Use when the agent needs to express, remove, or list a dependency or informational link between two `np` (nitpicking) issues — adding `blocked_by` because one issue cannot proceed until another finishes, adding `refs` for a "see also" pointer, removing a relationship that no longer applies, or listing the workspace's relationships at a glance. Triggers on prompts like "mark FOO-a3bxr as blocked by FOO-c4npt", "FOO-12345 references FOO-67890", "remove the blocker", "show me the blocking chains".
---

# np-managing-relationships

## Overview

`np` models three relationship kinds between issues:

| Kind | Direction | Affects readiness? |
|---|---|---|
| `blocked_by` / `blocks` | Directional dependency (mirror of each other) | Yes — a blocked issue is not ready |
| `refs` | Informational link, "see also" | No |
| parent–child | Structural hierarchy | Yes (children of a deferred or blocked ancestor are not ready) |

**Parent–child is set via the `parent` field on `np json create`, not via `np rel add`.** This skill covers the other two kinds.

## Author identity

Every relationship mutation requires `--author <name>`. If no name has been chosen for this session, generate one:

```bash
$ np agent name --seed=$PPID
blue-seal-echo
```

Reuse that name for the rest of the session. Relationship mutations do **not** require a claim — `blocked_by`, `blocks`, and `refs` can be added or removed without claiming either issue.

## Adding a relationship

```bash
$ np rel add FOO-a3bxr blocked_by FOO-c4npt --author blue-seal-echo
$ np rel add FOO-a3bxr blocks FOO-q1w2e --author blue-seal-echo
$ np rel add FOO-a3bxr refs FOO-67890 --author blue-seal-echo
```

The positional shape is `<issue-A> <rel> <issue-B>`. Pick the relationship name that reads naturally:

- `A blocked_by B` — A cannot proceed until B is resolved.
- `A blocks B` — same edge from the other side; pick whichever reads better in context.
- `A refs B` — A points at B for context, with no readiness implication.

If the exact subcommand syntax is unfamiliar, run `np rel add --help` to confirm.

## Removing a relationship

`np rel remove` mirrors `np rel add` exactly — same positional order, same accepted relationship names:

```bash
$ np rel remove FOO-a3bxr blocked_by FOO-c4npt --author blue-seal-echo
$ np rel remove FOO-a3bxr refs FOO-67890 --author blue-seal-echo
```

## Listing relationships

`np rel list` shows the workspace's active relationships in three sections — parent–child hierarchy, blocking chains, and reference clusters:

```bash
$ np rel list                       # all three sections
$ np rel list --rel=blocking        # blocking chains only
$ np rel list --rel=refs            # reference clusters only
$ np rel list --rel=parent-child    # parent-child tree only
```

`--rel` also accepts the relationship-name aliases: `blocked_by` and `blocks` map to `blocking`; `parent_of` and `child_of` map to `parent-child`.

For relationships on a single issue, prefer `np show <ID>` (covered by `np-reading-issues`) — it lists the relationships in the context of the issue itself.

## Choosing between `blocked_by` and `refs`

| Situation | Use |
|---|---|
| Issue A cannot proceed until issue B is closed | `blocked_by` |
| Issue A is a duplicate or supersedes B | `refs` (or a label like `duplicate-of:<id>`) |
| Issue A documents the design that B implements | `refs` |
| Issue A and B both must finish, but in either order | Neither — they are not blocking |
| Issue A is a child of issue B | Set `parent` on creation; do not use `np rel add` |

When in doubt, prefer `refs` — it preserves the link without affecting readiness.

## What this skill does not cover

- **Setting a parent relationship** — set the `parent` field when creating the issue (`np-creating-issues`). `np` does not allow re-parenting via `np rel add`.
- **Reading the related issues' content** — use `np-reading-issues` to follow a `blocked_by` or `refs` edge into the linked issue.
- **Listing a single issue's relationships in context** — use `np show <ID>` from `np-reading-issues`.

## Common mistakes

- **Using `np rel add` for parent–child.** Parent attachment is a creation-time concern; `np rel add` will not accept it.
- **Adding `blocked_by` when `refs` is meant.** `blocked_by` removes the issue from the ready queue. Use it only when the dependency is real.
- **Forgetting `--author`.** Relationship mutations still require the author flag, even though they do not require a claim.
- **Inverting the arguments on remove.** `np rel remove` takes the same positional order as `np rel add` — `<A> <rel> <B>`. Removing in reverse order is a no-op.
