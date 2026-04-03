# Getting Started

This tutorial walks you through `np` from first install to comfortable daily use. Each section builds on what came before. By the end, you will know how to create issues, work on them, track what blocks what, and filter your backlog with labels.

`np` is a local-only CLI issue tracker. It stores everything in an embedded SQLite database under `.np/` in your workspace. There is no network, no remote sync, and no background daemon.

---

## Installation

Build `np` from source:

```
$ git clone https://github.com/pinkhop/nitpicking.git
$ cd nitpicking
$ make build
```

This produces a static binary at `dist/np`. Copy or symlink it somewhere on your `PATH`, or invoke it directly via `./dist/np`.

The rest of this guide assumes `np` is on your `PATH`.

---

## Initializing a Workspace

Every workspace needs its own database. Run `np init` at the root of your project, passing a short prefix that will appear in all issue IDs:

```
$ cd ~/projects/my-app
$ np init MYAPP
[ok] Initialized database with prefix MYAPP
```

The prefix becomes the first part of every issue ID, such as `MYAPP-a3bxr`. Choose something short and recognizable.

This creates a `.np/` directory containing the SQLite database:

```
my-app/
└── .np/
    └── nitpicking.db
```

`np` does not modify your git configuration, install hooks, or create background processes. Add `.np/` to your `.gitignore` if you do not want to track the database in version control.

> **Working from multiple machines?** If you are a solo developer who works from both a workstation and a laptop, you can commit `.np/` so the issue database travels with the repo. To avoid merge conflicts on the binary SQLite file, only modify the database from one machine at a time: push before switching machines, pull before resuming.

`np` discovers the database by walking up from your current directory, so you can run commands from any subdirectory:

```
$ cd ~/projects/my-app/src/api
$ np admin where
/Users/you/projects/my-app/.np
```

If you need to skip parent traversal, pass the global `--workspace` flag or set `NP_WORKSPACE`.

---

## Creating Issues

`np create` auto-detects how you want to create an issue:

- If stdin is a terminal, it launches the interactive TUI form.
- If stdin is a pipe, it reads a JSON object from stdin and writes JSON to stdout.

### Quickest scripted path

Create a task by piping JSON:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "task",
  "title": "Add user login endpoint"
}
JSONEND
{
  "id": "MYAPP-a3bxr",
  "role": "task",
  "title": "Add user login endpoint",
  "priority": "P2",
  "state": "open",
  "created_at": "2026-03-28T14:30:00.000Z"
}
```

The issue ID is generated automatically. Priority defaults to `P2` when omitted.

### More fields

You can also include description, acceptance criteria, labels, and a parent epic:

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "task",
  "title": "Add rate limiting to public endpoints",
  "priority": "P1",
  "description": "The /api/v1/* endpoints are unprotected. Add rate limiting before launch.",
  "acceptance_criteria": "All /api/v1 endpoints return 429 when rate exceeded.",
  "labels": ["kind:feature", "area:api"]
}
JSONEND
```

### Interactive path

If you are working directly in a terminal and want prompts instead of JSON:

```bash
$ np create
```

That opens the same creation flow as `np form create`.

### Listing issues

Use `np list` to see what you have:

```
$ np list
MYAPP-f7kkd  task  open (ready)  P1  Fix CORS headers on API responses
MYAPP-r2npq  task  open (ready)  P1  Add rate limiting to public endpoints
MYAPP-a3bxr  task  open (ready)  P2  Add user login endpoint

3 issues
```

Issues are sorted by priority by default, so the `P1` issues appear before the `P2`.

### Showing an issue

Use `np show` for full detail on a single issue:

```
$ np show MYAPP-r2npq
MYAPP-r2npq  task  Add rate limiting to public endpoints
────────────────────────────────────────────────────
area:api  kind:feature

Priority:  P1
State:     open (ready)

Claimed by:  (none)

Created:    2026-03-28 14:30 UTC
Revision:   0
Author:     alice

Description:
The /api/v1/* endpoints are unprotected. Add rate limiting before launch.

Acceptance Criteria:
All /api/v1 endpoints return 429 when rate exceeded.
```

