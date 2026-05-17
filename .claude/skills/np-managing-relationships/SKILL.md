---
name: np-managing-relationships
description: Use when the agent needs to express, remove, or list a dependency or informational link between two `np` (nitpicking) issues — adding `blocked_by` because one issue cannot proceed until another finishes, adding `refs` for a "see also" pointer, removing a relationship that no longer applies, or listing the workspace's relationships at a glance. Triggers on prompts like "mark FOO-a3bxr as blocked by FOO-c4npt", "FOO-12345 references FOO-67890", "remove the blocker", "show me the blocking chains".
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np agent name:*) Bash(np claim:*) Bash(np issue release:*) Bash(np json create:*) Bash(np json update:*) Bash(np rel:*) Bash(np show:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-managing-relationships

## Overview

`np` models three relationship kinds between issues:

| Kind | Direction | Affects readiness? |
|---|---|---|
| `blocked_by` / `blocks` | Directional dependency (mirror of each other) | Yes — a blocked issue is not ready |
| `refs` | Informational link, "see also" | No |
| parent–child | Structural hierarchy | Yes (children of a deferred or blocked ancestor are not ready) |

Parent–child can be set at creation time via the `parent` field on `np json create` (see `np-creating-issues`), or after creation via `np rel add … child_of …` / `np rel add … parent_of …` (with `--claim` on the child), `np json update`'s `parent` field, or `np rel parent detach` to break a link. This skill covers all three relationship kinds.

## Author identity

Every relationship mutation requires `--author <name>`. If no name has been chosen for this session, generate one:

```bash
$ np agent name --seed=$PPID
agent-blue-seal-echo
```

Reuse that name for the rest of the session. `blocked_by`, `blocks`, and `refs` mutations do **not** require a claim — they can be added or removed without claiming either issue. Parent–child mutations via `np rel add … child_of …` / `parent_of …` **do** require `--claim` on the child issue, because they mutate the child's `parent` field.

## Adding a relationship

```bash
$ np rel add FOO-a3bxr blocked_by FOO-c4npt --author agent-blue-seal-echo
$ np rel add FOO-a3bxr blocks FOO-q1w2e --author agent-blue-seal-echo
$ np rel add FOO-a3bxr refs FOO-67890 --author agent-blue-seal-echo
```

The positional shape is `<issue-A> <rel> <issue-B>`. Pick the relationship name that reads naturally:

- `A blocked_by B` — A cannot proceed until B is resolved.
- `A blocks B` — same edge from the other side; pick whichever reads better in context.
- `A refs B` — A points at B for context, with no readiness implication.
- `A child_of B` / `A parent_of B` — structural hierarchy; **requires `--claim <CID>` on the child** (see "Parent–child after creation" below).

If the exact subcommand syntax is unfamiliar, run `np rel add --help` to confirm.

## Removing a relationship

`np rel remove` mirrors `np rel add` exactly for `blocked_by`, `blocks`, and `refs` — same positional order, same accepted relationship names:

```bash
$ np rel remove FOO-a3bxr blocked_by FOO-c4npt --author agent-blue-seal-echo
$ np rel remove FOO-a3bxr refs FOO-67890 --author agent-blue-seal-echo
```

To remove a parent–child link, use `np rel parent detach` (see "Parent–child after creation" below).

## Parent–child after creation

Parent–child is most commonly set at creation time via the `parent` field on `np json create` (see `np-creating-issues`). When that is not possible — re-parenting, attaching an orphan, or detaching a child — three paths exist:

```bash
# Attach FOO-child as a child of FOO-parent (requires a claim on the child).
$ np claim FOO-child --author agent-blue-seal-echo
# capture the returned claim ID
$ np rel add FOO-child child_of FOO-parent --claim <CID> --author agent-blue-seal-echo
$ np issue release --claim <CID>

# Equivalent mirror form:
$ np rel add FOO-parent parent_of FOO-child --claim <CID> --author agent-blue-seal-echo

# Detach a parent-child link (argument order does not matter; no explicit claim needed).
$ np rel parent detach FOO-child FOO-parent --author agent-blue-seal-echo
```

`np rel parent` also exposes read-only views of the hierarchy — `np rel parent children <ID>` lists direct children of any issue (epic or task), and `np rel parent tree <ID>` shows the full descendant tree. `np json update --claim <CID>` accepts a `parent` field as an alternative to `np rel add … child_of …` when other fields are being updated at the same time.

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
| Issue A is a child of issue B | Set `parent` on creation, or use `np rel add A child_of B --claim <CID>` after creation |

When in doubt, prefer `refs` — it preserves the link without affecting readiness.

## What this skill does not cover

- **Setting a parent relationship at creation time** — use the `parent` field on `np json create` (see `np-creating-issues`). After-creation re-parenting and detachment are covered here.
- **Reading the related issues' content** — use `np-reading-issues` to follow a `blocked_by` or `refs` edge into the linked issue.
- **Listing a single issue's relationships in context** — use `np show <ID>` from `np-reading-issues`.

## Common mistakes

- **Using `np rel add` for parent–child without `--claim`.** `child_of` / `parent_of` mutate the child issue's `parent` field, so the child must be claimed and the claim ID passed to `--claim`.
- **Adding `blocked_by` when `refs` is meant.** `blocked_by` removes the issue from the ready queue. Use it only when the dependency is real.
- **Forgetting `--author`.** Relationship mutations still require the author flag, even though they do not require a claim.
- **Inverting the arguments on remove.** `np rel remove` takes the same positional order as `np rel add` — `<A> <rel> <B>`. Removing in reverse order is a no-op.
