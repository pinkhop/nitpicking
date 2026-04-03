# np — User Documentation

`np` is a local-only, single-machine CLI issue tracker designed for AI agent workflows and solo development. No network, no remote sync, no background daemons — just a SQLite database in your workspace directory. Start with [Getting Started](getting-started.md) for the basic workflow and [Key Concepts](key-concepts.md) for fit, tradeoffs, and mental model.

Pick the path that matches where you are.

---

## New to np?

Start here to understand what `np` is, set it up, and complete your first task.

1. **[Getting Started](getting-started.md)** — Build from source, initialize a workspace, create an issue, and close it.
2. **[Key Concepts](key-concepts.md)** — The mental model: issue roles, state machines, claiming, readiness, relationships, labels, and priorities.

---

## Daily use

- **[Daily Use Guide](daily-use.md)** — The everyday commands you will repeat most often: finding work, updating issues, closing, deferring, unblocking, and diagnosing stuck work.

### Choosing a workflow

Start simple and add structure as your project grows. The workflows below build on each other — you do not have to pick one forever.

| Workflow | Best for | Signs you need it |
|----------|----------|-------------------|
| [Simple Task-Only](workflow-simple.md) | Small projects, focused sprints, solo work. | You have a flat list of things to do and no need for grouping. |
| [Epic-Driven](workflow-epics.md) | Features that decompose into multiple tasks. | You find yourself wanting "this set of tasks must all be done before the feature is complete." |
| [Label-Driven](workflow-labels.md) | Categorized work selection (bug vs. feature, area tags). | You want `np claim ready` to pick a specific *kind* of work, not just the next by priority. |
| [Multi-Agent](workflow-multi-agent.md) | Multiple AI agents or developers on one machine. | More than one agent is running and you need claim-based mutual exclusion. |

**When to switch:** If you started with simple tasks and now have 10+ issues that logically group into features, add epics. If agents are stepping on each other's claims, move to the multi-agent workflow. Labels can be layered onto any of the above at any time.

---

## Integrating np with AI agents

If you are configuring an AI coding assistant (Claude Code, Codex, Cursor, or similar) to use `np` as its issue tracker:

- **[Agent Integration Guide](agent-integration.md)** — Setup instructions, workflow patterns, and CLAUDE.md configuration.

---

## Reference and troubleshooting

- **[Command Reference](command-reference.md)** — Every `np` command: synopsis, flags, examples, exit codes, and notes.
- **[FAQ](faq.md)** — Common questions and answers.
- **[Troubleshooting](troubleshooting.md)** — Problems organized by symptom, with diagnostics and recovery steps.
