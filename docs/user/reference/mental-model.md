# Mental Model

This is the conceptual reference for `np`. Read it when you want the rules behind the workflow, not just the command sequence.

## Design Constraints

Nitpicking is deliberately:

- local-only
- non-invasive
- SQLite-backed
- CLI-driven
- per-workspace
- not an agent orchestrator

Those choices keep setup friction low and keep the tool honest about its scope.

## Issue Roles

Every issue is either:

- `task` - direct work
- `epic` - a container for grouped work

The role system is intentionally small. Use labels for other categories such as bug, docs, feature, or refactor.

## Hierarchy

The recommended mental model is:

```text
epic
├── task
└── epic
    └── task
```

The hierarchy depth limit is three levels:

```text
Level 1: Epic
Level 2: Epic or Task
Level 3: Task
```

Use hierarchy for decomposition. Use relationships for lateral dependencies.

## States

Primary states:

- `open`
- `closed`
- `deferred`

Secondary display states matter too:

- `ready` means work can be picked up now
- `claimed` means an active claim exists
- `completed` applies to epics whose children are all resolved

## Claims

Claims gate mutation. If you want to change issue fields or transition state, hold the claim.

Quick rules:

- claim before mutating
- comments do not require a claim
- most relationships do not require a claim
- stale claims are overwritten by later normal claims

The claim ID is a bearer token. If you lose it, you wait for staleness and reclaim normally.

## Readiness

Readiness answers: "what can be worked on now?"

A task is ready when it is:

- open
- unclaimed
- unblocked
- not suppressed by a deferred ancestor epic

An epic is ready when it is:

- open
- unclaimed
- unblocked
- childless
- not suppressed by a deferred ancestor epic

That last point is important: an empty epic is not "work in progress." It is a planning gap waiting for decomposition.

## Relationships

The main relationship types are:

- `blocked_by` / `blocks` - directional dependency, affects readiness
- `refs` - informational link, does not affect readiness
- parent-child - structural hierarchy

Use `blocked_by` when one issue must finish before another can proceed. Use `refs` when two issues are merely related.

## Why Epics Close Indirectly

Tasks close directly.

Epics do not. An epic is considered complete when all children are resolved, and `np epic close-completed` turns that structural completion into a closed issue. This prevents premature closure and keeps progress tied to the actual child work.

## Where Labels Fit

Labels are optional metadata, not part of the core issue model. They are for:

- classification
- filtering
- routing
- lightweight workflow markers

If the tracker feels useful without labels, keep it that way until the need is real.

## Recommended Adoption Order

1. tasks only
2. daily issue loop
3. epics for grouped work
4. labels for filtering and routing
5. multi-agent coordination when concurrency becomes real

That is the intended shape of the product. Start simple and add structure only when the pressure for it is obvious.
