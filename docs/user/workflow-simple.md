# Workflow: Simple Task-Only Serial Development

The simplest way to use `np` — a single developer (human or AI agent) working through tasks one at a time, without epics or hierarchy.

---

## When to Use This Workflow

This workflow fits when:

- **Small projects** — a handful of tasks that do not need grouping or decomposition.
- **Focused sprints** — you know exactly what needs to be done and want to track progress without overhead.
- **Exploratory work** — prototyping, spikes, or investigations where the shape of the work is not yet clear enough for epic-level planning.
- **Solo operation** — one human or one AI agent working serially; minimal coordination overhead.

If your tasks naturally group into features, or you find yourself wanting to track "this set of tasks must all be done before the feature is complete", graduate to the [epic-driven workflow](workflow-epics.md).

---

## Setting Up

Initialize a database and choose an author name:

```
$ np init MYAPP
[ok] Initialized database with prefix MYAPP

$ np agent name
bold-river-stride
```

Use the author name consistently across your session. Agents should generate a name at session start with `np agent name`; humans should pick a stable identifier such as `alice`, `david`, or `david-laptop`.

### Solo-Human Habits

If you are the only human using `np`, keep the workflow disciplined anyway:

- Reuse the same `--author` name every day unless you intentionally want a different identity in the audit trail.
- Still claim before editing or closing. Even when there is no concurrency, claims mark work as "in progress" and give you an explicit handle for finishing or releasing it later.
- Close issues immediately when finished. If you stop partway through, release the claim so `np ready` reflects reality the next time you sit down.
- Add short comments that explain what changed or why you stopped. Future-you is another reader of the audit trail.

---

## Creating Tasks

Create tasks with JSON stdin, or run `np create` interactively in a TTY:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Add input validation to login form",
  "priority": "P1"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Write unit tests for auth module",
  "priority": "P1"
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Update API docs for /login endpoint",
  "priority": "P3"
}
JSONEND
```

Priorities determine the order in which `np ready` and `np claim ready` present work:

| Priority | Meaning |
|----------|---------|
| P0 | Critical — security, data loss, broken builds |
| P1 | High — major features, important bugs |
| P2 | Medium (default) |
| P3 | Low — polish, optimization |
| P4 | Backlog — future ideas |

---

## Finding Work

Use `np ready` to see what is available:

```
$ np ready
MYAPP-2e22n  task  P1  Add input validation to login form
MYAPP-r44w9  task  P1  Write unit tests for auth module
MYAPP-x64m6  task  P3  Update API docs for /login endpoint

3 ready
```

Issues are sorted by priority (P0 first), then by creation date (oldest first). P1 tasks appear before P3.

---

## The Claim-Work-Comment-Close Cycle

This is the core loop. For each task:

### 1. Claim

```
$ np claim ready --author alice
[ok] Claimed MYAPP-2e22n
  Claim ID: f2fa05ba73d90760db00682f21df60f0
```

Save the claim ID — you will need it to close or release the issue. If you lose it, wait for the claim's stale duration to expire (default: 2 hours), then reclaim the issue normally — stale claims are overwritten automatically. `np claim ready` automatically picks the highest-priority ready issue, so you do not need to remember issue IDs.

For a solo human, claiming is still useful even without competing agents: it distinguishes "I plan to work on this now" from "this is still sitting in the queue."

### 2. Do the Work

This happens outside of `np` — write code, run tests, fix bugs. `np` does not need to know what you are doing until you are ready to report back.

### 3. Comment

Document your reasoning, approach, and trade-offs before closing:

```bash
$ np json comment MYAPP-2e22n --author alice <<'JSONEND'
{
  "body": "Added email format validation and password length check."
}
JSONEND
```

Comments do not require a claim. You can add them at any time — before, during, or after your work.

### 4. Close

Close the task with a reason:

```
$ np close --claim f2fa05ba73d90760db00682f21df60f0 --reason "Implemented email and password validation. All tests pass."
Closed MYAPP-2e22n
```

`np close` adds the reason as a comment and closes the issue in a single step.

If you are not done, release instead of leaving a stale claim behind:

```
$ np issue release --claim f2fa05ba73d90760db00682f21df60f0
Released MYAPP-2e22n
```

Use release when you pause for the day, realize the task needs more thought, or want the issue to return to the ready pool unchanged.

### Repeat

Claim the next ready issue and repeat:

```
$ np claim ready --author alice
[ok] Claimed MYAPP-r44w9
  Claim ID: a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6
```

---

## Checking Progress

Use `np admin tally` for a dashboard:

```
$ np admin tally
Open      2
Claimed   0
Deferred  0
Closed    1
Ready     2
Blocked   0

3 total
```

Use `np list --state closed` to review completed work:

```
$ np list --state closed
MYAPP-2e22n  task  P1  closed  Add input validation to login form
```

---

## When to Graduate

Consider switching to the [epic-driven workflow](workflow-epics.md) when you notice:

- **Tasks naturally group** — you find yourself mentally grouping tasks into "the auth feature" or "the API overhaul".
- **Ordering matters** — some tasks must be completed before others. Use `blocked_by` relationships or organize under an epic.
- **You want progress tracking** — `np epic status` shows completion percentages for groups of related work.
- **Multiple agents or developers** — epic decomposition helps coordinate who works on what without explicit orchestration.

The simple and epic-driven workflows are not mutually exclusive. You can create standalone tasks alongside epics, mixing both styles as needed.
