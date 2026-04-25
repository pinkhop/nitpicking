# Epics

This guide is for the point where labels and flat tasks are not enough. Use epics when a high-level idea needs explicit decomposition, progress tracking, or grouped closure.

If you are still comfortable working from standalone tasks and labels, stay with [Quickstart](quickstart.md), [Daily Work](daily-work.md), and [Labels](labels.md). `np` does not require epics.

## When To Introduce Epics

Add epics when one or more of these is true:

- several tasks clearly belong to one larger deliverable
- you want to track feature progress instead of only individual task progress
- you keep writing task titles that really describe bundles of work
- you want to shelve or resume a whole initiative at once

## The Core Model

- A `task` is direct work.
- An `epic` organizes related work.
- An empty open epic is `ready` because it needs decomposition.
- An epic with children is no longer ready; progress happens through its children.
- An epic becomes `completed` when all of its children are closed or recursively complete.
- You close completed epics with `np epic close-completed`.

## Create the Top-Level Epic

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Authentication overhaul",
  "description": "Replace legacy session-based auth with JWT tokens.",
  "priority": "P1"
}
JSONEND
```

At this point the epic is a planning gap. It has no children yet, so it appears in the ready queue.

## Decompose It

Claim the epic while planning:

```bash
$ np claim FOO-c4npt --author alice
```

Record the plan in a comment if helpful:

```bash
$ np json comment FOO-c4npt --author alice <<'JSONEND'
{
  "body": "Decomposition plan: token generation, middleware replacement, migration work."
}
JSONEND
```

Create child tasks:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Implement JWT token generation service",
  "priority": "P1",
  "parent": "FOO-c4npt"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Replace session middleware with JWT validation",
  "priority": "P1",
  "parent": "FOO-c4npt"
}
JSONEND
```

If one sub-area is large enough to need its own breakdown, make it a child epic:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Session-to-JWT migration",
  "priority": "P1",
  "parent": "FOO-c4npt"
}
JSONEND
```

Then release the top-level claim:

```bash
$ np issue release --claim <CLAIM-ID>
```

## Parent-Child Rules

The recommended planning model is:

```text
epic
├── task
├── task
└── epic
    ├── task
    └── task
```

`np` enforces a maximum depth of three levels:

```text
Level 1: Epic
Level 2: Epic or Task
Level 3: Task
```

If you want deeper nesting, that is usually a sign that the hierarchy is doing too much. Use `blocked_by` relationships to express sequencing between peer issues instead of adding more depth.

## View the Structure

Show direct children:

```bash
$ np epic children <EPIC-ID>
```

Show the full descendant tree:

```bash
$ np rel parent tree <EPIC-ID>
```

Show where one issue sits inside the broader hierarchy:

```bash
$ np rel tree <ISSUE-ID>
```

Render the full graph when the structure gets complicated:

```bash
$ np rel graph --format text
$ np rel graph --format dot --output issues.dot
```

## Work Through the Children

Once the epic is decomposed, the operating loop goes back to normal:

1. `np claim ready --author <name>`
2. do the work
3. comment on findings
4. close the task

You do not work an epic to completion directly. You work the children.

## Track Progress

See progress for all open epics:

```bash
$ np epic status
```

See progress for one epic:

```bash
$ np epic status <EPIC-ID>
```

See epics that are ready to close:

```bash
$ np epic status --completed-only
```

## Close Completed Epics

When all children are resolved, batch-close them:

```bash
$ np epic close-completed --author alice
```

Preview first if you want:

```bash
$ np epic close-completed --dry-run --author alice
```

## When To Use Labels Instead

Use an epic when the group itself is a deliverable and you care about structural completion.

Use labels when the grouping is just for filtering or reporting, such as:

- `sprint:2026-w16`
- `area:auth`
- `source:user-report`

Read [Labels](labels.md) for that layer.
