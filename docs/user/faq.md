# Frequently Asked Questions

---

## General

**What is np?**
A local-only, CLI-driven issue tracker designed for AI agent workflows. It stores issues in an embedded SQLite database — no network, no server, no background processes. See the [Getting Started guide](getting-started.md).

**How does np differ from beads?**
Beads installs global hooks, ties into the git lifecycle, requires a database server, and supports multi-machine agent collaboration. `np` deliberately avoids all of these — it is non-invasive, local-only, and focused solely on issue tracking. See the [design philosophy](key-concepts.md#design-philosophy) for the reasoning behind each of these decisions.

**Why local-only? Why no network sync?**
Simplicity and reliability. Local-only means no authentication, no API keys, no rate limits, no latency, and no dependency on external services. For single-machine development, there is nothing to sync. See the [Getting Started guide](getting-started.md) and [design philosophy](key-concepts.md#design-philosophy) to decide whether that tradeoff fits your workflow.

**Why SQLite instead of a database server?**
SQLite is embedded — no separate process to manage, no configuration, no ports to open. The database is a single file in `.np/`. It handles concurrent reads well and supports WAL mode for concurrent writes. See [embedded storage](key-concepts.md#embedded-storage--no-database-server) in key concepts.

---

## Setup

**Where does the database live?**
In a `.np/` directory. You choose where by running `np init` — typically at the workspace root. `np` discovers the database by walking up from your current working directory. See `np admin where`.

**Can I have multiple databases?**
Yes. Each `np init` creates a separate database. You might have one per workspace, or one parent database spanning multiple repos — it depends on where you run `np init`.

**How do I move a database?**
Move the `.np/` directory to the new location. `np` does not store the database path anywhere — it discovers it at runtime by walking up from the current directory.

**Should I commit `.np/` to version control?**
Generally no — add `.np/` to your `.gitignore`. The database is local state, not source code. If multiple developers or agents need to share issues, they share a machine (or a shared filesystem), not a git-tracked database.

---

## Workflow

**Why do I need to claim before editing?**
Claiming is `np`'s concurrency gate. It prevents two agents from modifying the same issue simultaneously, which would cause one agent's changes to overwrite the other's. See [Claiming](key-concepts.md#claiming) in key concepts and [Multi-Agent Coordination](workflow-multi-agent.md) for the workflow.

Even if you are the only human using `np`, claiming is still useful: it marks the issue as actively in progress and forces a clean finish through either `np close` or `np issue release`.

**What happens if I forget to close an issue?**
The claim remains active until the stale time expires (default 2 hours after claiming). During that time, no other agent can claim the issue. After the stale time, the claim is treated as nonexistent and any agent can claim the issue normally — no special flag is needed. Use `np admin doctor` to find stale claims. See [stale claims](key-concepts.md#stale-claims) and [Troubleshooting: Stale Claims](troubleshooting.md#stale-claims).

For solo use, the practical problem is different but still real: the next time you run `np ready`, the issue will be missing because it is still claimed. Release paused work explicitly so the queue stays truthful.

**Can I work on multiple issues at once?**
Yes — claim multiple issues, each with its own claim ID. Keep track of all claim IDs. However, releasing claims promptly is important to avoid blocking other agents. See the [Daily Use Guide](daily-use.md) for the everyday claim workflow.

---

## Epics

**Why can't I close an epic directly?**
An epic's completion is determined by the `completed` secondary state. It is "complete" when all children are closed or complete. Use `np epic close-completed` to close epics whose children are all resolved. This design prevents accidentally closing an epic with unfinished children. See [Epic-Driven Development](workflow-epics.md) for the full lifecycle and [Issue Roles](key-concepts.md#issue-roles) for the role model.

**What's the 3-layer limit?**
The parent-child hierarchy is limited to 3 levels: epic → epic → task. This prevents overly deep nesting that makes progress tracking confusing. If you need more structure, use `blocked_by` relationships between sibling issues instead. See [hierarchy and depth](key-concepts.md#hierarchy-and-depth) in key concepts.

**Are tasks supposed to have children?**
No in the recommended user model. Think of tasks as leaf work items and epics as the issues that hold children. Some lower-level commands manipulate the parent field generically, but the documentation's intended planning pattern is still epic → task or epic → epic → task.

**When should I use child epics vs. child tasks?**
Use a child epic when a sub-feature is large enough to need its own decomposition. Use child tasks for individual pieces of work. A good heuristic: if a child would have more than 5–7 tasks, it probably deserves to be a child epic.

---

## Relationships

**What's the difference between `blocked_by` and `refs`?**
`blocked_by` is directional and gates readiness — a blocked issue will not appear in `np ready` until its blocker is closed. `refs` is bidirectional and purely informational — it links related issues without affecting readiness. See the [Command Reference](command-reference.md#rel-add).

**Are `refs` relationships bidirectional?**
Yes. Adding `A refs B` is equivalent to `B refs A`. Either issue will show the other in its relationship list.

**Can I create cycles?**
`np` prevents invalid parent-child structures such as cycles and over-deep nesting at creation time, but blocking cycles are handled diagnostically rather than rejected on insert. Use `np rel cycles` and `np admin doctor` to detect circular blocking chains. See [Relationships](key-concepts.md#relationships) in key concepts for the design reasoning.

---

## Labels

**What label conventions should I use?**
Common conventions: `kind:` for work type (bug, feature, docs), `area:` for codebase area (api, frontend, auth), `scope:` for module (claim, issue, rel). See [Label-Driven Issue Selection](workflow-labels.md).

**How is label propagation useful?**
When you label an epic (e.g., `kind:feature`) and want all its children to share that label, `np label propagate kind --issue <epic-id>` copies it to every descendant in one command. This avoids labeling each child individually. See the [label propagation command](command-reference.md#label-propagate) for syntax.

---

## Agents

**How do I set up np for my AI assistant?**
Run `np agent prime` to generate workflow instructions, then provide the output to your agent at session start. Add a brief reference to `np` in your agent's static instruction file (CLAUDE.md, .cursorrules, etc.). See the [Agent Integration Guide](agent-integration.md).

**What does `--json` do?**
It switches command output from human-readable text to structured JSON. Agents should always use `--json` because the format is stable, parseable, and includes all fields. Human-readable output may change between versions.

**How do agents avoid stepping on each other?**
Through claiming. Each agent uses `np claim ready` to atomically find and claim the next available issue. Two agents running this simultaneously will each get a different issue. See [Multi-Agent Coordination](workflow-multi-agent.md).
