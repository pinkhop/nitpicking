# Agent Setup

This guide is about wiring `np` into an AI coding assistant. It is not the operational guide for concurrent agents; that lives in [Multi-Agent Operations](multi-agent.md).

## What Agents Need From `np`

`np` works well for agents because it is:

- local-only
- CLI-driven
- scriptable with `--json`
- explicit about concurrency through claims
- stable enough to use from shell loops

## General Pattern

Regardless of the assistant:

1. Mention `np` in the tool's static project instructions.
2. Provide fresh `np agent prime` output at session start.
3. Generate a stable session author name.
4. Use `--json` whenever another program will parse the output.

## Session Start

Generate the instructions:

```bash
$ np agent prime
```

Generate a stable agent name:

```bash
$ np agent name --seed=$PPID
blue-seal-echo
```

Use that name consistently for `--author` during the session.

## Minimal Static Instruction Block

Put something this small in `CLAUDE.md`, Codex project instructions, Cursor rules, or equivalent:

```markdown
## np

This project uses `np` as its issue tracker.
Run `np agent prime` at session start for the operational rules.
Use `np` instead of ad hoc task tracking.
```

## Claude Code

- Add the static instruction block to `CLAUDE.md`.
- Provide `np agent prime` output at the start of each session.
- Re-provide it after context compaction.

## OpenAI Codex

- Add the static instruction block to the project instructions or system prompt.
- Provide `np agent prime` output at session start.
- Generate the author name with `np agent name --seed=$PPID`.

## Cursor

- Add the static instruction block to `.cursorrules` or project instructions.
- Provide `np agent prime` output at the start of each chat or composer session.

## Operating Rules For Agents

- Prefer `--json` output.
- Treat exit codes as workflow signals, not incidental details.
- Claim before mutating.
- Comment before closing when the work involved a decision or investigation.
- Close or release claims when done.

Example:

```bash
$ np claim ready --author "$AUTHOR" --json
$ np show "$ISSUE_ID" --json
$ np json comment "$ISSUE_ID" --author "$AUTHOR" <<'JSONEND'
{
  "body": "Approach: ..."
}
JSONEND
$ np close --claim "$CLAIM_ID" --reason "Completed: ..."
```

## What To Read Next

- Read [Multi-Agent Operations](multi-agent.md) if several agents will share one workspace.
- Read [Core Concepts](../reference/core-concepts.md) if the agent needs more context on claims, readiness, and relationships.
