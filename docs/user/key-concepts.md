# Key Concepts

This guide covers the mental model you need to work effectively with nitpicking (`np`). It explains *why* things work the way they do — not just the mechanics, but the reasoning behind each design choice. If you want command syntax and flags, see the command reference; this document is about understanding the system.

---

## Design Philosophy

Nitpicking is built on a small set of principles that inform every feature and every deliberate omission.

### Local-only

Nitpicking runs entirely on a single machine. There is no network access, no remote sync, no cloud backend. Your issues live in an embedded SQLite database inside your workspace directory — nothing leaves your machine unless you choose to commit the database to version control.

**Why it matters:** Local-only means zero setup friction, zero latency, and zero dependency on external services. You can use `np` on an airplane, in a sandboxed environment, or on a machine with no internet access. It also means there is no account to create, no API key to manage, and no service to trust with your project data.

### Non-invasive

Nitpicking does not install global hooks, does not couple itself to your git lifecycle, and does not run background daemons. It is a standalone CLI that you invoke when you need it.

**Why it matters:** Invasive tools create hidden dependencies. Global git hooks can break workflows across unrelated projects. Background daemons consume resources and introduce failure modes. Nitpicking avoids all of this — it does one thing (track issues) and stays out of the way otherwise.

### Embedded storage — no database server

Nitpicking uses SQLite embedded directly in the `np` binary. There is no separate database process to install, configure, or keep running.

**Why it matters:** A database server adds operational complexity — installation, configuration, port management, process supervision. Embedded SQLite eliminates all of that. The database is a single file inside `.np/`, which makes backups trivial and the tool completely self-contained.

### CLI-driven

The `np` command is the sole interface. It is designed to be called equally well by humans typing in a terminal and by AI agents executing commands programmatically.

Every command supports two output modes: human-readable text (the default, formatted for terminal readability) and structured JSON (`--json`). **Agents should always use `--json`** — it is stable across versions and designed for programmatic parsing. Humans working interactively can rely on the default text output.

**Why it matters:** A single, well-defined interface means there is exactly one way to interact with the system. AI agents do not need a special SDK or API — they use the same commands a human would. This keeps the tool simple and auditable.

### Per-workspace databases

Each workspace gets its own issue database. You decide the scope boundary by choosing where to run `np init` — a single repository, a parent directory spanning multiple repos, or any other directory structure that makes sense for your workflow.

**Why it matters:** Per-workspace scoping means issues are always relevant to the context you are working in. There is no global issue database to query, filter, or accidentally pollute. When you delete a workspace directory, its issues go with it.

### No agent orchestration

Nitpicking tracks issues. It does not coordinate which agent works on what, does not assign tasks to agents, and does not manage agent lifecycles. That is the developer's responsibility.

**Why it matters:** Orchestration is a separate concern with its own complexity. Conflating issue tracking with agent coordination leads to scope creep and opinionated workflow assumptions. Nitpicking provides the building blocks (issues, claiming, readiness) and lets you compose them into whatever workflow suits your situation.

---

## Issue Roles

Every issue has exactly one of two roles: **task** (a leaf work item you complete directly) or **epic** (a container whose completion is determined by the `completed` secondary state). Use labels for further categorization (e.g., `kind:bug`, `kind:feat`).

### Task

A task is a leaf-node work item — it represents something that can be directly worked on and completed. Tasks are the fundamental unit of progress.

- A task can be created, claimed, updated, and closed through explicit state transitions.
- A task does not decompose further in the core user workflow. If you find yourself wanting sub-tasks, create an epic instead and make the work items children of that epic.
- A task may stand alone or belong to a parent epic.

### Epic

An epic organizes other issues. Its purpose is to group related work and track collective progress. An epic's completion is governed by the `completed` secondary state — it is complete when all of its children are closed or complete, and it cannot be closed directly through a command.

- An epic can have children of any role — tasks or other epics.
- An epic can be claimed (to edit metadata or decompose it into children), deferred, and reopened.
- An epic's progress is always a reflection of its children's states — there is no way to mark an epic "done" independently.

### Why only two roles?

Two roles cover the vast majority of issue-tracking needs without introducing taxonomic complexity. An epic is anything that decomposes into smaller work; a task is anything that does not. You do not need to decide whether something is a "story", "bug", "feature request", or "tech debt" at the type level — that is what labels are for (e.g., `kind:feat`, `kind:fix`, `kind:refactor`).

