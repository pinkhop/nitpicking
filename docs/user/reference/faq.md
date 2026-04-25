# Frequently Asked Questions

## General

**What is `np`?**  
A local-only, CLI-driven issue tracker for solo development and AI agent workflows. Start with [Quickstart](../quickstart.md).

**Why local-only?**  
To keep setup, operation, and trust boundaries simple. There is no network service, no auth layer, and no remote dependency.

**Why SQLite?**  
Because it gives `np` a real database without requiring a database server.

## Setup

**Where does the database live?**  
Inside `.np/` in the workspace where you ran `np init`.

**Can I have more than one database?**  
Yes. Each initialized workspace has its own database.

**Should I commit `.np/`?**  
Usually no. Treat it as local state unless you have a specific reason to move it with the repo.

## Workflow

**Do I need to claim issues even when working alone?**  
Yes, if you want the tracker to reflect what is actively in progress and what is ready.

**What happens if I forget to close or release a claim?**  
The claim remains active until it goes stale, then another normal claim can overwrite it.

**Can I work on multiple issues at once?**  
Yes, but it is easy to create a messy queue that way. Most teams should prefer one active claim per agent unless there is a clear reason otherwise.

## Epics

**When should I introduce epics?**  
When flat tasks plus labels stop being expressive enough and you need explicit hierarchy, progress tracking, or grouped closure. See [Epics](../epics.md).

**Why can’t I close an epic directly?**  
Because epic completion is derived from child completion. That is the point of the model.

**What is the depth limit?**  
Three levels: epic -> epic or task -> task. See [Core Concepts](core-concepts.md).

## Relationships

**What is the difference between `blocked_by` and `refs`?**  
`blocked_by` affects readiness. `refs` is informational only.

**Are blocking cycles prevented?**  
They are handled diagnostically rather than blocked on insert. Use `np admin doctor` to find them.

## Labels

**Do I need labels to start?**  
No. They are intentionally optional. Read [Labels](../labels.md) only when filtering and routing pressure appears.

**What label scheme should I start with?**  
Usually `kind` and `area`, and nothing more at first. Add `task-group:<name>` when related tasks need lightweight grouping without hierarchy.

## Agents

**How do I configure an AI assistant for `np`?**  
Use [Agent Setup](../agents/setup.md).

**How do multiple agents avoid stepping on each other?**  
Use `np claim ready` and honor claims. See [Multi-Agent Operations](../agents/multi-agent.md).
