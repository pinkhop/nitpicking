# Organizing Work

This guide covers how to structure issues into hierarchies using parent-child relationships and epics. It builds on the concepts from [Getting Started](getting-started.md) — if you have not worked through that tutorial yet, start there.

---

## Parent-Child Relationships

Any issue can be attached to a parent epic. This lets you group related tasks under a common umbrella.

### Attaching a child at creation

The simplest way to build a hierarchy is to include `parent` in the create JSON:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Authentication overhaul"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Replace session tokens with JWTs",
  "parent": "MYAPP-h5mqp"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Add refresh token rotation",
  "parent": "MYAPP-h5mqp"
}
JSONEND
```

### Attaching an existing issue

Use `np json update` or `np form update` to set or change an issue's parent after the fact. Both require a claim on the child issue.

```bash
$ np claim MYAPP-a3bxr --author alice
Claimed MYAPP-a3bxr
  Claim ID: c1a1m2...

$ np json update --claim c1a1m2... <<'JSONEND'
{
  "parent": "MYAPP-h5mqp"
}
JSONEND
```

To detach an issue from its parent:

```
$ np rel parent detach MYAPP-a3bxr MYAPP-h5mqp --author alice
```

### Depth limit

Hierarchies are limited to **three tiers**:

```
Level 1:  Epic (root)
Level 2:    ├── Epic or Task
Level 3:    │     └── Task only
```

This keeps hierarchies flat enough to reason about. Use blocking relationships (`blocked_by`) for sequencing rather than deeper nesting.

### Viewing the hierarchy

List the direct children of an epic:

```
$ np epic children MYAPP-h5mqp
MYAPP-b2frd  task  open (ready)  P2  Replace session tokens with JWTs
MYAPP-c9gnv  task  open (ready)  P2  Add refresh token rotation
MYAPP-a3bxr  task  open (ready)  P2  Add user login endpoint

3 children
```

Show the full descendant tree from any issue:

```
$ np rel parent tree MYAPP-h5mqp
MYAPP-h5mqp  epic  open  P1  Authentication overhaul
├── MYAPP-b2frd  task  open (ready)  P2  Replace session tokens with JWTs
├── MYAPP-c9gnv  task  open (ready)  P2  Add refresh token rotation
└── MYAPP-a3bxr  task  open (ready)  P2  Add user login endpoint
```

For a cross-cutting view of all issues and their relationships, use `np rel graph`:

```
$ np rel graph --format text
```

The graph also supports Graphviz DOT output for rendering as an image:

```
$ np rel graph --format dot --output issues.dot
$ dot -Tpng issues.dot -o issues.png
```

---

## Epics

An epic is an issue whose job is to **organize** work, not to be worked on directly. Its completion is determined by the `completed` secondary state: an epic is done when all its children are closed.

### The epic lifecycle

The lifecycle of an epic is different from a task:

1. **Create** the epic to describe the body of work.
2. **Claim** it and **decompose** it into child tasks (and sub-epics, if the work is large enough).
3. **Release** the epic — you have planned the work; now others (or future-you) can claim and complete the children.
4. The children get claimed, worked on, and closed — independently and possibly in parallel.
5. When every child is closed, the epic reaches the **completed** secondary state.
6. **Close** the epic with `np epic close-completed`.

### Readiness

An open epic with **no children** is considered ready — it needs to be decomposed. Once it has children, it is no longer ready; it is active (waiting for children to be completed).

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Performance improvements"
}
JSONEND

$ np ready
MYAPP-k8jds  epic  open (ready)  P2  Performance improvements

1 ready
```

The epic appears in `np ready` because it has no children yet — it is waiting to be planned. Once you add children, it drops out of the ready list.

### Decomposing an epic

Claim the epic, create its children, then release:

```bash
$ np claim MYAPP-k8jds --author alice
Claimed MYAPP-k8jds
  Claim ID: f19a...
  Author: alice
  Stale at: 2026-03-29 09:00:00

$ np create --author alice <<'JSONEND'
{
  "title": "Add Redis caching layer",
  "parent": "MYAPP-k8jds"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Optimize database queries in search",
  "parent": "MYAPP-k8jds"
}
JSONEND

$ np issue release --claim f19a...
Released MYAPP-k8jds
```

The epic is now active — its children are ready for work.

### Why you cannot close an epic directly

Closing an epic requires that all its children are closed first. If you try to close an epic that still has open children, `np` will refuse — this protects you from accidentally marking work as done when tasks remain.

### Checking epic progress

Use `np epic status` to see how your epics are progressing:

```
$ np epic status MYAPP-h5mqp
● MYAPP-h5mqp Authentication overhaul [active]
  1/3 children closed (33%)
```

Without an issue ID, it shows all open epics:

```
$ np epic status
● MYAPP-h5mqp Authentication overhaul [active]
  1/3 children closed (33%)
○ MYAPP-k8jds Performance improvements [active]
  0/2 children closed (0%)
```

To see only epics that are ready to close:

```
$ np epic status --completed-only
● MYAPP-h5mqp Authentication overhaul — Completed
  3/3 children closed (100%)
```

### Closing completed epics

Once all children are closed, close the epic with `np epic close-completed`:

```
$ np epic close-completed --author alice
✓ Closed MYAPP-h5mqp Authentication overhaul

1 of 1 completed epics closed.
```

This command finds all epics in the completed state and closes them in one batch. Use `--dry-run` to preview what would be closed:

```
$ np epic close-completed --author alice --dry-run
Would close 1 completed epics:
  MYAPP-h5mqp Authentication overhaul
```

---

## Putting It Together

A typical workflow for larger projects combines everything from this guide and the getting-started tutorial:

1. **Create an epic** for a body of work.
2. **Decompose** it into tasks (and sub-epics if needed), using `--parent` to build the hierarchy.
3. **Add blocking relationships** between tasks to express sequencing: `np rel add TASK-B blocked_by TASK-A --author alice`.
4. **Use `np claim ready`** to pick the most important, unblocked task and start working.
5. **Close tasks** as you finish them — this unblocks downstream tasks and advances epic progress.
6. **Close completed epics** with `np epic close-completed` when all children are done.

For quick reference:

- **[Getting Started](getting-started.md)** — issue creation, claiming, closing, states, blockers, labels.
- **[Command Reference](command-reference.md)** — every command, flag, and exit code.
- **`np admin doctor`** — detects stale claims, analyzes readiness, and suggests unblocking actions.