Keeping the role set minimal also simplifies the state machine. Tasks and epics have different but straightforward state models, and every rule in the system can be expressed in terms of these two roles.

### Hierarchy and depth

Issues form a tree through parent–child relationships. Nitpicking enforces a maximum depth of three levels:

```
Level 1:  Epic (root)
Level 2:    ├── Epic or Task
Level 3:    │     └── Task
```

A root issue is level 1, its child is level 2, and its grandchild is level 3. Attempts to create a fourth level are rejected.

**Why three levels?** Three levels provide enough structure for meaningful decomposition (initiative → feature area → work item) without allowing the kind of deeply nested hierarchies that become difficult to navigate and reason about. If you need more granularity, use relationships (`blocked_by`, `refs`) to connect issues laterally rather than nesting deeper.

For users, the mental model should stay simple: epics organize hierarchy, and tasks are leaves. In practice that means level 1 is typically an epic, level 2 is an epic or task, and level 3 is a task.

Advanced note: some commands operate on the underlying parent field more generically than this mental model suggests. Treat that as implementation detail rather than the recommended planning pattern.

---

## State Machine

Every issue moves through four states: `open`, `claimed`, `closed`, and `deferred`. All state changes except claiming require that you hold the active claim.

| State      | Meaning |
|------------|---------|
| `open`     | Available for work. This is the default state at creation. |
| `claimed`  | An agent or human has taken ownership and is actively working on it or updating its fields. |
| `closed`   | Fully resolved. Terminal for tasks; reached via the `completed` secondary state for epics. |
| `deferred` | Shelved — should not be worked on now. Can be restored later. |

### Transitions

All state changes — except claiming itself — require that you hold the active claim on the issue. This is the central concurrency guarantee in nitpicking.

```
                    claim
           ┌─────────────────────┐
           │                     ▼
        ┌──────┐  release   ┌─────────┐  close   ┌────────┐
  ───▶  │ open │◀───────────│ claimed │────────▶  │ closed │
        └──────┘            └─────────┘           └────────┘
           ▲                     │                     │
           │     defer           │                     │
           │        ┌────────────┘               reopen│
           │        ▼                                  │
        ┌──────────┐                                   │
        │ deferred │                                   │
        └──────────┘                                   │
           ▲    │                                      │
           │    │  undefer                             │
           │    └──────────▶ ┌──────┐ ◀────────────────┘
           │                 │ open │
           └─────────────────┴──────┘
```

Reading the diagram:

- **open → claimed**: Anyone can claim an open issue. No prior claim needed.
- **claimed → open**: Release the claim without completing the work. The issue returns to the pool.
- **claimed → closed**: Mark the work as done. For tasks, this is a direct action. For epics, closure happens automatically when all children are resolved.
- **claimed → deferred**: Shelve the issue for later. Optionally attach a revisit date.
- **deferred → open**: Restore a deferred issue with `undefer`. It becomes available for work again.
- **closed → open**: Reopen a closed issue if the resolution turns out to be incomplete or incorrect.

### Tasks vs epics

Both roles use the same states, but they differ in how `closed` is reached:

- **Tasks** are closed directly — you claim the task, do the work, and close it.
- **Epics** are closed through the `completed` secondary state — an epic is complete when all of its children are closed or recursively complete. The `epic close-completed` command batch-closes epics that meet this criterion.

### Terminal vs recoverable states

`closed` and `deferred` look similar in that neither is actively being worked on, but they differ in intent:

- `closed` means "this is done." You can reopen it if needed, but the default expectation is that it stays closed.
- `deferred` means "not now, but later." It is explicitly recoverable — the issue is waiting for the right time, not resolved.

---

## Claiming

Claiming gates all field updates and state transitions. You must claim an issue before changing its title, priority, description, or state.

**Quick rules:**

- Claim before mutating. Every mutation requires the claim ID.
- Comments and relationships (except parent–child) do not require claiming.
- Claims go stale after 2 hours of inactivity (configurable up to 24 hours).
- A stale claim can be stolen by any agent.

### How claiming works

When you claim an issue, `np` returns a **claim ID** — a random bearer token. Pass this token to every subsequent mutation. There is no login, no session, no user account; possession of the claim ID is the sole proof of ownership.

If you lose the claim ID, it cannot be recovered — it is only revealed at claim time. Wait for the claim's stale time to expire, then reclaim or steal the issue.

