# Core Concepts

This guide explains the model behind the `np` workflow. Read it after
[Quickstart](../quickstart.md), [Agent Setup](../agents/setup.md), and
[Daily Work](../daily-work.md) when you want the rules behind the commands.

`np` is easiest to understand as a local queue of work for one workspace.
Readiness decides what can be picked up, claims mark who is actively changing
an issue, and comments preserve context for the next human or agent.

## The Daily Loop

The basic workflow is intentionally small:

```bash
np ready
np claim ready --author <name>
# do the work
np close --claim <claim-id> --reason "Done."
```

Everything else in `np` supports that loop. Labels make a larger backlog easier
to organize, filter, and route. Epics are for high-level ideas that need
explicit decomposition into child work. Relationships express dependencies.
Agents use the same model through JSON commands.

## Workspace Locality

Each workspace has its own `.np/` directory. That directory contains the SQLite
database and related metadata for that workspace only.

`np` is deliberately:

- local-only
- non-invasive
- SQLite-backed
- CLI-driven
- per-workspace
- not an agent orchestrator

There is no hosted service, account, background daemon, global git hook, or
repo-coupled automation. `np` discovers the workspace by walking up from the
current directory until it finds `.np/`.

## Issues

Every issue is either a `task` or an `epic`.

A `task` is direct work. It is the default issue type and the right place to
start. You can use `np` effectively with tasks only.

An `epic` is a container for grouped work. Use epics when a feature or
initiative needs explicit decomposition, progress tracking, or grouped closure.
Do not start with epics unless labels and plain tasks are not enough.

Use labels for categories such as `kind:bug`, `area:auth`, or `risk:high`.
Labels are metadata, not issue types. They are usually the first tool to reach
for when you need to organize related work.

## Hierarchy

Hierarchy is for decomposition. Relationships are for lateral dependencies.

The intended hierarchy shape is:

```text
epic
|-- task
`-- epic
    `-- task
```

`np` enforces a maximum depth of three levels:

```text
Level 1: Epic
Level 2: Epic or Task
Level 3: Task
```

If you want deeper nesting, the hierarchy is probably carrying too much meaning.
Split the work differently or use `blocked_by` relationships to express
sequencing between peer issues.

## States

Primary states describe the lifecycle of an issue:

- `open` means the issue still needs attention.
- `closed` means the issue is resolved.
- `deferred` means the issue is intentionally shelved for later.

Secondary display states explain what is happening to an issue without changing
its primary lifecycle state:

- `ready` means the issue can be picked up now.
- `claimed` means someone has an active claim on it.
- `blocked` means an unresolved dependency prevents progress.
- `completed` applies to epics whose children are all resolved.

For example, a claimed task is still primarily `open`; it displays as
`open (claimed)` because active work is in progress.

## Priority

Priority describes urgency, not readiness.

Valid priorities are `P0` through `P4`:

- `P0` is the highest urgency.
- `P2` is the default for new issues.
- `P4` is the lowest urgency.

Priority affects ordering. `np claim ready` chooses from the ready queue by
priority first (`P0` before `P1`), then by creation time. Priority does not
make blocked, claimed, closed, or deferred work ready.

## Readiness

Readiness answers one question: what can be worked on now?

A task is ready when it is:

- open
- unclaimed
- not blocked by unresolved dependencies
- not under a deferred or blocked ancestor

An epic is ready when it is:

- open
- unclaimed
- not blocked by unresolved dependencies
- not under a deferred or blocked ancestor
- childless

A childless epic is ready because it is a planning gap: the next useful work is
to decompose it into child tasks or child epics. Once an epic has children, the
work moves to those children.

## Claims

Claims gate mutation. If you want to update fields, defer, delete, label, or
close an issue, you need the active claim ID for that issue.

Claims primarily exist for multi-agent coordination. They prevent two agents
from unknowingly mutating the same issue at the same time. They are still useful
for solo humans because they mark active work and force a clean finish: close
the issue, release the claim, or defer the issue.

Quick rules:

- Claim before mutating.
- Save the claim ID; it is the bearer token for later mutations.
- Comments do not require a claim.
- Non-structural relationships such as `blocked_by` and `refs` do not require a claim.
- Stale claims can be overwritten by a later normal claim.

The claim ID is sensitive. Treat it like a local bearer token: do not paste it
into places where another process should not be able to use it.

## Comments

Comments are the audit trail. Use them to record decisions, discoveries, failed
approaches, and handoff context.

Comments do not require a claim because they are additive. A human or agent can
leave context on an issue without taking ownership of the work.

`np close --reason` automatically records the reason as a comment and closes the
claimed issue in one step.

## Relationships

Relationships connect issues without changing their primary state.

The main relationship types are:

- `blocked_by` / `blocks` - directional dependency, affects readiness
- `refs` - informational link, does not affect readiness
- parent-child - structural hierarchy

Use `blocked_by` when one issue must finish before another can proceed. Use
`refs` when two issues are merely related. Use parent-child hierarchy when one
issue decomposes into smaller pieces of work.

When the ready queue looks wrong, run `np blocked` to inspect dependencies and
`np admin doctor` to diagnose stuck state.

## Labels

Labels are optional `key:value` metadata. They are useful for:

- classification
- filtering
- routing
- lightweight workflow markers

Start without labels. Add them when scanning the backlog gets noisy or when
humans and agents need a shared vocabulary for selecting work.

Labels can organize related work without creating hierarchy. For example, a
task whose description says "decompose this into follow-up tasks" can ask that
new tasks carry a shared label such as `initiative:auth-overhaul`.

## Epics

Tasks close directly.

Epics close indirectly. An epic is `completed` when all of its children are
resolved, but it remains open until `np epic close-completed` closes it. That
keeps epic progress tied to actual child work and avoids treating an empty
container as finished work.

This is why an empty open epic is ready: it needs decomposition. A non-empty
epic is not the work queue; its children are.

## Humans And Agents

Humans and agents use the same issue database and the same command surface.

Humans usually prefer interactive forms and text output:

```bash
np create
np form update --claim <claim-id>
np show <issue-id>
```

Agents and scripts should prefer JSON input and output:

```bash
np claim ready --author "$AUTHOR" --json
np show <issue-id> --json
np json update --claim <claim-id>
```

The important part is that both paths operate on the same queue. Claims,
readiness, blockers, comments, and closure mean the same thing whether the actor
is a human or an agent.

## Adoption Order

Use `np` in this order:

1. Start with plain tasks.
2. Wire in agents if agents will work in the repository.
3. Practice the daily loop until it feels routine.
4. Add labels when filtering, routing, or lightweight grouping becomes useful.
5. Add epics when a high-level idea needs explicit decomposition.
6. Read [Multi-Agent Operations](../agents/multi-agent.md) when concurrent agents share the workspace.

That is the intended shape of the product. Start simple and add structure only
when the pressure for it is real.