---

## Claiming, Commenting, and Closing

Before you can update or close an issue, you must **claim** it. Claiming is `np`'s concurrency gate. It prevents two people or agents from mutating the same issue at the same time.

### Claiming an issue

```
$ np claim MYAPP-f7kkd --author alice
Claimed MYAPP-f7kkd
  Claim ID: a4dace30e46eb1ec14019c79a59c6b27
  Author: alice
  Stale at: 2026-03-28 16:30:00
```

The claim ID is a bearer token. Save it. You will need it for every subsequent mutation on this issue. If you lose it, you can steal the claim back after it goes stale, which defaults to 2 hours.

### Adding comments

Comments record your reasoning, trade-offs, and decisions. They do not require a claim.

For agents and scripts, use JSON input:

```bash
$ np json comment MYAPP-f7kkd --author alice <<'JSONEND'
{
  "body": "Root cause: Express app was not setting Access-Control-Allow-Origin. Fixed in middleware."
}
JSONEND
{
  "comment_id": "comment-1",
  "issue_id": "MYAPP-f7kkd",
  "author": "alice"
}
```

For humans working interactively at a terminal:

```bash
$ np form comment MYAPP-f7kkd
```

### Closing the issue

When the work is done, `np close` adds a closing comment and closes the issue in one step:

```
$ np close \
    --claim a4dace30e46eb1ec14019c79a59c6b27 \
    --reason "CORS headers now set via middleware for all API routes."
Closed MYAPP-f7kkd
```

Closed an issue by mistake? `np issue reopen` moves it back to open:

```
$ np issue reopen MYAPP-f7kkd --author alice
```

### Releasing instead of closing

Sometimes you claim an issue, start working, and realize you cannot finish right now. Release the claim instead of leaving it stuck:

```
$ np issue release \
    --claim a4dace30e46eb1ec14019c79a59c6b27
Released MYAPP-f7kkd
```

This moves the issue back to `open` so someone else, or future-you, can claim it.

---

## Updating an Issue

If you hold the claim on an issue, update it with either the interactive form or JSON stdin.

### Interactive update

```
$ np form update --claim <CLAIM-ID>
```

This opens a form pre-populated with the current values and updates only the fields you change.

### Scripted update

```bash
$ np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Add rate limiting to all public API endpoints",
  "priority": "P0",
  "comment": "Expanded scope after reviewing all public routes."
}
JSONEND
{
  "issue_id": "MYAPP-r2npq",
  "updated": true
}
```

`json update` follows PATCH semantics:

- Omitted fields are unchanged.
- Explicit `null` clears a field.
- Present values replace the current value.

---

## Issue States

Every issue has one of four primary states:

| State | Meaning |
|-------|---------|
| **open** | Available for work. This is the initial state. |
| **claimed** | Someone is actively working on it. |
| **closed** | Done. Can be reopened if needed. |
| **deferred** | Shelved for later. Not lost, just not now. |

### Deferring and undeferring

If an issue is not relevant right now, defer it to get it out of your active backlog. You need a claim first:

```
$ np claim MYAPP-a3bxr --author alice
Claimed MYAPP-a3bxr
  Claim ID: b8e1f20c...
  Author: alice
  Stale at: 2026-03-28 16:30:00

$ np issue defer \
    --claim b8e1f20c... \
    --until 2026-04-15
```

The `--until` date is informational. `np` does not auto-undefer, but `np admin doctor` can report overdue deferrals.

When you are ready to pick it back up:

```
$ np issue undefer MYAPP-a3bxr --author alice
```

This moves the issue back to `open`.

---

## Blocking Relationships

Sometimes one issue cannot proceed until another is resolved. `np` models this with `blocked_by` relationships, and they affect readiness directly.

### Adding a blocker

