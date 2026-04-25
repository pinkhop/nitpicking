# Multi-Agent Operations

This guide is for the point where more than one agent or developer is operating against the same `np` workspace on one machine.

## The Core Rule

Use `np claim ready` as the default work pickup mechanism:

```bash
$ np claim ready --author "$AUTHOR" --json
```

That command finds and claims the highest-priority ready issue atomically, so parallel agents do not race for the same task.

## Stable Agent Identity

At session start, generate a stable author name:

```bash
$ np agent name --seed=$PPID
```

Reuse that name for the whole session.

## The Safe Multi-Agent Loop

1. `np claim ready --author "$AUTHOR" --json`
2. Inspect with `np show <id> --json`.
3. Do the work.
4. Comment on non-obvious decisions.
5. Close with `np close --claim <claim-id> --reason "..."`.

If the issue should not stay claimed, release or defer it explicitly.

## Claim Conflicts

When `np claim <ID>` returns exit code `3`, someone else still holds the claim.

Your options are:

- claim something else with `np claim ready`
- wait for the stale time to pass
- inspect `claim_stale_at` with `np show <ID> --json`

## Stale Claims

Claims expire after their stale time. Once stale, another agent can claim the issue normally. No special recovery flag is required.

Check the situation:

```bash
$ np admin doctor
$ np show <ISSUE-ID> --json | jq '.claim_stale_at'
```

Recover the work:

```bash
$ np claim <ISSUE-ID> --author "$AUTHOR"
```

## Partition the Queue With Labels

When agents should specialize, label filters are the simplest routing mechanism:

```bash
$ np claim ready --author agent-a --label kind:bug --json
$ np claim ready --author agent-b --label kind:feature --json
$ np claim ready --author agent-c --label kind:docs --json
```

Use labels for lightweight routing, not assignment bureaucracy.

## Communicate Through Comments

Agents should write down:

- why they chose one approach over another
- what they discovered that affects nearby issues
- why work was deferred or released

Example:

```bash
$ np json comment FOO-a3bxr --author "$AUTHOR" <<'JSONEND'
{
  "body": "Chose approach B because approach A would require a schema migration."
}
JSONEND
```

## Capture Incidentals

When an agent discovers unrelated work:

1. Search for an existing issue.
2. Create one if needed.
3. Add a blocking relationship if it blocks the current task.
4. Otherwise keep going on the original task.

This prevents scope creep and stops discoveries from disappearing.

## Diagnose Stuck State

When the pool stops moving:

```bash
$ np admin doctor --verbose
```

Common causes:

- every issue is blocked
- every issue is claimed
- a deferred parent or ancestor in a hierarchical workspace
- the queue is over-filtered by labels

## Recovery Rules

- If the agent is done: close.
- If the agent is pausing: release.
- If the work should stop for now: defer.
- If the agent crashed: wait for staleness, then reclaim normally.

## Related Docs

- [Agent Setup](setup.md) for session bootstrap and static instructions
- [Labels](../labels.md) for routing patterns
- [Troubleshooting](../reference/troubleshooting.md) for symptom-driven recovery
