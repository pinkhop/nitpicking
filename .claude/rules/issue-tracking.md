This workspace uses the nitpicking (`np`) command line tool for issue tracking. Nitpicking is local-only — no network, no remote sync, no background daemons. It manages issues in a manner safe for parallel access.

`np` is the **exclusive** tool for task management in this workspace. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

## Choosing an Author Name

Every mutation requires an `--author` flag identifying who is acting. If you do not already have a name, generate one with `np agent name`. Pick a stable name and reuse it for your entire session.

## Issue Types

| Role | Purpose | How to work on it |
|------|---------|-------------------|
| **Task** | Leaf-node work item | Implement what it describes, then close it |
| **Epic** | Organizes children; completion via `completed` secondary state | Decompose into child tasks (and sub-epics if large), then release it |

An epic is complete when all its children are closed or complete. You never close an epic directly.

## Core Workflow

### 1. Find work

```
np claim ready --author <your-name>   # claim the highest-priority ready issue
np list --ready                       # browse all ready issues without claiming
```

Filter which issue gets claimed with `--with-label` and `--with-role`:

```
np claim ready --with-label kind:bug --author <your-name>         # only claim bugs
np claim ready --with-role task --author <your-name>              # only claim tasks
np claim ready --with-label kind:bug --with-role task --author <your-name>  # combine filters
```

`--with-label` uses `key:value` or `key:*` format (repeatable, AND semantics). `--with-role` accepts `task` or `epic`.

Control claim staleness timing with `--duration` or `--stale-at`:

```
np claim ready --duration 4h --author <your-name>                 # claim expires in 4 hours
np claim ready --stale-at 2026-04-02T18:00:00Z --author <your-name>  # claim expires at specific time
```

`--duration` sets how long until the claim goes stale (default 2h, max 24h). `--stale-at` sets an absolute RFC3339 UTC timestamp (must be in the future, max 24h from now). They are mutually exclusive.

### 2. Work on the issue

**If you claimed a task:** implement it. Use the claim ID for all updates via `np json update`:

```
np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title"
}
JSONEND
```

**If you claimed an epic:** plan and decompose it into child tasks using `np json create`:

```
np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Subtask A",
  "parent": "<EPIC-ID>"
}
JSONEND

np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Subtask B",
  "parent": "<EPIC-ID>"
}
JSONEND
```

Use the `parent` field to attach children to the epic. For large epics, decompose into sub-epics and leave further planning to a future implementor. Add `blocked_by` relationships between children to indicate required ordering.

To create a deferred issue (so it does not appear as ready work until explicitly undeferred), use a three-step workflow:

1. Create the issue and immediately claim it using `--with-claim`
2. Defer it: `np issue defer --claim <CLAIM-ID>`
3. Release it: `np issue release --claim <CLAIM-ID>`

```
np json create --author <your-name> --with-claim <<'JSONEND'
{
  "title": "Step 2",
  "parent": "<EPIC-ID>"
}
JSONEND
np issue defer --claim <CLAIM-ID>
np issue release --claim <CLAIM-ID>
```

### 3. Document your work with comments

**Before transitioning state, add a comment to the issue.** Comments record context that the code and commit history cannot capture — your reasoning, trade-offs considered, dead ends explored, or anything a future reader would find useful.

```
np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Approach taken: ..."
}
JSONEND
```

Comments do not require claiming and can be added to any issue, including closed ones.

### 4. Transition state when done

**You MUST transition state when you are done.** Abandoned claims block other agents.

| Transition | When to use |
|------------|-------------|
| `np close --claim <CID> --reason "..."` | Task is complete (can be reopened if needed) |
| `np issue release --claim <CID>` | Epic has been decomposed; or task cannot be completed now — deletes the local claim record without changing the issue's primary state |
| `np issue defer --claim <CID>` | Shelve for later (can be restored with undefer) |

## Handling Incidentals

If you discover something unrelated to your current issue (e.g., a failing test, a bug, a missing feature):

1. Search for an existing issue: `np issue search "description"`
2. If none found, create a new issue:

```
np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "..."
}
JSONEND
```
3. If the incidental blocks your current work, add a relationship:
   `np rel add <YOUR-ISSUE> blocked_by <BLOCKER-ID> --author <your-name>`

## Stale Claims

If no ready issues exist and there are stale claims, stale claims are automatically
overwritten when you run the normal claim command. Run `np admin doctor` to identify
stale claims blocking ready work, then claim normally:

```
np claim ready --author <your-name>
```

## Backups

Run `np admin backup` before any destructive operation (resets, restores, schema experiments). The backup is a gzip-compressed JSONL file written to `.np/` by default. Use `--output` to specify a file or directory.

## Diagnostics

```
np admin backup    # create a backup in .np/ (default filename includes the database prefix)
np admin doctor    # detect stale claims, no-ready-issues analysis, suggest unblock actions
np show <ID>       # full issue detail including readiness and relationships
np issue history <ID> # audit trail of all changes
```

## JSON Agent API

The `np json` command tree provides structured JSON input/output for all mutation operations. These commands read a JSON object from stdin and write JSON to stdout. Identity and context flags remain on the command line; the JSON object provides content fields only. All `json` subcommands output JSON unconditionally — there is no `--json` flag.

### json create

Create an issue from a JSON object on stdin. The `role` field defaults to `task` when omitted.

```
np json create --author <your-name> <<'JSONEND'
{
  "title": "Fix auth bug",
  "priority": "P1"
}
JSONEND
```

**CLI flags:** `--author` (required), `--with-claim` (optional, immediately claims the new issue).
**JSON fields:** `title` (required), `role` (defaults to `task`), `description`, `acceptance_criteria`, `priority`, `parent`, `labels` (array of `key:value` strings), `comment` (creates a comment on the new issue). Unknown fields are rejected.

### json update

Update fields on a claimed issue. Missing fields mean "no change"; null fields mean "unset/clear".

```
np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title",
  "priority": "P0"
}
JSONEND
```

**CLI flags:** `--claim` (required).
**JSON fields:** `title`, `description`, `acceptance_criteria`, `priority`, `parent`, `labels` (array of `key:value` strings), `label_remove` (array of key strings), `comment`, `role` (errors if different from current role). All fields are optional. Unknown fields are rejected.

### json comment

Add a comment to an issue from a JSON object on stdin.

```
np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Found the root cause in auth.go"
}
JSONEND
```

**Positional args:** `<ISSUE-ID>` (required).
**CLI flags:** `--author` (required).
**JSON fields:** `body` (required).

## Key Rules

- **Use `np claim ready` to find work.** Do not browse and cherry-pick issues.
- **Document your work.** Add a comment before transitioning state — capture reasoning, trade-offs, and findings.
- **Always transition state when done.** Close, release, or defer — never abandon a claim.
- **Closed issues can be reopened.** Use `np issue reopen <ID> --author <name>` to restore them.
- **Epics are never closed directly.** They complete when all children resolve.
- **Use `np` exclusively.** Do not track work outside of `np`.

