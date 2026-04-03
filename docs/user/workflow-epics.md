# Workflow: Epic-Driven Top-Down Development

This guide walks through a structured development workflow using epics and tasks. The pattern is top-down decomposition: start with a high-level goal, break it into pieces, and work through the pieces one at a time.

This is the most common workflow in nitpicking — it maps naturally to how most features, refactors, and initiatives are planned and executed.

---

## The pattern

1. Create a top-level epic for the initiative.
2. Decompose it into child tasks (and child epics for large sub-features).
3. Work through children: claim, do the work, comment, close.
4. Track progress with `np epic status`.
5. When all children are resolved, batch-close completed epics.

Each step is explained below with a running example: an "Authentication overhaul" initiative.

---

## Step 1: Create the top-level epic

An epic represents a body of work that will be broken into smaller pieces. Start by creating one for the initiative:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Authentication overhaul",
  "description": "Replace legacy session-based auth with JWT tokens. Covers token generation, middleware, and migration.",
  "priority": "P1"
}
JSONEND
```

At this point the epic has no children — it is in the "ready" pool because childless epics signal a planning gap that needs decomposition.

---

## Step 2: Decompose into children

Claim the epic, plan the breakdown, then create child issues. You need the claim to add comments documenting your plan, though creating children does not technically require it (parent–child relationships require a claim on the child, not the parent — but claiming the epic first is good practice for coordination).

```bash
$ np claim PKHP-a3bxr --author alice
Claimed PKHP-a3bxr (claim: 7f2e9a...)

$ np json comment PKHP-a3bxr --author alice <<'JSONEND'
{
  "body": "Decomposition plan:\n1. JWT token generation service (task)\n2. Auth middleware replacement (task)\n3. Migration from sessions to tokens (epic with child tasks)"
}
JSONEND
```

Now create the children:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Implement JWT token generation service",
  "priority": "P1",
  "parent": "PKHP-a3bxr"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Replace session middleware with JWT validation",
  "priority": "P1",
  "parent": "PKHP-a3bxr"
}
JSONEND
```

For the migration work — which is complex enough to warrant its own decomposition — create a child epic:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Session-to-JWT migration",
  "description": "Migrate existing users from session-based auth to JWT. Includes data migration, backward compatibility, and rollback plan.",
  "priority": "P1",
  "parent": "PKHP-a3bxr"
}
JSONEND
```

And decompose that child epic into its own tasks:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Write session-to-JWT data migration script",
  "priority": "P1",
  "parent": "PKHP-h2mtv"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Add backward-compatible session fallback",
  "priority": "P2",
  "parent": "PKHP-h2mtv"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Write rollback procedure and test it",
  "priority": "P1",
  "parent": "PKHP-h2mtv"
}
JSONEND
```

Release the claim on the top-level epic now that planning is done:

```
$ np issue release --claim 7f2e9a...
Released PKHP-a3bxr
```

The hierarchy now looks like this:

```
PKHP-a3bxr  Authentication overhaul (epic)
├── PKHP-d4kwr  Implement JWT token generation service (task)
├── PKHP-f7npq  Replace session middleware with JWT validation (task)
└── PKHP-h2mtv  Session-to-JWT migration (epic)
    ├── PKHP-k9ryz  Write session-to-JWT data migration script (task)
    ├── PKHP-m3qpw  Add backward-compatible session fallback (task)
    └── PKHP-p6xhn  Write rollback procedure and test it (task)
```

### When to use child epics vs only tasks

Use a child epic when a piece of work is large enough that it needs its own decomposition — typically when it involves 3+ related tasks that form a logical unit. If a piece of work is a single, atomic action, make it a task.

The rule of thumb: if you would write multiple child tasks for something, wrap them in a child epic. If it is one thing to do, it is a task.

### The 3-layer depth limit

Nitpicking enforces a maximum hierarchy depth of 3 levels:

```
Level 1:  Epic (e.g., "Authentication overhaul")
Level 2:    ├── Task or Epic (e.g., "Session-to-JWT migration")
Level 3:    │     └── Task only (e.g., "Write migration script")
```

This means you cannot nest epics more than two levels deep — a level-2 epic's children must be tasks, not further epics. If you find yourself wanting deeper nesting, reconsider your decomposition: use `blocked_by` relationships to express sequencing between peer issues instead of deeper hierarchy.

---

## Step 3: Work through children

The work loop is straightforward: claim a ready issue, do the work, document your reasoning, close it.

```bash
$ np claim ready --author bob
Claimed PKHP-d4kwr (claim: a1b2c3...)

# ... do the work ...

$ np json comment PKHP-d4kwr --author bob <<'JSONEND'
{
  "body": "Implemented RS256 JWT signing with 15-minute expiry. Keys stored in environment variables per security guidelines."
}
JSONEND

$ np close --claim a1b2c3... \
    --reason "JWT generation service complete. All tests pass."
Closed PKHP-d4kwr
```

`np claim ready` automatically picks the highest-priority ready issue. As children are completed, previously blocked issues may become ready. The cycle continues until all children are closed.

---

## Step 4: Track progress

Use `np epic status` to see how each epic is progressing:

```
$ np epic status

○ PKHP-a3bxr Authentication overhaul
  1/3 children closed (33%)
○ PKHP-h2mtv Session-to-JWT migration
  0/3 children closed (0%)
```

For a specific epic:

```
$ np epic status PKHP-a3bxr

○ PKHP-a3bxr Authentication overhaul
  1/3 children closed (33%)
```

To see just epics that are ready to close:

```
$ np epic status --completed-only

(no output — none are completed yet)
```

You can also view the children directly:

```
$ np epic children PKHP-a3bxr

PKHP-d4kwr  Implement JWT token generation service    closed  P1
PKHP-f7npq  Replace session middleware with JWT...     open   P1
PKHP-h2mtv  Session-to-JWT migration                   open   P1
```

---

## Step 5: The `completed` secondary state and batch closing

Epic completion is determined by the `completed` secondary state — an epic is complete when *all* of its children are either closed (for tasks) or recursively complete (for child epics). You never close an epic directly.

Once all children of the "Session-to-JWT migration" epic are closed, it is completed. Once that epic and all sibling tasks under "Authentication overhaul" are also resolved, the top-level epic is completed too.

Use `np epic close-completed` to batch-close all completed epics:

```
$ np epic close-completed --author alice

Closed PKHP-h2mtv Session-to-JWT migration
Closed PKHP-a3bxr Authentication overhaul
```

You can preview what would be closed without actually closing:

```
$ np epic close-completed --dry-run --author alice

Would close PKHP-h2mtv Session-to-JWT migration
Would close PKHP-a3bxr Authentication overhaul
```

### Why secondary-state completion?

Secondary-state completion prevents premature closure. An epic cannot be marked "done" while it still has open work — the system enforces this structurally. It also means you never need to remember to close an epic; `epic close-completed` handles it as a batch operation at the end of a work session.

---

## Putting it together

The full cycle for epic-driven development:

1. **Plan**: Create the top-level epic. Claim it, document your decomposition plan as a comment, create children.
2. **Execute**: Agents claim ready tasks, do the work, comment on their approach, and close.
3. **Monitor**: Use `np epic status` to track progress across the initiative.
4. **Complete**: Run `np epic close-completed` to close all fully resolved epics.

This pattern scales from a single feature with 3 tasks to a multi-phase initiative with nested epics — the mechanics are the same at every level.
