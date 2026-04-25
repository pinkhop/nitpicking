# Using Labels Effectively

Labels are optional `key:value` tags you attach to issues. They are not the place to start with `np`.

Start with plain tasks first. Add labels only when the backlog is large enough that you need better filtering, routing, reporting, or grouping.

When you do need them, labels are flexible metadata. `np` does not enforce which keys exist or which values are valid. That flexibility is the point: use labels for context that is useful to your workspace but not important enough to become a built-in field.

Labels are also the default way to keep related work together without creating hierarchy. A shared label such as `task-group:auth-overhaul` can group tasks for a feature, cleanup pass, incident follow-up, or planning effort.

This guide covers naming conventions, filtering, flat grouping, routing, and higher-order patterns that become useful as your backlog grows.

---

## Labels in 60 Seconds

Start with a small vocabulary. For most workspaces, `kind` and `area` are enough to make the backlog easier to scan and filter. Add `task-group` when related tasks need lightweight grouping.

Create an issue with labels attached:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Fix auth token expiry race condition",
  "priority": "P1",
  "labels": ["kind:bug", "area:auth"]
}
JSONEND
```

Add labels later if needed:

```bash
$ np claim FOO-a3bxr --author alice
Claimed FOO-a3bxr
  Claim ID: c1a1m2...

$ np label add kind:bug --claim c1a1m2...
$ np label add area:auth --claim c1a1m2...
$ np issue release --claim c1a1m2...
```

Filter by one label:

```bash
$ np list --label kind:bug
```

Filter by two labels:

```bash
$ np list --label kind:bug --label area:auth
```

Match any value for a key:

```bash
$ np list --label area:*
```

`--label` filters use AND semantics. There is no OR operator. `area:*` means "has any `area` label", not "all areas" or "one of these areas".

## Label Syntax And Validation

Label keys and values are stored as `key:value`.

Key rules:

- A key must be 1-64 bytes of ASCII printable characters.
- The first character must be an ASCII letter (`A-Z` or `a-z`) or underscore (`_`).
- Remaining characters may be any ASCII printable non-whitespace character.
- Leading digits, hyphens, and punctuation are rejected.

Practical implications:

- `kind:bug` is valid.
- `area:auth` is valid.
- `_queue:fast` is valid.
- `1kind:bug` is invalid.
- `-kind:bug` is invalid.

`np` does not reserve label keys for internal use. Your workspace owns the full label vocabulary.

---

## Recommended Starter Vocabulary

These keys provide the highest value early.

| Key | Common values | Why use it |
|---|---|---|
| `kind` | `bug`, `feature`, `docs`, `chore`, `enhancement`, `spike` | What sort of work this is |
| `area` | workspace-defined (`auth`, `api`, `cli`, `storage`, `ui`) | Where it lives |
| `task-group` | workspace-defined (`auth-overhaul`, `q2-cleanup`) | Lightweight grouping without hierarchy |
| `effort` | `xs`, `s`, `m`, `l`, `xl` | Rough size |
| `risk` | `low`, `med`, `high` | Blast radius if the change goes wrong |

Everything else in this guide is optional. Add more keys only when they solve a real sorting, filtering, or coordination problem.

### `kind`

`kind` answers "what is this issue?"

| Value | Meaning |
|---|---|
| `bug` | Something is broken |
| `feature` | New capability |
| `docs` | Documentation-only work |
| `chore` | Maintenance work |
| `enhancement` | Improvement to an existing capability |
| `spike` | Time-boxed investigation |

Some workspaces use `spike:true` instead of `kind:spike`. Either is fine. Pick one and keep it consistent.

### `area`

`area` answers "where does this work land?"

```
area:auth       area:cli        area:storage
area:api        area:ui         area:agent
```

You can use a different key name such as `component`, `service`, or `module`. What matters is consistency across the workspace.

### `effort` and `risk`

Priority tells you how urgent something is. `effort` and `risk` tell you how large it is and how careful you need to be.

| Label | Meaning |
|---|---|
| `effort:xs` | An hour or less |
| `effort:s` | A few hours |
| `effort:m` | A day or two |
| `effort:l` | About a week |
| `effort:xl` | More than a week; consider splitting |
| `risk:low` | Small, isolated, easy to revert |
| `risk:med` | Non-trivial surface area |
| `risk:high` | Critical path, auth, migrations, or shared infrastructure |

These pair well in filters:

```bash
$ np ready --label effort:xs --label risk:low
```

---

## Common Patterns

### Make the backlog scannable

Labels pay off before you run a single filtered query. A backlog with `kind` and `area` labels reads faster because readers can recognize patterns without opening each issue.

If you are deciding where to start, bias toward labels that appear on most issues. `kind` and `area` usually come first. Add `effort`, `risk`, or `source` later if they solve real triage problems.

To inspect the label vocabulary already in use:

```bash
$ np label list-all
KEY       POPULAR VALUES
area      auth (12), cli (8), storage (5)
kind      bug (19), feature (11), docs (8)
risk      low (10), med (4), high (2)
```

This is also your best drift detector. If `kind:bug` and `kind:Bug` both appear, your conventions have started to drift.

### Filter and find issues

`--label` works on `np list`, `np ready`, and `np issue search`.

Common queries:

```bash
$ np list --label kind:bug
$ np list --label kind:bug --label area:auth
$ np ready --role task --label skill:go --label repo:backend
$ np issue search "timeout" --label area:api
```

There is no OR operator. If you need "bugs or enhancements", run two queries or use `kind:*` and narrow the results manually.

### Drive issue selection with labels

Labels are especially useful when you want `np claim ready` to pull a specific kind of work instead of simply taking the next ready issue overall.

Claim the highest-priority ready bug:

```bash
$ np claim ready --author alice --label kind:bug
```

Claim work in a specific area:

```bash
$ np claim ready --author alice --label kind:feature --label area:auth
```

For agents and scripts, prefer JSON output:

```bash
$ np claim ready --author "$AUTHOR" --label kind:bug --json
```

That gives you a lightweight routing mechanism without introducing assignment fields or an orchestration layer.

### Group related tasks without hierarchy

Use a shared label when a body of work should stay flat but still be easy to list, route, or report on.

For a high-level idea that needs decomposition, create a planning task that says how to split it, then create follow-up tasks with the same grouping label:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Plan authentication overhaul",
  "description": "Decompose this into implementation tasks. Apply task-group:auth-overhaul to each follow-up task and keep area labels specific.",
  "priority": "P1",
  "labels": ["kind:spike", "area:auth", "task-group:auth-overhaul"]
}
JSONEND
```

