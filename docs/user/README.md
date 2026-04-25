# np User Documentation

`np` is a local-only, single-machine CLI issue tracker for solo development and AI agent workflows. The docs below are organized as an adoption ladder: start with the flat task loop, wire in agents, learn the daily workflow, then add labels when you need filtering, routing, or lightweight grouping.

## Start Here

If you are new to `np`, read these in order:

1. [Quickstart](quickstart.md) - install `np`, initialize a workspace, create a task, claim it, comment on it, and close it.
2. [Agent Setup](agents/setup.md) - configure coding agents to use the same tracker and workflow as the human developer.
3. [Daily Work](daily-work.md) - the commands humans and agents repeat most often once the tracker becomes part of the workflow.
4. [Core Concepts](reference/core-concepts.md) - claims, readiness, issue roles, relationships, and priorities.

## Add Structure Later

Only read these when the simple task loop stops being enough:

- [Labels](labels.md) - when you need better filtering, routing, backlog hygiene, or flat grouping such as `task-group:<name>`.
- [Epics](epics.md) - when flat tasks plus labels are not enough and you need explicit hierarchy, progress tracking, or grouped closure.

## Agents

Use these when AI assistants are part of the workflow:

- [Multi-Agent Operations](agents/multi-agent.md) - how concurrent agents share one workspace safely.

## Reference

Use reference material when you need a rule, edge case, or exact command shape:

- [Command Reference](reference/command-reference.md) - every command, flag, exit code, and example.
- [Troubleshooting](reference/troubleshooting.md) - symptom-driven diagnosis and recovery steps.
- [FAQ](reference/faq.md) - common questions and short answers.

## Recommended Reading Paths

### Solo human, small project

`Quickstart` -> `Daily Work` -> `Core Concepts`

### Solo human or one agent, growing project

`Quickstart` -> `Agent Setup` -> `Daily Work` -> `Core Concepts` -> `Labels`

### Agent-heavy project

`Quickstart` -> `Agent Setup` -> `Daily Work` -> `Core Concepts` -> `Multi-Agent Operations`

### Considering hierarchy

`Quickstart` -> `Daily Work` -> `Labels` -> `Epics`

### Unsure whether `np` fits

Start with [Quickstart](quickstart.md), then skim the design constraints in [Core Concepts](reference/core-concepts.md).
