# Troubleshooting

Common problems are organized by symptom.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Not found |
| 3 | Claim conflict |
| 4 | Validation error |
| 5 | Database error |

## No Database Found

Symptom:

Commands fail because no `.np/` directory can be found.

Check:

```bash
$ np admin where
```

Fix:

- move into the right workspace
- run `np init <PREFIX>` if the workspace was never initialized

## Claim Conflict

Symptom:

`np claim <ID>`, `np json update`, or `np close` returns exit code `3`.

Check:

```bash
$ np show <ISSUE-ID> --json | jq '.claim_author, .claim_stale_at'
```

Fix:

- wait for the stale time to pass
- claim a different issue
- verify you are using the right claim ID

## Nothing Is Ready

Symptom:

`np ready` shows nothing or `np claim ready` returns exit code `2`.

Check:

```bash
$ np admin doctor
$ np blocked
```

Typical causes:

- everything is blocked
- everything is already claimed
- a deferred parent or ancestor in a hierarchical workspace
- label filters are excluding the queue you expected

## Stale Claims

Symptom:

An issue is still claimed but the claimer is gone.

Check:

```bash
$ np show <ISSUE-ID> --json | jq '.claim_stale_at'
```

Fix:

Once stale, reclaim normally:

```bash
$ np claim <ISSUE-ID> --author <name>
```

## Validation Error

Symptom:

A command returns exit code `4`.

Typical causes:

- missing `--author`, `--claim`, `--reason`, or `--confirm`
- invalid transition
- invalid role or priority
- hierarchy too deep

Fix:

Read the specific error text, then check the [Command Reference](command-reference.md).

## Database Error

Symptom:

A command returns exit code `5`.

Typical causes:

- running `np init` twice
- database corruption
- write contention or interruption

Fix:

- do not re-run `np init` on an initialized workspace
- restore from backup if the database is damaged
- use reset only when you really intend to wipe the database

## Database Not in .gitignore

Symptom:

`np admin doctor` reports a `git-ignore` warning: ".np/ directory is not ignored by git".

Fix:

```bash
$ np admin fix git-ignore
```

Use `--dry-run` to preview before applying. The fix is idempotent — re-running it is safe.

## Invalid Parent References

Symptom:

`np admin doctor` reports an `invalid-parent-reference` finding, or issues behave unexpectedly because their parent issue no longer exists.

Fix:

```bash
$ np admin fix invalid-parent-reference --author <name>
```

Use `--dry-run` to preview which issues would be affected before applying. Repaired issues become top-level issues; an audit comment is recorded on each one.

## Useful Diagnostics

Doctor — runs 16 checks across four categories (database, environment, graph health, issue lifecycle):

```bash
$ np admin doctor --verbose
$ np admin doctor --json --verbose
```

Exit codes: `0` = all passed, `1` = warnings present, `2` = errors present. See [Command Reference](command-reference.md#admin-doctor) for the full flag reference and JSON output shape.

Relationship graph:

```bash
$ np rel graph --format dot --output issues.dot
$ dot -Tpng issues.dot -o issues.png
```

## Recovering From Common Mistakes

Reopen a closed issue:

```bash
$ np issue reopen <ISSUE-ID> --author <name>
```

Undefer an issue:

```bash
$ np issue undefer <ISSUE-ID> --author <name>
```

Remove a bad blocking edge:

```bash
$ np rel remove <A> blocked_by <B> --author <name>
```

Detach the wrong parent:

```bash
$ np rel parent detach <CHILD> <PARENT> --author <name>
```