Assume that planning task returns `FOO-a3bxr`. Then create the actual work as peer tasks:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Add JWT token generation service",
  "priority": "P1",
  "labels": ["kind:feature", "area:auth", "task-group:auth-overhaul", "spawned-from:FOO-a3bxr"]
}
JSONEND

$ np create --author alice <<'JSONEND'
{
  "title": "Replace session middleware with JWT validation",
  "priority": "P1",
  "labels": ["kind:feature", "area:auth", "task-group:auth-overhaul", "spawned-from:FOO-a3bxr"]
}
JSONEND
```

List the group when you want to see the whole body of work:

```bash
$ np list --label task-group:auth-overhaul
```

Use the grouping label for the set, and use a pointer label only when you want to know which planning task produced a follow-up. `spawned-from:<issue-id>` is clearer than `parent:<issue-id>` because `np` already uses "parent" for real hierarchy.

The planning task can be closed once the follow-up tasks exist, or left open if the decomposition itself still needs work.

### Group work by time

Temporal labels are useful when grouping is mainly for planning, reporting, or release coordination.

Common patterns:

```
sprint:2026-w16
milestone:v0.4.0
quarter:2026Q2
```

Attach them directly to the tasks in that timebox, then query the group:

```bash
$ np list --label sprint:2026-w16
$ np ready --label milestone:v0.4.0
```

If a group needs structural completion instead of lightweight filtering, read [Epics](epics.md).

### Record where work came from

Once the basics are in place, provenance labels often pay for themselves quickly:

```
source:user-report
source:code-review
source:postmortem
source:linter
source:fuzz
```

These answer questions like:

```bash
$ np list --label source:postmortem --label quarter:2026Q2
```

Keep provenance separate from impact. For example, `source:user-report` means "this was filed because a user reported it", while `customer-reported` means "this is currently affecting a customer".

---

## Advanced Patterns

These are useful once labels are doing more than simple classification.

### Cross-reference external systems

A good pattern for external identifiers is `<system>:<id>`:

| Label | Meaning |
|---|---|
| `jira:PKHP-1234` | JIRA ticket |
| `gh-issue:123` | GitHub issue |
| `pr:456` | Pull request |
| `commit:abc1234` | Commit |
| `fixed-in:v0.3.1` | Release containing the fix |

Put the identifier in the value, not in a comment. That keeps the label searchable and self-describing:

```bash
$ np list --label pr:456
$ np list --label jira:PKHP-1234
$ np list --label fixed-in:v0.3.1
```

### Route work to the right agent

In multi-agent or multi-repo workflows, labels can partition the queue without an explicit assignment system.

Repository-based routing:

```bash
$ np claim ready --author "$AUTHOR" --label repo:backend --json
```

Skill-based routing:

```bash
$ np claim ready --author "$AUTHOR" --label skill:go
$ np claim ready --author "$AUTHOR" --label skill:docs
```

You can compose these:

```bash
$ np claim ready --author "$AUTHOR" --label skill:go --label repo:backend --label area:storage
```

For machine-driven loops, keep the pattern simple:

```bash
$ RESULT=$(np claim ready --author "$AUTHOR" --label kind:bug --json)
$ CLAIM_ID=$(echo "$RESULT" | jq -r '.claim_id')
```

Use `--json` whenever another program needs to parse the result, and treat a non-zero exit as "no matching work" or an operational failure that needs handling.

### Add lightweight workflow gates

Labels can model process checkpoints that `np` does not treat as first-class state:

```
needs-review
ready-to-merge
review-gate:security
approved:security
release-notes:yes
release-notes:breaking
```

Use them when you want filterable process markers without changing the issue state machine.

Examples:

```bash
$ np list --label review-gate:security
$ np list --label release-notes:breaking --label milestone:v0.4.0
```

If you use gate labels, keep the meaning clear. `review-gate:security` means approval is required and still pending. `approved:security` means the review happened and passed.

### Capture closure and deferral context

Some labels explain why work ended or why it is stalled.

Examples:

```
resolution:fixed
resolution:wontfix
resolution:superseded
resolution:implemented

