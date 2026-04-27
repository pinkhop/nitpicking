# Daily Work

This guide picks up after [Quickstart](quickstart.md). It stays task-first and focuses on the commands you will use repeatedly once the workspace already exists.

## The Everyday Loop

Most sessions look like this:

1. Find work
2. Claim work
3. Update the issue while working
4. Comment on findings
5. Close, defer, or release it

## Find Work

See what is ready now:

```bash
$ np ready
```

Claim the next ready issue:

```bash
$ np claim ready --author <your-name>
```

Search by text:

```bash
$ np issue search "login timeout"
```

See the broader queue:

```bash
$ np list
$ np admin tally
```

## Inspect an Issue

Show one issue in detail:

```bash
$ np show <ISSUE-ID>
```

View its history:

```bash
$ np issue history <ISSUE-ID>
```

For automation, prefer machine-readable output:

```bash
$ np show <ISSUE-ID> --json
$ np list --json
$ np admin tally --json
```

## Update While Working

If you hold the claim, update fields with the form:

```bash
$ np form update --claim <CLAIM-ID>
```

Or update through JSON:

```bash
$ np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title",
  "description": "More detail",
  "priority": "P1",
  "comment": "Expanded scope after investigating the failure mode."
}
JSONEND
```

## Comment As You Go

Comments do not require a claim, so use them freely to capture reasoning and discoveries.

```bash
$ np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Root cause is the retry loop skipping context cancellation."
}
JSONEND
```

## Finish Cleanly

Close when done:

```bash
$ np close --claim <CLAIM-ID> --reason "Implemented fix and verified with tests."
```

Defer when the work should stop for now:

```bash
$ np issue defer --claim <CLAIM-ID>
```

Release when you are pausing but do not want to change the issue state:

```bash
$ np issue release --claim <CLAIM-ID>
```

Reopen or undefer later:

```bash
$ np issue reopen <ISSUE-ID> --author <your-name>
$ np issue undefer <ISSUE-ID> --author <your-name>
```

## Handle Dependencies

See what is blocked:

```bash
$ np blocked
```

Add a blocker:

```bash
$ np rel add <A> blocked_by <B> --author <your-name>
```

Remove a blocker:

```bash
$ np rel remove <A> blocked_by <B> --author <your-name>
```

When nothing is ready, ask the doctor:

```bash
$ np admin doctor
$ np admin doctor --verbose
```

## Good Habits

- Claim before mutating.
- Comment before closing when the work involved a decision, tradeoff, or investigation.
- Release abandoned work instead of letting claims go stale.
- Use `np admin doctor` when the queue stops making sense.
- Keep the early workflow simple. Add labels when filtering, routing, or grouping becomes useful. Reach for epics only when flat tasks plus labels are not enough.

## When To Add Structure

Read [Labels](labels.md) when:

- backlog scanning is getting noisy
- different kinds of work need different routing
- you want better filtering without changing the issue model
- related tasks need a shared grouping label such as `task-group:<name>`
- a high-level idea should become a planning task that asks for follow-up tasks with the same grouping label

Read [Epics](epics.md) when:

- flat tasks plus labels are not enough to express the structure
- you need explicit parent-child hierarchy
- you want structural progress tracking or grouped closure for a body of work
