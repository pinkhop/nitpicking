# Workflow: Label-Driven Issue Selection

Use labels to categorize issues and filter work by type. This workflow is useful when agents (or developers) specialize — one handles bugs, another handles documentation, a third works on features.

---

## Establishing Label Conventions

Labels are key-value pairs. Establish conventions early so labels are consistent across the project:

| Key | Purpose | Example values |
|-----|---------|---------------|
| `kind` | Type of work | `bug`, `feature`, `docs`, `fix`, `refactor`, `test` |
| `area` | Codebase area | `api`, `frontend`, `auth`, `storage`, `cli` |
| `scope` | Component or module | `claim`, `issue`, `rel`, `label`, `comment` |
| `docs` | Documentation topic | `getting-started`, `command-reference`, `workflow` |

Conventions are not enforced by `np` — they work by agreement. Use `np label list-all` to see what labels are already in use.

### Reserved system labels

Some label keys are reserved by `np` for internal use. These **virtual labels** are backed by dedicated database columns rather than the labels table — they look like labels in output and filtering, but reads and writes are redirected to their columns.

| Key | Purpose |
|-----|---------|
| `idempotency-key` | Deduplication key for imports; backed by the `issues.idempotency_key` column |

**Naming convention:** System labels use hyphen-separated keys (e.g., `idempotency-key`). User-defined labels conventionally use short alphanumeric keys (e.g., `kind`, `area`, `scope`). This reduces collision risk — hyphens are valid label key characters but uncommon in user-defined keys.

**Key validation rules:** A label key must be 1–64 bytes of ASCII printable characters. The **first character must be an ASCII letter (`A`–`Z` or `a`–`z`) or an underscore (`_`)**; interior and trailing characters may be any ASCII printable non-whitespace character. Leading digits, hyphens, colons, and other punctuation are rejected to avoid ambiguity with CLI filter grammar (e.g., leading `!` means negation in `--label` filters) and to align with the identifier convention developers intuitively recognize as a "name".

---

## Labeling Issues

### At Creation Time

Include labels in the JSON payload when creating an issue:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Fix claim timeout race condition",
  "labels": ["kind:bug", "area:auth"]
}
JSONEND
```

The `labels` field accepts an array of `key:value` strings.

### After Creation

Label an already-claimed issue:

```
$ np label add kind:bug --claim a4dace30
```

To update labels on an existing issue, claim it first, then use `label add` or `label remove`.

---

## Filtering Issues by Label

### Finding Ready Work

Filter `np ready` or `np list` by label:

```
$ np list --ready --label kind:bug
MYAPP-2e22n  task  P1  Fix claim timeout race condition
MYAPP-x64m6  task  P2  Handle nil pointer in search results
```

Multiple `--label` flags narrow the results (all labels must match):

```
$ np list --ready --label kind:bug --label area:auth
MYAPP-2e22n  task  P1  Fix claim timeout race condition
```

### Claiming Filtered Work

Use `np claim ready` with label filters to claim the highest-priority issue matching your criteria:

```
$ np claim ready --author alice --with-label kind:bug
[ok] Claimed MYAPP-2e22n
  Claim ID: f2fa05ba73d90760db00682f21df60f0
```

This is the core of label-driven selection — an agent configured to only work on `kind:bug` issues can loop on this command, pulling and resolving bugs in priority order.

---

## Discovering Labels

List all unique labels across the database:

```
$ np label list-all
area:api
area:auth
kind:bug
kind:docs
kind:feature
scope:claim
```

List labels for a specific issue:

```
$ np label list MYAPP-2e22n
kind:bug
area:auth
```

---

## Propagating Labels

When an epic should share a label with all its descendants, use propagation instead of labeling each child individually:

```
$ np label propagate kind --issue MYAPP-a3bxr --author alice
```

This copies the `kind` label (and its value) from the epic to every descendant. Descendants that already have the same key-value pair are skipped. Each descendant is atomically claimed, labeled, and released.

---

## Practical Example: Bug-Fixing Agent Loop

An AI agent configured to work only on bugs runs this loop:

```
# At session start:
AUTHOR=$(np agent name)

# Work loop:
while true; do
    RESULT=$(np claim ready --author "$AUTHOR" --with-label kind:bug --json)

    if [ $? -ne 0 ]; then
        echo "No bugs ready. Done."
        break
    fi

    ISSUE_ID=$(echo "$RESULT" | jq -r '.issue_id')
    CLAIM_ID=$(echo "$RESULT" | jq -r '.claim_id')

    # ... do the work ...

    np json comment "$ISSUE_ID" --author "$AUTHOR" <<'JSONEND'
{
  "body": "Fixed: ..."
}
JSONEND
    np close --claim "$CLAIM_ID" --reason "Bug resolved."
done
```

Key patterns:

- **Use `--json`** for machine-readable output that agents can parse.
- **Check exit codes** — exit 0 means success; exit 2 means no matching issues found.
- **Always close when done** — abandoned claims block other agents until the stale duration expires.

---

## Combining with Other Workflows

Label-driven selection works alongside any workflow:

- **Simple tasks** — label tasks and use `np claim ready --with-label` to pull specific types.
- **Epic-driven** — propagate labels from epics to children, then agents filter by label to specialize.
- **Multi-agent** — each agent uses a different label filter, naturally partitioning work without explicit coordination.
