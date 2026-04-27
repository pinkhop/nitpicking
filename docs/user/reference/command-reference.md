# Command Reference

Complete reference for every `np` command. Each entry includes the synopsis, all flags with defaults, examples of human-readable and `--json` output, exit codes, and notes on edge cases.

Commands are grouped by the categories shown in `np help`.

> **Conventions used below:**
> - `<REQUIRED>` — a required positional argument.
> - `[OPTIONAL]` — an optional argument or flag.
> - Default values are shown in parentheses after the flag description.

## Terminology

- **Ready queue** — the set of issues that can be picked up now.
- **Stale time** — the moment an active claim stops counting as active.
- **Claim conflict** — exit code `3`, meaning the issue still has an active claim and the claim you supplied does not authorize the mutation.
- **Example IDs** — examples use the `FOO-xxxxx` issue ID shape consistently.
- **Example times** — example timestamps use UTC and cluster around `2026-04-01`.

---

## Table of Contents

- [Setup](#setup)
  - [init](#init)
- [Core Workflow](#core-workflow)
  - [create](#create)
  - [show](#show)
  - [list](#list)
  - [claim](#claim)
  - [close](#close)
  - [ready](#ready)
  - [blocked](#blocked)
- [Issues](#issues)
  - [issue search](#issue-search)
  - [issue release](#issue-release)
  - [issue reopen](#issue-reopen)
  - [issue undefer](#issue-undefer)
  - [issue defer](#issue-defer)
  - [issue delete](#issue-delete)
  - [issue history](#issue-history)
  - [issue orphans](#issue-orphans)
  - [epic status](#epic-status)
  - [epic close-completed](#epic-close-completed)
  - [epic children](#epic-children)
  - [rel add](#rel-add)
  - [rel list](#rel-list)
  - [rel remove](#rel-remove)
  - [rel parent children](#rel-parent-children)
  - [rel parent tree](#rel-parent-tree)
  - [rel parent detach](#rel-parent-detach)
  - [rel issue](#rel-issue)
  - [rel tree](#rel-tree)
  - [rel graph](#rel-graph)
  - [label add](#label-add)
  - [label remove](#label-remove)
  - [label list](#label-list)
  - [label list-all](#label-list-all)
  - [label propagate](#label-propagate)
  - [comment list](#comment-list)
  - [comment search](#comment-search)
  - [form create](#form-create)
  - [form update](#form-update)
  - [form comment](#form-comment)
- [Agent Toolkit](#agent-toolkit)
  - [json create](#json-create)
  - [json update](#json-update)
  - [json comment](#json-comment)
  - [agent name](#agent-name)
  - [agent prime](#agent-prime)
- [Admin](#admin)
  - [admin backup](#admin-backup)
  - [admin completion](#admin-completion)
  - [admin doctor](#admin-doctor)
  - [admin gc](#admin-gc)
  - [admin reset](#admin-reset)
  - [admin restore](#admin-restore)
  - [admin tally](#admin-tally)
  - [admin upgrade](#admin-upgrade)
  - [admin where](#admin-where)
  - [import jsonl](#import-jsonl)
- [Info](#info)
  - [version](#version)

---

## Setup

Commands for setting up `np` in a workspace.

---

### init

Initialize a new nitpicking workspace rooted at the current directory. Creates a `.np/` directory containing the SQLite database.

**Synopsis:**

```
np init [options] <PREFIX>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `PREFIX` | Project prefix for issue IDs. Convention is 2–4 uppercase letters (e.g., `FOO`, `APP`). Every issue ID will start with this prefix followed by a hyphen and a random suffix. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON instead of human-readable text. |

**Examples:**

```text
$ np init FOO
[ok] Initialized database with prefix FOO
```

```json
$ np init FOO --json
{
  "prefix": "FOO"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Database created successfully. |
| 5 | Database error — most commonly, a `.np/` directory already exists in the current directory. |

**Notes:**

- The prefix is stored permanently in the database; it cannot be changed after initialization.
- Running `np init` in a directory that already has a `.np/` directory produces exit code 5.
- `np` discovers the database by walking up from the current working directory, so you only need one `.np/` per workspace tree — even if the project spans multiple subdirectories.
- Add `.np/` to your `.gitignore` if you do not want to track the database in version control.

---

## Core Workflow

Commands for creating, viewing, claiming, and closing issues. These form the core workflow of `np`.

---

### create

Create a new issue. Auto-detects input mode: when stdin is a pipe, reads a JSON object and writes JSON output; when stdin is a terminal, launches an interactive form.

**Synopsis:**

```
np create [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required for pipe mode; collected by form in TTY mode. Env: `NP_AUTHOR`. |

**Examples:**

```bash
$ np create --author alice <<'JSONEND'
{
  "role": "epic",
  "title": "Authentication overhaul",
  "priority": "P1"
}
JSONEND
```

```bash
$ np create
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue created successfully. |
| 4 | Validation error (e.g., missing required fields, invalid role or priority). |
| 5 | Database error. |

**Notes:**

- In pipe mode, the JSON object accepts the same content fields as `json create`, except the root `create` command does not expose `--with-claim`.
- In pipe mode, output is always JSON. There is no `--json` flag on `create`.
- In TTY mode, the interactive form collects all fields interactively.

---

### show

Display full details for a single issue, including state, relationships, labels, readiness, and completion status (for epics).

**Synopsis:**

```
np show [options] <ISSUE-ID>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | The issue ID to display. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np show FOO-a3bxr
FOO-a3bxr  Add user login endpoint
Role: task  |  State: open  |  Priority: P2
Revision: 1  |  Author: alice
```

```json
$ np show FOO-a3bxr --json | jq '.state, .claim_author, .claim_stale_at'
"claimed"
"alice"
"2026-04-01T11:30:00.000Z"
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue displayed successfully. |
| 2 | Issue not found. |

**Notes:**

- For claimed issues, JSON output includes `claim_author`, `claimed_at`, and `claim_stale_at`.
- Claim IDs are only returned at claim time. `show --json` helps inspect claim ownership and staleness, but it does not reveal the bearer token again.

---

### list

List issues with optional filtering by state, role, label, parent, and readiness.

**Synopsis:**

```
np list [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--state`, `-s` | Filter by state: `open`, `closed`, `deferred`. Repeatable. |
| `--role`, `-r` | Filter by role: `task` or `epic`. Repeatable. |
| `--label` | Filter by label in `key:value` format. Repeatable. |
| `--parent` | Filter by parent epic ID. Repeatable. |
| `--ready` | Show only ready issues. |
| `--all`, `-a` | Include all issues regardless of state, including closed. |
| `--order` | Sort order. One of `ID` (default), `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `PRIORITY`, `ROLE`, `STATE`, `TITLE`, or `MODIFIED`. Append `:asc` or `:desc` to set direction (ascending is the default). |
| `--columns` | Comma-separated list of columns to display. Valid columns: `ID`, `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `PRIORITY`, `ROLE`, `STATE`, `TITLE`. Replaces the previous `--timestamps` flag. |
| `--limit`, `-n` | Maximum number of results. Default: 20. |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np list --ready
FOO-a3bxr  P2  task  Add user login endpoint
FOO-b7mqd  P2  task  Write tests
```

```json
$ np list --state closed --json
[ ... closed issues as JSON ... ]
```

```text
$ np list --label kind:bug --ready
[ ... ready issues labeled kind:bug ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | List displayed successfully (even if empty). |

**Notes:**

- Closed issues are hidden by default. Use `--all` to show them alongside open issues, or `--state closed` to show only closed issues.
- `np issue list` is not available; use `np list` exclusively.

---

### claim

Claim an issue by ID or the next ready issue. Claiming is required before updating fields or transitioning state.

When given an issue ID, claims that specific issue. When given `ready` (case-insensitive), claims the next ready issue by priority (P0 first), then by creation date (oldest first).

**Synopsis:**

```
np claim [options] <ISSUE-ID | ready>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | The issue ID to claim, or `ready` to claim the next ready issue. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name for the claim. Required. Env: `NP_AUTHOR`. |
| `--label` | Label filter in `key:value` or `key:*` format. Repeatable, AND semantics. With `ready`: filters which issue gets claimed. With an issue ID: guard-rail assertion (claim fails if unmet). |
| `--role` | Filter by role: `task` or `epic`. With `ready`: filters which issue gets claimed. With an issue ID: guard-rail assertion (claim fails if unmet). |
| `--duration` | Claim duration before the claim becomes stale (e.g., `30m`, `1h`, `4h`). Default: `2h`. Mutually exclusive with `--stale-at`. |
| `--stale-at` | RFC3339 UTC stale time for the claim (e.g., `2026-04-02T14:00:00Z`). Must be in the future and within 24h. Mutually exclusive with `--duration`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

Claim by ID:

```text
$ np claim FOO-a3bxr --author alice
[ok] Claimed FOO-a3bxr
  Claim ID: 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5
  Author: alice
  Stale at: 2026-04-01 11:30:00
```

```json
$ np claim FOO-a3bxr --author alice --json
{
  "issue_id": "FOO-a3bxr",
  "claim_id": "7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5",
  "author": "alice",
  "created_at": "2026-04-01T09:30:00Z",
  "stale_at": "2026-04-01T11:30:00Z"
}
```

Claim the next issue from the ready queue:


```text
$ np claim ready --author alice
[ok] Claimed FOO-a3bxr
  Claim ID: 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5
  Author: alice
  Stale at: 2026-04-01 11:30:00
```

Claim from the ready queue with filters:

```json
$ np claim ready --author alice --role task --label kind:bug --json
{
  "issue_id": "FOO-d4kzs",
  "claim_id": "9c4d7e2f1a6b5c8d3e0f4a7b2c1d6e9f",
  "author": "alice",
  "created_at": "2026-04-01T09:45:00Z",
  "stale_at": "2026-04-01T11:45:00Z"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Claim acquired successfully. |
| 2 | Issue not found (by ID) or no ready issues found (with `ready`). |
| 3 | Claim conflict — the issue is already claimed and the claim is not yet stale. |
| 4 | Guard-rail assertion failed — the issue does not match `--label` or `--role`. |

**Notes:**

- The claim ID is a bearer token. Anyone with the claim ID can use it — there is no per-author verification on subsequent operations. Guard it accordingly.
- Claiming an issue with a stale claim succeeds automatically. No special flag is required.
- Attempting to claim an issue with another active claim returns a claim conflict. Wait for the stale time to pass or claim a different issue from the ready queue.
- Use `--duration` or `--stale-at` to set the stale time.
- See [Terminology](#terminology) and [`ready`](#ready) for the exact readiness rule.

---

### close

Close an issue that you have claimed. The reason is added as a comment.

**Synopsis:**

```
np close [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--reason`, `-r` | Reason for closing. Added as a comment. Required. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np close --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5 --reason "Login endpoint complete with JWT auth."
Closed FOO-a3bxr
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue closed successfully. |
| 3 | Claim conflict. |
| 4 | Validation error. |

---

### ready

List all issues currently ready for work. Shortcut for `np list --ready`.

**Synopsis:**

```
np ready [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--role`, `-r` | Filter by role: `task` or `epic`. Repeatable, AND semantics. |
| `--state`, `-s` | Filter by state: `open`, `closed`, `deferred`. Repeatable. |
| `--parent` | Filter by parent epic ID. Repeatable. |
| `--label` | Label filter in `key:value` format. Repeatable, AND semantics. |
| `--order` | Sort order. One of `ID`, `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `PRIORITY` (default), `ROLE`, `STATE`, `TITLE`, or `MODIFIED`. Append `:asc` or `:desc` to set direction (ascending is the default). |
| `--columns` | Comma-separated list of columns to display. Valid columns: `ID`, `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `PRIORITY`, `ROLE`, `STATE`, `TITLE`. |
| `--limit N`, `-n N` | Maximum number of results (default 20). |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np ready
[ ... issues in the ready queue ... ]
```

```
$ np ready --role task
[ ... ready tasks ... ]
```

```
$ np ready --label kind:bug
[ ... ready issues labeled kind:bug ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Ready issues listed successfully (even if none). |

**Notes:**

- An issue is in the ready queue when it is `open`, has no unresolved `blocked_by` relationships, and no deferred ancestor epic. Epics must also have no children.
- Filter flags combine with AND semantics.
- The default sort order is priority ascending (P0 first), which differs from `np list` whose default is ID ascending.

---

### blocked

List all issues that are blocked by unresolved dependencies.

**Synopsis:**

```
np blocked [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit N`, `-n N` | Maximum number of results (default 20). |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np blocked
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Blocked issues listed successfully (even if none). |

---

## Issues

Commands for managing issues, epics, relationships, labels, comments, and interactive forms.

---

### issue search

Full-text search across issue titles and descriptions.

**Synopsis:**

```
np issue search [options] <QUERY>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `QUERY` | Text to search for across issue titles and descriptions. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--search-comments` | Include comment bodies in the search. |
| `--state`, `-s` | Filter by state. |
| `--role`, `-r` | Filter by role: `task` or `epic`. Repeatable. |
| `--label` | Filter by label in `key:value` format. Repeatable. |
| `--order` | Sort order. One of `PRIORITY` (default), `ID`, `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `ROLE`, `STATE`, `TITLE`, or `MODIFIED`. Append `:asc` or `:desc` to set direction (ascending is the default). |
| `--columns` | Comma-separated list of columns to display. Valid columns: `ID`, `CREATED`, `PARENT_ID`, `PARENT_CREATED`, `PRIORITY`, `ROLE`, `STATE`, `TITLE`. Replaces the previous `--timestamps` flag. |
| `--limit`, `-n` | Maximum number of results. Default: 20. |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np issue search "login timeout"
FOO-a3bxr  P2  task  Add user login endpoint
```

```json
$ np issue search "JWT" --search-comments --json
[ ... matching issues as JSON ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Search completed successfully (even if no results). |

---

### issue release

Release a claimed issue without closing it. The issue returns to the `open` state.

**Synopsis:**

```
np issue release [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue release --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5
Released FOO-a3bxr
```

```
$ np issue release --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5 --json
{
  "action": "release",
  "issue_id": "FOO-a3bxr"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Claim released successfully. |
| 3 | Claim conflict — wrong or expired claim ID. |

**Notes:**

- Use this when you need to stop working on an issue without closing it. Releasing puts it back in the ready queue if it is otherwise ready.

---

### issue reopen

Reopen one or more closed issues, transitioning them back to `open`.

**Synopsis:**

```
np issue reopen [options] <ISSUE-ID> [ISSUE-ID...]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | One or more issue IDs to reopen. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue reopen FOO-a3bxr --author alice
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue(s) reopened successfully. |
| 2 | Issue not found. |
| 4 | Validation error (e.g., issue is not in `closed` state). |

**Notes:**

- Does not require a claim — reopening transitions the issue to `open`, which is unclaimed by definition.
- Accepts multiple issue IDs to batch-reopen in a single command.

---

### issue undefer

Restore one or more deferred issues, transitioning them back to `open`.

**Synopsis:**

```
np issue undefer [options] <ISSUE-ID> [ISSUE-ID...]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | One or more issue IDs to undefer. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue undefer FOO-a3bxr --author alice
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue(s) undeferred successfully. |
| 2 | Issue not found. |
| 4 | Validation error (e.g., issue is not in `deferred` state). |

**Notes:**

- Does not require a claim — undefer transitions the issue to `open`.
- Accepts multiple issue IDs.

---

### issue defer

Defer a claimed issue for later. Deferred issues are excluded from the ready queue, and descendants under a deferred ancestor are excluded too.

**Synopsis:**

```
np issue defer [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue defer --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue deferred successfully. |
| 3 | Claim conflict. |

**Notes:**

- Deferring an epic effectively defers all its descendants. They will not re-enter the ready queue until the epic is undeferred.

---

### issue delete

Soft-delete a claimed issue. Requires the `--confirm` flag as a safety gate.

**Synopsis:**

```
np issue delete [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--confirm` | Confirm the deletion. Required — the command will not execute without this flag. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue delete --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5 --confirm
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue deleted successfully. |
| 3 | Claim conflict. |
| 4 | Validation error (e.g., missing `--confirm`). |

**Notes:**

- Deletion is soft — the issue is marked as deleted but remains in the database until garbage collected with `np admin gc --confirm`.
- Deleting an issue also removes its relationships, labels, and comments.

---

### issue history

Display the full mutation history (audit trail) of an issue, showing every change made since creation.

**Synopsis:**

```
np issue history [options] <ISSUE-ID>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | The issue ID to inspect. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit`, `-n` | Maximum number of entries. Default: 20. |
| `--no-limit` | Return all matching entries. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np issue history FOO-a3bxr
```

```
$ np issue history FOO-a3bxr --json --no-limit
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | History displayed successfully. |
| 2 | Issue not found. |

**Notes:**

- Every mutation (create, update, state transition, claim, release) is recorded as a history entry with a timestamp and author.
- History entries are immutable — they cannot be edited or deleted.

---

### issue orphans

List issues that have no parent epic.

**Synopsis:**

```
np issue orphans [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit`, `-n` | Maximum number of results. Default: 20. |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Orphans listed successfully (even if none). |

**Notes:**

- Useful for finding issues that should be organized under an epic but have not been parented yet.

---

### epic status

Show completion status for open epics. Displays child counts by state and whether the epic is completed.

**Synopsis:**

```
np epic status [options] [EPIC-ID]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `EPIC-ID` | Optional. If provided, show status for only this epic. If omitted, show status for all open epics. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--completed-only` | Show only completed epics (all children closed or complete). |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np epic status
```

```json
$ np epic status FOO-c4npt --json
[ ... epic status as JSON ... ]
```

```text
$ np epic status --completed-only
[ ... completed epics ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Status displayed successfully. |
| 2 | Epic not found (when a specific ID is provided). |

---

### epic close-completed

Close all epics whose children are fully resolved. An epic is completed when all its children are in a terminal state (closed or complete).

**Synopsis:**

```
np epic close-completed [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name for claiming and commenting. Required. Env: `NP_AUTHOR`. |
| `--dry-run` | List completed epics without closing them. |
| `--include-tasks` | Also close parent tasks whose children are all closed. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```json
$ np epic close-completed --author alice --json
{
  "closed": 2,
  "results": [
    { "id": "FOO-c4npt", "title": "Authentication overhaul", "closed": true },
    { "id": "FOO-e8vqm", "title": "Database migration", "closed": true }
  ]
}
```

```text
$ np epic close-completed --dry-run --author alice
[ ... epics that would be closed ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Command completed successfully (even if no completed epics were found). |

**Notes:**

- This command atomically claims, closes (with a comment), and releases each completed epic.
- Use `--dry-run` to preview which epics would be closed without making changes.
- Use `--include-tasks` to also close parent tasks whose children are all in a terminal state.

---

### epic children

List all direct children of an epic.

**Synopsis:**

```
np epic children [options] <EPIC-ID>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `EPIC-ID` | The epic whose children to list. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit`, `-n` | Maximum number of results. Default: 20. |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np epic children FOO-c4npt
```

```json
$ np epic children FOO-c4npt --json
[ ... direct children as JSON ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Children listed successfully (even if none). |
| 2 | Epic not found. |

**Notes:**

- Also available as `np rel parent children`.

---

### rel add

Add a relationship between two issues.

**Synopsis:**

```
np rel add [options] <A> <rel> <B>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `A` | The source issue ID. |
| `rel` | Relationship type: `blocked_by`, `blocks`, `refs`, `parent_of`, `child_of`. |
| `B` | The target issue ID. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--claim` | Claim ID. Required only for `parent_of` and `child_of` (which mutate the child issue's parent field). Env: `NP_CLAIM`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np rel add FOO-a3bxr blocked_by FOO-b7mqd --author alice
```

```
$ np rel add FOO-c4npt parent_of FOO-a3bxr --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5 --author alice
```

```
$ np rel add FOO-a3bxr refs FOO-d4kzs --author alice --json
{
  "action": "added",
  "source": "FOO-a3bxr",
  "target": "FOO-d4kzs",
  "type": "refs"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Relationship added successfully. |
| 2 | One or both issue IDs not found. |
| 3 | Claim conflict (for `parent_of`/`child_of` when claim is wrong or missing). |
| 4 | Validation error (e.g., would create a cycle, or exceeds the 3-layer depth limit). |

**Notes:**

- `blocked_by` and `blocks` are directional inverses: `A blocked_by B` means A cannot progress until B is closed.
- `refs` is bidirectional — `A refs B` and `B refs A` are equivalent.
- `parent_of` and `child_of` require a claim on the child issue because they mutate the child's parent field.

---

### rel list

List all relationships across active (non-closed) issues, organized into three sections: parent-child hierarchy, blocking dependencies, and contextual references. Use `--rel` to restrict output to a single section.

**Synopsis:**

```
np rel list [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--rel string` | Filter to one section. Accepts canonical category names and `np rel add` aliases (see below). |

**`--rel` values:**

| Value | Section shown | Aliases accepted |
|-------|---------------|-----------------|
| `blocking` | Blocking dependency chains | `blocked_by`, `blocks` |
| `refs` | Contextual reference clusters | _(none)_ |
| `parent-child` | Parent-child hierarchy tree | `parent_of`, `child_of` |

**Section formats:**

- **Parent-child** — hierarchical tree rooted at top-level issues. Header reports root count and total issue count.
- **Blocking** — dependency chain forest, roots-down. Header reports chain count, edge count, and cycle count.
- **Refs** — undirected connected components, largest first. Header reports component count and total edge count. Each component lists edges as `LEFT-ID  Title  —  RIGHT-ID  Title`; the two endpoints within an edge are sorted lexicographically by ID for deterministic output.

**Examples:**

```
$ np rel list
Parent-child (1 roots, 3 issues)

TREE       P   ROLE  STATE           TITLE
FOO-abc12  P1  epic  open (active)   Build authentication system
  FOO-def34  P2  task  open (claimed)  Implement login endpoint

Blocking (1 chains, 1 edges, 0 cycles)

TREE       P   ROLE  STATE           TITLE
FOO-def34  P2  task  open (claimed)  Implement login endpoint
  FOO-ghi56  P3  task  open (blocked)  Write integration tests

Refs (1 components, 2 edges)

Component 1 (3 issues, 2 edges)
  FOO-abc12  Build authentication system  —  FOO-jkl78  Security audit notes
  FOO-def34  Implement login endpoint  —  FOO-jkl78  Security audit notes
```

```
$ np rel list --rel=blocking
Blocking (1 chains, 1 edges, 0 cycles)

TREE       P   ROLE  STATE           TITLE
FOO-def34  P2  task  open (claimed)  Implement login endpoint
  FOO-ghi56  P3  task  open (blocked)  Write integration tests
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | List displayed successfully. |
| 4 | Invalid `--rel` value. |

---

### rel remove

Remove a relationship between two issues. The argument syntax mirrors `rel add` exactly: the same relationship types are accepted in the same positional order.

**Synopsis:**

```
np rel remove [options] <A> <rel> <B>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `A` | The source issue ID. |
| `rel` | Relationship type: `blocked_by`, `blocks`, `refs`, `parent_of`, `child_of`. |
| `B` | The target issue ID. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np rel remove FOO-a3bxr blocked_by FOO-b7mqd --author alice
```

```
$ np rel remove FOO-a3bxr refs FOO-d4kzs --author alice --json
{
  "action": "removed",
  "source": "FOO-a3bxr",
  "target": "FOO-d4kzs",
  "type": "refs"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Relationship removed successfully. |
| 2 | Relationship not found. |

**Notes:**

- The `<rel>` token must match the direction used when the relationship was created. `np rel add A blocked_by B` is removed by `np rel remove A blocked_by B`; passing `blocks` instead will silently no-op.
- `refs` is symmetric — either `np rel remove A refs B` or `np rel remove B refs A` removes the edge.
- No claim is required for any relationship type.

---

### rel parent children

List direct children of an issue.

**Synopsis:**

```
np rel parent children [options] <ISSUE-ID>
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit N`, `-n N` | Maximum number of results (default 20). |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Children listed successfully. |

---

### rel parent tree

Show the full descendant hierarchy of an issue as an indented tree.

**Synopsis:**

```
np rel parent tree [options] <ISSUE-ID>
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np rel parent tree FOO-c4npt
FOO-c4npt  Authentication overhaul
  FOO-a3bxr  Add user login endpoint
  FOO-b7mqd  Write tests
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Tree displayed successfully. |
| 2 | Issue not found. |

---

### rel parent detach

Remove the parent-child relationship between two issues. Order-independent — specify parent and child in either order.

**Synopsis:**

```
np rel parent detach [options] <A> <B>
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Parent-child relationship removed successfully. |
| 2 | Relationship not found. |

**Notes:**

- Does not require a claim, unlike adding a parent-child relationship.

---

### rel issue

List all relationships for an issue — blocking, references, and parent-child.

**Synopsis:**

```
np rel issue [options] <ISSUE-ID>
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Relationships listed successfully. |

---

### rel tree

Show the ancestry and descendant hierarchy for an issue.

Renders a columnar table that begins at the root ancestor of the specified issue, traces the ancestry path down to the specified issue, fully expands the issue's descendant subtree, and inserts sibling summary rows (`and N siblings`) at each ancestor tier for branches not on the ancestry path. Use `--full` to expand the entire tree from the root ancestor with no sibling summaries.

The specified issue's row is bold on TTY output. Priority, role, and state columns use the same coloration as `np list`.

**Synopsis:**

```
np rel tree [options] <ISSUE-ID>
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--full` | Expand the entire tree from the root ancestor with no sibling summaries. |
| `--json` | Output machine-readable JSON. |

**Examples:**

Given epic `FOO-c4npt` with children `FOO-d6rsk` (an epic with two tasks) and `FOO-g3wbn` (a task):

```
$ np rel tree FOO-c4npt
TREE             P   ROLE  STATE          TITLE
FOO-c4npt        P2  epic  open (active)  Root epic
  FOO-d6rsk      P2  epic  open (active)  Child epic
    FOO-e2mvh    P2  task  open (ready)   Task A
    FOO-f7qxl    P2  task  open (ready)   Task B
  FOO-g3wbn      P2  task  open (ready)   Standalone child
```

Running on a child shows the ancestry path plus sibling summaries:

```
$ np rel tree FOO-d6rsk
TREE             P   ROLE  STATE          TITLE
FOO-c4npt        P2  epic  open (active)  Root epic
  FOO-d6rsk      P2  epic  open (active)  Child epic
    FOO-e2mvh    P2  task  open (ready)   Task A
    FOO-f7qxl    P2  task  open (ready)   Task B
  and 1 sibling
```

Use `--full` to see the complete tree regardless of which issue you specify:

```
$ np rel tree FOO-d6rsk --full
TREE             P   ROLE  STATE          TITLE
FOO-c4npt        P2  epic  open (active)  Root epic
  FOO-d6rsk      P2  epic  open (active)  Child epic
    FOO-e2mvh    P2  task  open (ready)   Task A
    FOO-f7qxl    P2  task  open (ready)   Task B
  FOO-g3wbn      P2  task  open (ready)   Standalone child
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Tree displayed successfully. |
| 2 | Issue not found. |

---

### rel graph

Render a graph of issues and relationships in various output formats.

**Synopsis:**

```
np rel graph [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--format`, `-f` | Output format: `dot`, `json`, or `text`. Required. |
| `--output`, `-o` | Write output to a file instead of stdout. |
| `--include-closed` | Include closed issues in the graph (hidden by default). |
| `--json` | Alias for `--format=json`. |

**Examples:**

```
$ np rel graph --format dot
```

```
$ np rel graph --format dot -o issues.dot
$ dot -Tpng issues.dot -o issues.png
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Graph generated successfully. |

**Notes:**

- Requires Graphviz (`dot` command) to render DOT output into an image.
- Closed issues are excluded by default to keep the graph readable.

---

### label add

Set a label on a claimed issue. If the key already exists, its value is overwritten.

**Synopsis:**

```
np label add [options] <key:value>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key:value` | The label to set, in `key:value` format. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np label add kind:bug --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Label set successfully. |
| 3 | Claim conflict. |
| 4 | Validation error (e.g., missing `key:value` format). |

**Notes:**

- Common label conventions: `kind:` (bug, feature, docs), `area:` (auth, api, cli), `scope:` (component names).

---

### label remove

Remove a label from a claimed issue by key.

**Synopsis:**

```
np label remove [options] <key>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | The label key to remove. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Label removed successfully. |
| 3 | Claim conflict. |

---

### label list

List all labels for a specific issue.

**Synopsis:**

```
np label list <ISSUE-ID> [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Labels listed successfully (even if none). |

---

### label list-all

List all label keys in use across the database, together with each key's three
most popular values. Closed and deferred issues are included so that the
popularity signal reflects historical usage, not just open work.

**Breaking change (pre-1.0):** The `--json` output shape changed in the release
that introduced popularity aggregation. Each element in the `labels` array now
has a `popular_values` array (up to three strings, ordered by descending usage
count with an alphabetical tiebreaker) instead of a single `value` string. The
envelope `count` now reflects the number of distinct keys, not the number of
distinct key-value pairs.

**Synopsis:**

```
np label list-all [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**JSON output shape:**

```json
{
  "count": 2,
  "labels": [
    { "key": "area",  "popular_values": ["cli", "domain"] },
    { "key": "kind",  "popular_values": ["bug", "feature", "refactor"] }
  ]
}
```

`popular_values` is always a non-null array containing 1–3 entries ordered by
descending usage count with an alphabetical tiebreaker on the value string.
`count` reflects the number of distinct keys returned.

**Human-readable output:**

The text output uses a two-column table with headers (`KEY` and `POPULAR VALUES`).
Popular values are comma-joined in the second column. Column alignment is
ANSI-color-safe.

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Labels listed successfully. |

**Notes:**

- Useful for discovering which label keys and values are in use across the project.
- All non-deleted issues (open, closed, and deferred) contribute to popularity counts.
- With more than three distinct values for a key, only the three most frequently
  used values are shown.

---

### label propagate

Propagate a label from a parent issue to all its descendants. Each descendant is atomically claimed, labeled, and released.

**Synopsis:**

```
np label propagate [options] <key>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `key` | The label key to propagate. The value is copied from the parent issue. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name (for claiming descendants). Required. Env: `NP_AUTHOR`. |
| `--issue`, `-i` | Parent issue ID. Required. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np label propagate kind --issue FOO-c4npt --author alice
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Label propagated successfully. |
| 2 | Issue not found. |
| 3 | Claim conflict on one of the descendants. |

**Notes:**

- Only propagates the specified key — other labels on the parent are not copied.
- Skips descendants that already have the same key-value pair.

---

### comment list

List comments for an issue, ordered by creation time.

**Synopsis:**

```
np comment list <ISSUE-ID> [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--limit`, `-n` | Maximum number of results. Default: 20. |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np comment list FOO-a3bxr
```

```json
$ np comment list FOO-a3bxr --json
[ ... comments as JSON ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Comments listed successfully (even if none). |
| 2 | Issue not found. |

---

### comment search

Search comments by text across all issues, with optional scoping filters.

**Synopsis:**

```
np comment search [options] <QUERY>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `QUERY` | Text to search for across comment bodies. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--issue`, `-i` | Scope to comments on a specific issue. Repeatable. |
| `--author`, `-a` | Filter by comment author. Repeatable. |
| `--label` | Scope to comments on issues with a label (key:value or key:*). Repeatable. |
| `--parent` | Scope to comments on an issue and its direct children. Repeatable. |
| `--tree` | Scope to comments on all issues in a tree. Repeatable. |
| `--follow-refs` | Expand scope to include referenced issues. |
| `--limit N`, `-n N` | Maximum number of results (default 20). |
| `--no-limit` | Return all matching results. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np comment search "root cause"
```

```json
$ np comment search "auth" --issue FOO-a3bxr --json
[ ... matching comments as JSON ... ]
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Search completed successfully (even if no results). |

---

### form create

Interactively create an issue using a terminal form. Collects all fields through an interactive TUI.

**Synopsis:**

```
np form create
```

**Flags:**

None.

**Notes:**

- Requires a terminal (TTY). Not available in pipe mode.
- Also accessible via `np create` when stdin is a terminal.

---

### form update

Interactively update a claimed issue using a terminal form.

**Synopsis:**

```
np form update [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |

**Notes:**

- Requires a terminal (TTY).

---

### form comment

Interactively compose a comment on an issue using a terminal form.

**Synopsis:**

```
np form comment <ISSUE-ID>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | The issue ID to comment on. |

**Flags:**

None.

**Notes:**

- Requires a terminal (TTY).

---

## Agent Toolkit

Structured JSON input/output commands for AI agents, plus agent utilities.

---

### json create

Create an issue from a JSON object on stdin. All `json` subcommands output JSON unconditionally — there is no `--json` flag.

**Synopsis:**

```
np json create [options] <<'JSONEND'
{
  "role": "task",
  "title": "Fix auth bug"
}
JSONEND
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |
| `--with-claim` | Immediately claim the new issue and include `claim_id` in the output. |

**JSON fields:**

| Field | Description |
|-------|-------------|
| `role` | Issue role: `task` or `epic`. Defaults to `task` when omitted. |
| `title` | Issue title. Required. |
| `description` | Issue description. |
| `acceptance_criteria` | Acceptance criteria. |
| `priority` | Priority level: `P0`–`P4`. |
| `parent` | Parent epic issue ID. |
| `labels` | Array of `key:value` strings. |
| `label_remove` | Accepted for schema compatibility with `json update`, but ignored on create. |
| `comment` | Optional comment to add immediately after issue creation. |
| `claim` | Accepted in JSON for schema compatibility, but ignored. Use `--with-claim` on the command line instead. |

**Examples:**

```json
$ np json create --author alice <<'JSONEND'
{
  "role": "task",
  "title": "Fix auth bug",
  "priority": "P1"
}
JSONEND
{
  "id": "FOO-a3bxr",
  "role": "task",
  "title": "Fix auth bug",
  "priority": "P1",
  "state": "open",
  "created_at": "2026-04-01T12:00:00.000Z"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Issue created successfully. |
| 4 | Validation error. |
| 5 | Database error. |

---

### json update

Update fields on a claimed issue. Reads a JSON object from stdin. Missing fields mean "no change"; null fields mean "unset/clear".

**Synopsis:**

```
np json update [options] <<'JSONEND'
{
  "title": "Revised title"
}
JSONEND
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--claim` | Active claim ID. Required. Env: `NP_CLAIM`. |

**JSON fields:**

| Field | Description |
|-------|-------------|
| `title` | New title. |
| `description` | New description. |
| `acceptance_criteria` | New acceptance criteria. |
| `priority` | New priority: `P0`–`P4`. |
| `parent` | New parent epic ID. |
| `labels` | Array of `key:value` strings to set or replace. |
| `label_remove` | Array of key strings to remove. |
| `comment` | Add a comment as part of the update. |
| `role` | Accepted for schema compatibility with `json create`; if present, it must match the issue's current role. |
| `claim` | Accepted for schema compatibility, but ignored. |

**Examples:**

```json
$ np json update --claim 7f2e9a41c3d8b6e5a4f1c0d9b8a7e6f5 <<'JSONEND'
{
  "title": "Revised title",
  "priority": "P0"
}
JSONEND
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Update applied successfully. |
| 3 | Claim conflict. |
| 4 | Validation error. |

**Notes:**

- `json update` follows PATCH semantics: omitted fields mean no change, and explicit `null` clears supported scalar fields.

---

### json comment

Add a comment to an issue from a JSON object on stdin.

**Synopsis:**

```
np json comment [options] <ISSUE-ID> <<'JSONEND'
{
  "body": "Found the root cause"
}
JSONEND
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `ISSUE-ID` | The issue ID to comment on. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Author name. Required. Env: `NP_AUTHOR`. |

**JSON fields:**

| Field | Description |
|-------|-------------|
| `body` | Comment body text. Required. |

**Examples:**

```json
$ np json comment FOO-a3bxr --author alice <<'JSONEND'
{
  "body": "Found the root cause in auth.go:142"
}
JSONEND
{
  "comment_id": "comment-1",
  "issue_id": "FOO-a3bxr",
  "author": "alice"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Comment added successfully. |
| 2 | Issue not found. |
| 4 | Validation error (e.g., empty body). |

**Notes:**

- Comments are also added automatically by `np close` (using the `--reason` text) and by `np epic close-completed`.

---

### agent name

Generate a random agent name. Agents should generate their own name at the start of each session and reuse it for all `--author` flags.

**Synopsis:**

```
np agent name [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |
| `--seed VALUE` | Derive the name deterministically from `VALUE`; the same seed always yields the same name. |

**Examples:**

```json
$ np agent name --json
{
  "name": "kind-comet-quest"
}
```

```text
$ np agent name
blue-seal-echo
```

Agents should use `--seed=$PPID` to produce a stable identity tied to their process ID. The same seed always yields the same name:

```text
$ np agent name --seed=$PPID
calm-spruce-spray

$ np agent name --seed=$PPID
calm-spruce-spray
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Name generated successfully. |

**Notes:**

- Without `--seed`, each call produces a fresh random name using an adjective-noun-verb pattern. Names are not guaranteed to be unique, but collisions are rare.
- With `--seed=<value>`, the name is derived deterministically from the seed string — the same seed always yields the same generated name. Providing an empty seed is an error.
- Agents should use `--seed=$PPID` so that resuming a session with the same process ID produces the same author identity.
- Humans can use any stable identifier; this command is primarily for agents.

---

### agent prime

Print agent workflow instructions in Markdown. This output is designed to be provided to an AI agent at the start of a session so the agent knows how to use `np`.

**Synopsis:**

```
np agent prime [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np agent prime
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Instructions printed successfully. |

**Notes:**

- The output is too large for static instruction files like CLAUDE.md. Provide it dynamically at session start.
- Re-provide the output whenever context is compacted or cleared.

---

## Admin

Administrative, maintenance, and diagnostic commands for managing the `np` database.

---

### admin backup

Create a JSONL backup of the issue database.

**Synopsis:**

```
np admin backup [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--output`, `-o` | Destination file or directory for the backup. If a directory, the default filename is used inside it. Default: `.np/backup-<prefix-lowercase>.<timestamp>.jsonl.gz`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin backup
```

```
$ np admin backup -o my-backup.jsonl.gz
```

```
$ np admin backup -o /tmp/backups/
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Backup created successfully. |

---

### admin completion

Output a shell completion script for the specified shell. Source the output in your shell configuration to enable tab completion for all `np` commands and flags.

**Synopsis:**

```
np admin completion <SHELL>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `SHELL` | Target shell. One of: `bash`, `zsh`, `fish`. |

**Examples:**

```
$ np admin completion bash >> ~/.bashrc
$ np admin completion zsh >> ~/.zshrc
$ np admin completion fish > ~/.config/fish/completions/np.fish
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Completion script output successfully. |
| 1 | General error (e.g., unrecognized shell name). |

**Notes:**

- The completion script is printed to stdout. Redirect it to the appropriate shell configuration file.
- After adding the completion script, restart your shell or source the file for it to take effect.

---

### admin doctor

Run diagnostics on the database. Detects stale claims, analyzes why no issues are ready, and suggests unblocking actions.

**Synopsis:**

```
np admin doctor [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--verbose`, `-v` | Show per-check pass/fail status for every diagnostic. |
| `--severity` | Minimum severity threshold: `error`, `warning`, `info`. Default: `info`. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin doctor
```

```
$ np admin doctor --verbose
```

```
$ np admin doctor --severity warning
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Diagnostics completed successfully. |

**Notes:**

- Checks include stale claims, issues with no ready path, cycles, orphaned issues, and more.
- Use `--severity error` to skip informational and warning checks — useful for CI integration.

---

### admin gc

Garbage-collect deleted issues (and optionally closed issues) from the database.

**Synopsis:**

```
np admin gc [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--confirm` | Confirm the garbage collection. Required. |
| `--include-closed`, `--aggressive` | Also remove closed issues, not just soft-deleted ones. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin gc --confirm
```

```
$ np admin gc --confirm --include-closed
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Garbage collection completed successfully. |
| 4 | Validation error (e.g., missing `--confirm`). |

**Notes:**

- Without `--include-closed`, only soft-deleted issues (those deleted with `np issue delete`) are removed.
- With `--include-closed`, closed issues are also permanently removed — use this to reclaim space after a project milestone.

---

### admin reset

Reset the database. Uses a two-step key verification process for safety.

**Synopsis:**

```
np admin reset [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--reset-key` | Reset key from step 1. When provided, executes the reset. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin reset
# Outputs a reset key
$ np admin reset --reset-key <KEY>
# Executes the reset
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Reset completed (or key generated) successfully. |

**Notes:**

- Step 1: Run `np admin reset` without `--reset-key` to receive a one-time reset key.
- Step 2: Run `np admin reset --reset-key <KEY>` to execute the destructive reset.
- There is no undo. Consider creating a backup with `np admin backup` before running this command.

---

### admin restore

Restore the database from a JSONL backup file. This is a destructive operation — the current database is replaced.

**Synopsis:**

```
np admin restore [options] <backup-file>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `backup-file` | Path to the JSONL backup file to restore from. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```text
$ np admin restore .np/backup-foo.20260401T120000Z.jsonl.gz
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Restore completed successfully. |

**Notes:**

- This replaces the current database entirely. The previous database is lost.
- The command requires interactive confirmation by typing a specific phrase, which intentionally blocks unattended restore operations.
- Back up the current database first if you need to preserve it.

---

### admin tally

Show a summary of the issue database: open, claimed, deferred, closed, ready, blocked, and total counts.

**Synopsis:**

```
np admin tally [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin tally
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Tally displayed successfully. |

---

### admin upgrade

Check for and apply database schema upgrades.

**Synopsis:**

```
np admin upgrade [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Check completed (or upgrade applied) successfully. |

**Notes:**

- Currently a placeholder for future use. Schema migrations are not part of the `np` codebase — see CLAUDE.md for migration policy.

---

### admin where

Print the absolute path to the `.np/` directory that `np` discovered for the current working directory.

**Synopsis:**

```
np admin where [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np admin where
/Users/you/projects/my-app/.np
```

```
$ cd /Users/you/projects/my-app/src/lib
$ np admin where
/Users/you/projects/my-app/.np
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Path printed successfully. |
| 1 | No `.np/` directory found in the current directory or any parent. |

**Notes:**

- `np` walks up from the current working directory looking for a `.np/` directory. This means you can run `np admin where` (or any `np` command) from any subdirectory of your workspace.
- Useful for verifying which database `np` will use, especially when multiple workspaces share a parent directory.

---

### import jsonl

Import issues from a JSONL file. Each line in the file is a JSON object describing one issue. The command validates all lines before creating any issues — validation errors are reported without modifying the database.

**Synopsis:**

```
np import jsonl [options] <file>
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `file` | Path to the JSONL file to import. |

**Flags:**

| Flag | Description |
|------|-------------|
| `--author`, `-a` | Default author for imported issues. Required. Env: `NP_AUTHOR`. |
| `--force-author` | Override per-line `author` fields with the `--author` value. |
| `--json` | Output machine-readable JSON. |

**Examples:**

```
$ np import jsonl backlog.jsonl --author alice
[ok] Imported 5 issues from backlog.jsonl
```

```
$ np import jsonl migration.jsonl --author alice --force-author --json
{
  "action": "imported",
  "source": "migration.jsonl",
  "created": 3,
  "skipped": 0,
  "failed": 0
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Import completed successfully (or validation errors reported in JSON mode). |
| 1 | Validation failed (text mode). |

**Notes:**

- The JSONL format is documented in `docs/developer/reference/jsonl-import-format.md`.
- Import is idempotent: each line carries a required `idempotency_label` (a `key:value` string), and re-importing a file skips lines whose `idempotency_label` is already present as a label on a non-deleted issue.
- Validation runs before any database mutations. If any line fails validation, no issues are created.
- Per-line `author` fields override the `--author` default unless `--force-author` is set.
- Issues can be imported in `open`, `deferred`, or `closed` state. The `claimed` and `blocked` states are not valid for import.
- References between issues (parent, blocked_by, blocks, refs) can use intra-file `idempotency_label` values or existing np issue IDs.

---

## Info

Informational commands.

---

### version

Print the application version, platform, and optionally VCS build metadata.

**Synopsis:**

```
np version [options]
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--json` | Output machine-readable JSON with all fields. |
| `--verbose` | Include VCS build metadata (commit hash and build timestamp). |

**Examples:**

```
$ np version
np version 1.0.0 darwin/arm64
```

```
$ np version --verbose
np version 1.0.0 darwin/arm64
commit: b2d4d375ce13
built:  2026-04-01T16:35:38Z
```

```
$ np version --json
{
  "name": "np",
  "version": "1.0.0",
  "os": "darwin",
  "arch": "arm64",
  "commit": "b2d4d375ce13",
  "dirty": false,
  "built": "2026-04-01T16:35:38.000Z"
}
```

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Version printed successfully. |

**Notes:**

- The version shows `dev` when built without an explicit version string. Pass `VERSION=x.y.z` to `make build` to bake in a release version.
- The `--json` output always includes all fields (commit, dirty, built) regardless of whether `--verbose` is passed.
- The `dirty` field in JSON output indicates whether the binary was built from a working directory with uncommitted changes.