### What does not require claiming

- **Comments** — anyone can comment on any issue at any time, including closed issues.
- **Relationships** — `blocked_by`, `blocks`, and `refs` can be added or removed without claiming either issue.

Parent–child relationships (`parent_of`, `child_of`) are the exception — they require a claim on the child issue, because they modify the child's parent field.

### Stale claims

Every claim has a stale time (default: 2 hours after claiming; maximum: 24 hours). Set via `--duration` or `--stale-at` when claiming. Once the stale time passes, the claim is considered stale and can be stolen by another agent.

### Stealing

When a claim goes stale, any agent can steal it. Stealing atomically invalidates the old claim and creates a new one. If two agents race to steal, exactly one succeeds; the other receives a claim-conflict error.

You can steal explicitly (`np claim <ID> --steal`) or let the system do it when no ready issues are available (`np claim ready --steal`). Stealing auto-generates an audit comment noting who stole from whom.

### Design rationale

**Why claiming?** When multiple agents work in the same workspace, they need a way to avoid stepping on each other's work. Claiming provides a lightweight, cooperative lock.

**Why bearer auth?** It is the simplest model that works for AI agents. A claim ID is a single value that can be stored in a variable — no cookies, no OAuth, no token refresh.

**Why stale claims?** Without them, a crashed agent or forgotten claim would permanently lock an issue. Staleness is the safety valve.

**Why are comments and relationships ungated?** Comments are observational; they do not modify state. Relationships express connections between issues. Gating either behind claiming would force unnecessary coordination.

---

## Readiness

Readiness answers the question: "What can I work on right now?" It is a computed property, not a state — `np claim ready` uses it to pick the next issue.

**Quick rules:**

- A **task** is ready when it is `open`, has no unresolved blockers, and no ancestor epic is deferred.
- An **epic** is ready when it is `open`, has **no children** (needs decomposition), has no unresolved blockers, and no ancestor epic is deferred.
- Deferring an epic suppresses readiness for all its unclaimed descendants.

### Task readiness (detailed)

A task is **ready** when all of the following are true:

1. Its state is `open` (not claimed, closed, or deferred).
2. It has no unresolved blockers — either it has no `blocked_by` relationships, or every blocker has been closed or completed.
3. No ancestor epic is `deferred`.

### Epic readiness (detailed)

An epic is **ready** when all of the following are true:

1. Its state is `open`.
2. It has **no children** — it needs to be decomposed into tasks or sub-epics before progress can be made.
3. It has no unresolved blockers.
4. No ancestor epic is `deferred`.

An epic that already has children is *not* ready — its work is defined; progress comes from completing those children. An empty epic represents a planning gap: someone needs to claim it and break it down.

### Deferred ancestors suppress readiness

Deferring an epic drops all of its unclaimed descendants out of the ready pool. This is how you shelve an entire initiative with one action. Already-claimed descendants are unaffected — the agent working on them can finish.

### Design rationale

Readiness drives `np claim ready`, the main loop for AI agent workflows: claim, work, close, repeat. Without a clear readiness definition, agents would waste time picking up issues they cannot progress. Propagating deferral downward eliminates the need to individually defer every descendant.

---

## Relationships

Issues can be connected to each other through relationships. Relationships express dependencies and informational links between issues — they are separate from the parent–child hierarchy.

### blocked_by / blocks

A directional dependency: "this issue cannot progress until the other issue is closed or completed."

- `blocked_by` and `blocks` are two views of the same relationship. Adding `A blocked_by B` is equivalent to adding `B blocks A`.
- Blocking relationships directly affect readiness — an issue with unresolved blockers is not ready.
- Circular blocking chains are not prevented at creation time. They are detected by `np rel cycles` and the `np admin doctor` command. This is a pragmatic choice: preventing cycles would require a graph traversal on every insert, which is disproportionate to the risk. Cycles are rare and easily resolved once detected.

### refs

A bidirectional informational link: "these two issues are related, but neither blocks the other."

- `refs` relationships are symmetric — adding `A refs B` makes both issues show the reference.
- They do not affect readiness or state transitions. They exist purely for context — when working on one issue, you can see what other issues are relevant.

### No claim required

Relationships can be added or removed by anyone without claiming either issue. This is deliberate — relationships are observations about how issues relate, not modifications to the issues themselves.

The exception is parent–child relationships (`parent_of`, `child_of`), which require a claim on the child issue because they modify the child's parent field.

