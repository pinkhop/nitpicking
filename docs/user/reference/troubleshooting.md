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
$ np admin doctor
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

## Useful Diagnostics

Doctor:

```bash
$ np admin doctor --verbose
```

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
$ np rel blocks unblock <A> <B> --author <name>
```

Detach the wrong parent:

```bash
$ np rel parent detach <CHILD> <PARENT> --author <name>
```
