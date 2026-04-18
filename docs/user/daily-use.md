# Daily Use

Everyday commands for working with `np` once your workspace is set up. This guide covers the operations you will repeat most often: finding work, updating issues, and keeping the tracker healthy.

For first-time setup, see [Getting Started](getting-started.md). For specialized patterns such as epics, labels, and multi-agent workflows, see the guides linked from the [documentation index](README.md).

### A note on output modes

Commands in this guide default to human-readable text. AI agents should generally append `--json` where supported and prefer the `json` command group when sending structured input. See the [Agent Integration Guide](agent-integration.md) for agent-specific patterns.

---

## Finding work

### See what is ready

```
$ np ready
```

Ready issues are open, unblocked, and not deferred. For epics, readiness means having no children yet, which signals that they need decomposition.

### Claim the next ready issue

```
$ np claim ready --author <your-name>
```

Claiming assigns the issue to you and returns a claim ID. Save it. You need it for every mutation on that issue.

### Search for a specific issue

```
$ np issue search "login timeout"
```

### Check the overall backlog summary

```
$ np admin tally
```

---

## Working on an issue

### Update fields on a claimed issue

Use the interactive form if you are working directly in a terminal:

```
$ np form update --claim <CLAIM-ID>
```

Use JSON stdin for scripted or agent-driven updates:

```bash
$ np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title",
  "description": "More detail",
  "priority": "P1",
  "comment": "Updated after investigating the failure mode."
}
JSONEND
```

### Add a comment

Comments do not require claiming and can be added to any issue, including closed ones.

Interactive:

```
$ np form comment <ISSUE-ID>
```

Scripted:

```bash
$ np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Found the root cause."
}
JSONEND
```

---

## Closing and transitioning

### Close with a reason

```
$ np close --claim <CLAIM-ID> --reason "All tests pass."
```

This adds the reason as a comment and closes the issue in one step.

### Defer for later

```
$ np issue defer --claim <CLAIM-ID>
```

### Release without closing

Return the issue to the open pool so someone else can pick it up:

```
$ np issue release --claim <CLAIM-ID>
```

### Reopen a closed or deferred issue

```
$ np issue reopen <ISSUE-ID> --author <your-name>
$ np issue undefer <ISSUE-ID> --author <your-name>
```

---

## Creating issues

### Create interactively

```
$ np create
```

When stdin is a terminal, `np create` launches the same TUI flow as `np form create`.

### Create from JSON

```bash
$ np create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Implement retry logic",
  "priority": "P1"
}
JSONEND
```

### Create a task under an epic

```bash
$ np create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Write tests",
  "parent": "<EPIC-ID>"
}
JSONEND
```

### Create and claim in one step

```bash
$ np json create --author <your-name> --with-claim <<'JSONEND'
{
  "title": "Urgent fix"
}
JSONEND
```

---

## Unblocking work

### See what is blocked

```
$ np blocked
```

### Add or remove blocking relationships

```
$ np rel add <A> blocked_by <B> --author <your-name>
$ np rel blocks unblock <A> <B> --author <your-name>
```

### Run diagnostics when nothing is ready

```
$ np admin doctor
```

The doctor analyzes why no issues are ready and suggests specific unblocking actions. Add `--verbose` for per-check detail.

---

## Labels

### Set a label on a claimed issue

```
$ np label add kind:bug --claim <CLAIM-ID>
```

### Remove a label

```
$ np label remove kind --claim <CLAIM-ID>
```

### List all labels in use

```
$ np label list-all
```

---

## Inspecting issues

### Show full details

```
$ np show <ISSUE-ID>
```

### View the audit trail

```
$ np issue history <ISSUE-ID>
```

### Machine-readable output

Append `--json` where supported:

```
$ np show <ISSUE-ID> --json
$ np list --json
$ np admin tally --json
```

---

## Tips

- Always claim before mutating. Field updates and state transitions require an active claim.
- Document your work. Add a comment before closing to capture reasoning, trade-offs, and findings.
- Always transition state when done. Abandoned claims block other agents until the stale duration expires.
- Use `np admin doctor` when stuck. It diagnoses why no issues are ready and suggests fixes.
- Prefer structured flows for automation. Use `--json` output where available and the `json` command group for stdin-driven operations.
