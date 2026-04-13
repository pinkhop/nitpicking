# Troubleshooting

Common problems organized by symptom, with diagnostics and recovery steps.

---

## Quick Reference: Exit Codes

| Code | Meaning | Common causes |
|------|---------|---------------|
| 0 | Success | — |
| 1 | General error | Unexpected failure, missing database |
| 2 | Not found | Issue ID does not exist, no ready issues |
| 3 | Claim conflict | Issue already claimed by someone else, wrong claim ID |
| 4 | Validation error | Missing required fields, invalid state transition, bad input |
| 5 | Database error | Corruption, concurrent access, init on existing database |

---

## Database Not Found

**Symptom:** Commands fail with a general error about no `.np/` directory.

**Cause:** `np` discovers the database by walking up from your current working directory. If no `.np/` directory exists in the current directory or any parent, commands fail.

**Diagnosis:**

```
$ np admin where
```

If this prints a path, the database is found. If it fails, no `.np/` directory exists above your working directory.

**Resolution:**

- Verify you are in the right directory — `np admin where` works from any subdirectory of the workspace.
- If the database has not been created yet, run `np init <PREFIX>` in the workspace root.
- If you accidentally deleted `.np/`, there is no recovery — the database must be re-created with `np init`.

---

## Claim Conflict (Exit Code 3)

**Symptom:** `np claim <ID>`, `np json update`, `np form update`, or `np close` returns exit code 3.

**Causes:**

- **Already claimed by someone else** — another agent or developer holds the claim.
- **Wrong claim ID** — the claim ID you passed does not match the active claim on the issue.
- **Claim expired but not yet reclaimed** — rare; the claim was stale but the system state has not updated.

**Diagnosis:**

```
$ np show <ISSUE-ID> --json | jq '.state, .claim_author, .claim_stale_at'
```

This shows whether the issue is claimed, who holds the claim, and when the claim becomes stale.

**Resolution:**

- **Wait** — the claim expires after the stale duration (default 2 hours). Check `claim_stale_at` for the exact time. Once stale, claim the issue normally — no special flag is required.
- **Work on something else** — use `np claim ready` to find another issue while you wait.

---

## No Ready Issues

**Symptom:** `np ready` shows nothing; `np claim ready` returns exit code 2.

**Causes:**

- All issues are blocked by unresolved dependencies.
- All issues are deferred.
- All issues are already claimed by other agents.
- An ancestor epic is deferred, making its descendants not ready.
- Open epics with no children are ready for decomposition, so if they are missing from the ready pool they are likely blocked, deferred, already claimed, or filtered out some other way.

**Diagnosis:**

```
$ np admin doctor
```

The doctor diagnostic analyzes why no issues are ready and suggests unblocking actions. For more detail:

```
$ np admin doctor --verbose
```

Also check which issues are blocked:

```
$ np blocked
```

**Resolution:**

- **Unblock** — close the blocking issues, or remove the blocking relationship:
  ```
  $ np rel blocks unblock <A> <B> --author <name>
  ```
- **Undefer** — restore deferred issues:
  ```
  $ np issue undefer <ISSUE-ID> --author <name>
  ```
- **Claim normally** — stale claims are automatically overwritten. Run `np claim ready` to pick up a stale-claimed issue once its claim has expired.

---

## Stale Claims

**Symptom:** An issue is claimed but the agent that claimed it is no longer running.

**Cause:** The agent crashed, was terminated, or lost its context without releasing the claim.

**Diagnosis:**

```
$ np admin doctor
```

The doctor identifies stale claims and reports them. You can also check directly:

```
$ np show <ISSUE-ID> --json | jq '.claim_stale_at'
```

If the `claim_stale_at` time is in the past, the claim is stale and treated as nonexistent.

**Resolution:**

Once the claim is stale, claim the issue normally — stale claims are automatically overwritten:

```
$ np claim <ISSUE-ID> --author <name>
```

Or let `np claim ready` pick it up automatically when it becomes the highest-priority ready issue.

To prevent stale claims in the future, set a longer duration when claiming long-running work:

```
$ np claim <ISSUE-ID> --author <name> --duration 4h
```

---

## Validation Errors (Exit Code 4)

**Symptom:** A command returns exit code 4.

**Common causes:**

- **Missing required fields** — `--author`, `--reason`, `--claim`, or `--confirm` not provided.
- **Invalid state transition** — e.g., trying to close an issue that is not claimed, or trying to reopen an issue that is not closed.
- **Invalid role or priority** — role must be `task` or `epic`; priority must be `P0`–`P4`.
- **Depth limit exceeded** — trying to nest issues more than 3 levels deep (epic → epic → task).
- **Would create a cycle** — a relationship that would introduce a circular dependency.

**Resolution:**

Check the error message — it usually describes the specific validation failure. Review the [Command Reference](command-reference.md) for required flags and valid inputs.

---

## Database Errors (Exit Code 5)

**Symptom:** A command returns exit code 5.

**Common causes:**

- **Initializing an already-initialized workspace** — running `np init` when `.np/` already exists.
- **Concurrent write conflicts** — multiple processes writing to the database simultaneously. SQLite handles this with WAL mode, but extreme concurrency can cause issues.
- **Corrupted database** — rare; can occur if the process is killed mid-write.

**Resolution:**

- For double-init, simply do not run `np init` again — the database already exists.
- For corruption, there is no built-in repair. If you have a backup of `.np/`, restore it. Otherwise, use the two-step `np admin reset` flow to wipe the database and start fresh.

---

## Using np admin doctor

The `doctor` command runs a suite of diagnostics and reports findings:

```
$ np admin doctor --verbose
```

**What it checks:**

- **No ready issues** — analyzes why no issues are ready and which blockers or deferred states are responsible.
- **Orphaned issues** — issues with no parent epic that might need to be organized.
- **Cycle detection** — circular dependencies in blocking or parent-child relationships.
- **Schema version** — whether the database needs migration (v1 databases require `np admin upgrade` before other commands will run).

**Severity filtering:**

```
$ np admin doctor --severity warning   # skip informational checks
$ np admin doctor --severity error     # only critical issues
```

---

## Using np rel graph

Visualize issue relationships to spot structural problems:

```
$ np rel graph --format dot -o issues.dot
$ dot -Tpng issues.dot -o issues.png
```

The graph shows:

- Parent-child relationships (solid lines).
- Blocking relationships (dashed lines).
- Reference relationships (dotted lines).
- Issue states (color-coded).

Use `--include-closed` to see the full history:

```
$ np rel graph --format dot --include-closed -o full-graph.dot
```

Requires [Graphviz](https://graphviz.org/) to render the DOT file into an image.

---

## Recovering from Mistakes

### Reopen a Closed Issue

```
$ np issue reopen <ISSUE-ID> --author <name>
```

### Restore a Deferred Issue

```
$ np issue undefer <ISSUE-ID> --author <name>
```

### Remove an Incorrect Blocking Relationship

```
$ np rel blocks unblock <A> <B> --author <name>
```

### Detach a Child from the Wrong Parent

```
$ np rel parent detach <A> <B> --author <name>
```

### Undo a Deletion

Deletion in `np` is soft — the issue is marked as deleted but remains in the database until garbage collected. If you have not yet run `np admin gc --confirm`, the issue can be recovered by reopening it (if you know the ID).

Once `np admin gc --confirm` has run, deleted issues are permanently removed.