defer-reason:requires-human
defer-reason:unclear
defer-reason:too-big

blocked-reason:upstream-lib
blocked-reason:waiting-human
blocked-reason:spec-unclear

needs:repro
needs:design
needs:decision
```

These are useful when issue state alone is too coarse.

```bash
$ np list --state deferred --label defer-reason:unclear
$ np list --label needs:repro
```

When closing an issue, adding a `resolution:*` label can save future readers time:

```bash
$ np label add resolution:wontfix --claim <claim-id>
$ np close --claim <claim-id> --reason "Design decision: out of scope for v1."
```

### Use labels as informal pointers, not structural relationships

Sometimes you want a visible reference to another issue without creating a first-class `np rel` edge.

Examples:

```
duplicate-of:FOO-abc12
superseded-by:FOO-xyz99
spawned-from:FOO-q1w2e
```

This works well when:

- You want the reference visible in `np list` output.
- The relationship is informal or one-way.
- You do not need blocking semantics or graph traversal.

Use `np rel` instead when the relationship should affect readiness, show up in `np blocked`, or behave like a real dependency.

---

## Filtering Recipes

These are the combinations most worth memorizing.

```bash
$ np list --label kind:bug --label area:auth
$ np list --label task-group:auth-overhaul
$ np ready --role task --label skill:go
$ np claim ready --author "$AUTHOR" --label repo:backend --json
$ np ready --label effort:xs --label risk:low
$ np list --label sprint:*
$ np list --label source:postmortem --label quarter:2026Q2
$ np list --label review-gate:security
```

All of these use AND semantics. There is no OR operator.

---

## Keep Conventions Tight

`np` will happily store both `kind:bug` and `kind:BUg`. Label consistency is a workspace convention problem, not a tool problem.

Two lightweight ways to keep the vocabulary stable:

Write it into `CLAUDE.md`:

```markdown
## Label conventions

- kind: bug | feature | docs | chore | enhancement | spike
- area: auth | api | cli | storage | ui
- task-group: short-slug for flat grouping
- spawned-from: np issue ID for follow-ups from a planning task
- effort: xs | s | m | l | xl
- risk: low | med | high
```

Or keep a conventions issue in `np` itself:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Label vocabulary and conventions",
  "labels": ["kind:docs", "doc-target:agent"],
  "description": "Agreed label keys and values for this workspace. Update this issue when conventions change."
}
JSONEND
```

Run `np label list-all` periodically to spot drift and fix outliers before they spread.