**Why this design?** If adding a `blocked_by` required claiming, an agent that discovers a dependency while working on issue A would need to also claim issue B — or worse, wait for B's claimer to add the relationship. Ungated relationship management keeps the system responsive.

---

## Labels

Labels are key–value pairs attached to issues for filtering, categorization, and agent coordination.

### Structure

A label has a **key** (1–64 ASCII printable characters, no whitespace) and a **value** (1–256 UTF-8 characters, no whitespace). Keys are unique per issue — setting an existing key overwrites its previous value.

### Conventions

Labels are freeform; nitpicking does not enforce a vocabulary. However, some conventions have emerged:

- **`kind:`** — categorizes the type of work. Recommended values follow the Conventional Commits vocabulary: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `style`, `build`, `ci`, `chore`.
- **`docs:`** — identifies documentation-related metadata (e.g., `docs:section`).

You can define your own keys and values for whatever categorization your project needs.

### Propagation

Labels can be propagated from a parent issue to all of its descendants using `np label propagate`. This is useful for tagging an entire initiative — set the label on the top-level epic and propagate it downward.

### Requiring a claim

Adding, modifying, or removing labels requires claiming the issue. This is consistent with the general rule: any field mutation is gated by claiming.

**Why key–value pairs instead of tags?** Tags (single values without keys) cannot express structured relationships between categories and values. Key–value pairs let you filter on both the category (`kind:`) and the specific value (`feat`), supporting queries like "all feature tasks" (`kind:feat`) or "all issues with a kind label" (`kind:*`).

---

## Priorities

Every issue has a priority from P0 (most urgent) to P4 (least urgent). Priority defaults to P2 if not specified.

| Level | Meaning | When to use |
|-------|---------|-------------|
| P0    | Critical | Security vulnerabilities, data loss, broken builds — anything that demands immediate attention. |
| P1    | High | Major features, important bugs, work that unblocks other high-priority items. |
| P2    | Medium | The default. Standard work that should be done in normal course. |
| P3    | Low | Polish, optimization, minor improvements — valuable but not urgent. |
| P4    | Backlog | Future ideas, speculative work, things to revisit later. |

`np claim ready` uses priority as the primary sort key — it always claims the highest-priority (lowest P-number) ready issue. This means P0 issues jump the queue automatically.

**Why five levels?** Fewer than five makes it hard to distinguish urgency. More than five creates decision paralysis — "Is this a P3 or a P4?" Five levels provide enough granularity for practical prioritization without overthinking.

---

## Comments

Comments are annotations on issues — observations, reasoning, status updates, or anything else that provides context for future readers.

### No claim required

Anyone can add a comment to any issue at any time. This includes closed issues — closure prevents state changes and field updates, but not commentary. A post-mortem note on a closed bug, for example, is perfectly valid.

### Audit trail

Comments serve as the informal audit trail alongside the structured history. While history records *what* changed (field deltas, state transitions), comments record *why* — the reasoning, trade-offs, and context that motivated the changes.

**Why ungated?** Gating comments behind claiming would make it impossible for one agent to leave observations about another agent's work-in-progress. It would also prevent post-closure documentation. Comments are read-append; they do not modify the issue, so there is no concurrency risk.

---

## Issue IDs

Every issue has a unique ID in the format `<PREFIX>-<random>` — for example, `PKHP-a3bxr`.

### Prefix

The prefix is set once at database initialization (`np init --prefix NP`) and applies to all issues in that database. It is uppercase ASCII letters only, 1–10 characters. The prefix scopes IDs to the workspace, making them unambiguous when discussed across workspaces.

### Random portion

The random portion is 5 lowercase Crockford Base32 characters, giving an ID space of roughly 33.5 million. Crockford Base32 excludes visually ambiguous characters (I, L, O, U), making IDs easier to read and communicate verbally.

The uppercase prefix and lowercase random portion create visual contrast — you can immediately see where the prefix ends and the unique portion begins.

### Database discovery

When you run any `np` command, it locates the database by walking up from your current working directory, looking for a `.np/` directory. The search proceeds parent by parent until it finds one or reaches the filesystem root. This means you can run `np` from any subdirectory within your workspace and it will find the right database.

**Why random IDs instead of sequential?** Sequential IDs leak information about creation order and total issue count. Random IDs are opaque — they do not encode ordering, and they avoid conflicts when issues are created concurrently by multiple agents.