Suppose the rate-limiting task (`MYAPP-r2npq`) cannot start until the CORS fix (`MYAPP-f7kkd`) is deployed:

```
$ np rel add MYAPP-r2npq blocked_by MYAPP-f7kkd --author alice
Added blocked_by: MYAPP-r2npq → MYAPP-f7kkd
```

Now check what is ready for work:

```
$ np ready
MYAPP-f7kkd  task  open (ready)  P1  Fix CORS headers on API responses
MYAPP-a3bxr  task  open (ready)  P2  Add user login endpoint

2 ready
```

`MYAPP-r2npq` no longer appears because it is blocked. You can see blocked issues with `np blocked`:

```
$ np blocked
MYAPP-r2npq  task  open (blocked)  P1  Add rate limiting to public endpoints [← MYAPP-f7kkd]

1 blocked
```

### How blockers resolve

Once `MYAPP-f7kkd` is closed, the blocker is automatically resolved and `MYAPP-r2npq` becomes ready again. You do not need to remove the relationship manually.

### Using `np claim ready`

`np claim ready` picks the highest-priority ready issue and claims it in one step:

```
$ np claim ready --author alice
Claimed MYAPP-f7kkd
  Claim ID: c93ab...
  Author: alice
  Stale at: 2026-03-28 16:30:00
```

This is the recommended way to find work. It respects both priority and readiness, so you get the most important unblocked issue.

---

## Reference Relationships

Not every connection between issues is a blocker. Use `refs` to link related issues for context. `refs` is informational and does not affect readiness:

```
$ np rel add MYAPP-a3bxr refs MYAPP-r2npq --author alice
Added refs: MYAPP-a3bxr → MYAPP-r2npq
```

References are symmetric. If A refs B, then B also refs A. They appear in `np show` output so readers can follow related work.

---

## Labels

Labels attach `key:value` pairs to issues. There is no separate label-definition step.

### Adding labels at creation

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "task",
  "title": "Sanitize HTML in comment bodies",
  "priority": "P1",
  "labels": ["kind:bug", "area:api"]
}
JSONEND
```

### Adding labels to existing issues

Label changes on an existing issue require a claim:

```
$ np claim MYAPP-a3bxr --author alice
Claimed MYAPP-a3bxr
  Claim ID: d47f8...

$ np label add area:api --claim d47f8...
$ np label remove area --claim d47f8...
```

### Filtering by label

Labels become useful when filtering. Pass `--label` to `np list` or `--with-label` to `np claim ready`:

```
$ np list --label area:api
MYAPP-w4ttx  task  open (ready)  P1  Sanitize HTML in comment bodies

1 issues
```

Use `key:*` to match any value for a given key:

```
$ np list --label kind:*
MYAPP-w4ttx  task  open (ready)  P1  Sanitize HTML in comment bodies

1 issues
```

Claim the highest-priority ready bug:

```
$ np claim ready --author alice --with-label kind:bug
Claimed MYAPP-w4ttx
  Claim ID: d47f8...
  Author: alice
  Stale at: 2026-03-28 16:30:00
```

### Conventions

Labels are free-form, but a few conventions are useful:

- **`kind:`** — categorize issues: `kind:bug`, `kind:feature`, `kind:refactor`, `kind:debt`
- **`area:`** — scope by system area: `area:api`, `area:auth`, `area:frontend`
- **Cross-system references** — correlate with external trackers: `jira:FOO-312`, `gh:42`

To see all labels currently in use:

```
$ np label list-all
area:api
kind:bug
```

---

## What's Next

So far you have worked with standalone tasks. To organize tasks into hierarchies and plan larger bodies of work, see [Organizing Work](organizing-work.md).

For quick reference:

- **[Command Reference](command-reference.md)** — every command, flag, and exit code
- **[Key Concepts](key-concepts.md)** — design philosophy, state machines, readiness, and claiming in depth
- **`np --help`** — complete list of commands in your terminal
- **`np admin doctor`** — detects stale claims, analyzes why no issues are ready, and suggests unblocking actions
