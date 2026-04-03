# Agent Integration Guide

How to configure AI coding assistants to use `np` as their issue tracker.

---

## Why np Is Designed for Agents

`np` was built with AI agent workflows as a first-class concern:

- **Local-only** — no network, no API keys, no rate limits. Agents run `np` as a local binary.
- **Structured I/O** — commands that return human-readable output usually support `--json`, and the `json` command group provides machine-oriented stdin/stdout flows for create, update, and comment operations.
- **Exit codes** — predictable exit codes (0 success, 2 not found, 3 claim conflict, 4 validation error, 5 database error) enable branching workflow logic.
- **Claim-based mutual exclusion** — multiple agents can work concurrently without conflicting; claiming prevents two agents from modifying the same issue.
- **Atomic operations** — `np claim ready` finds and claims the highest-priority work in a single step, eliminating race conditions.
- **No state to manage** — no sessions, tokens, or connections. Each `np` command is a standalone invocation.

---

## General Setup Pattern

Every AI coding assistant has a configuration mechanism for project-level instructions. The pattern is the same regardless of the tool:

1. **Generate workflow instructions** with `np agent prime`. This outputs a comprehensive Markdown block covering every command and workflow pattern the agent needs.
2. **Provide the instructions to the agent** at session start. The output is too large for static instruction files; provide it dynamically.
3. **Add a brief reference** to your static instruction file so the agent knows `np` exists.

---

## Claude Code Setup

### Static Instructions (CLAUDE.md)

Add a brief reference to `np` in your `CLAUDE.md`:

```markdown
# np — Issue Tracker

np is the exclusive tool for task management in this project. Run
`np agent prime` at the start of each session for full workflow
instructions.
```

### Dynamic Instructions

At the start of each coding session, provide the full workflow instructions:

```
np agent prime
```

Re-provide whenever context is compacted or cleared — the instructions do not persist across context windows.

### Agent Name

Generate an agent name at session start:

```
np agent name
```

Use this name consistently for all `--author` flags throughout the session.

---

## OpenAI Codex Setup

Add `np` instructions to your Codex agent configuration or system prompt. The same pattern applies:

1. Add a brief mention of `np` to your agent's static instructions.
2. Provide `np agent prime` output at session start.
3. Generate an agent name with `np agent name`.

---

## Cursor Setup

Add `np` instructions to `.cursorrules` or your project-level Cursor instructions:

```markdown
# np — Issue Tracker

np is the exclusive tool for task management in this project. Run
`np agent prime` at the start of each session for full workflow
instructions.
```

Provide the full `np agent prime` output at the start of each Composer or Chat session.

---

## Key Patterns for Agents

### Always Use --json

Agents should always pass `--json` for machine-readable output:

```bash
np claim ready --author "$AUTHOR" --json
np show "$ISSUE_ID" --json
np list --ready --json
```

JSON output is stable and parseable; human-readable output may change between versions.

### Check Exit Codes

Branch workflow logic based on exit codes:

```bash
np claim ready --author "$AUTHOR" --json
case $? in
    0) echo "Claimed successfully" ;;
    2) echo "No ready issues found" ;;
    *) echo "Unexpected error" ;;
esac
```

| Code | Meaning | Agent action |
|------|---------|-------------|
| 0 | Success | Proceed. |
| 2 | Not found | No matching issues; stop or wait. |
| 3 | Claim conflict | The issue is already claimed; try another or wait for the stale duration. |
| 4 | Validation error | Check inputs; likely a missing required field. |
| 5 | Database error | Report to the user; likely a corrupted database. |

### Handle Claim Conflicts Gracefully

When `np claim <ID>` returns exit code 3, the issue is already claimed by another agent. Options:

- **Try another issue** — use `np claim ready` to claim the next available.
- **Wait** — the claim will expire after the stale duration (default 2 hours).
- **Steal** — if the other agent is gone, use `--steal` (works for both `np claim <ID>` and `np claim ready`).

### Always Close When Done

Abandoned claims block other agents for up to 2 hours. Always close with `np close` or release with `np issue release` when you are finished.

### Document Work with Comments

Add comments before closing to create an audit trail:

```bash
np json comment "$ISSUE_ID" --author "$AUTHOR" <<'JSONEND'
{
  "body": "Approach: ..."
}
JSONEND
np close --claim "$CLAIM_ID" --reason "Completed: ..."
```

---

## Example Agent Instruction Block

A minimal instruction block you can adapt for any AI coding assistant:

```markdown
## Task Management

This project uses `np` for issue tracking. It is the exclusive tool for
task management — do not use any other task-tracking mechanism.

### Setup

1. Generate your agent name: `np agent name`
2. Use this name for all --author flags in this session.

### Workflow

1. Find and claim work: `np claim ready --author <name> --json`
2. Review the issue: `np show <issue-id> --json`
3. Do the work.
4. Add a comment: `np json comment <id> --author <name>`
5. Close the issue: `np close --claim <claim-id> --reason "..."`
6. Repeat from step 1.

### Key Rules

- Always use --json for machine-readable output.
- Always close or release claims when done.
- Check exit codes: 0 = success, 2 = not found, 3 = claim conflict.
- Epics: decompose into child tasks, then release the claim.
```

---

## Troubleshooting Agent Issues

### Lost Claim ID

If an agent loses its claim ID mid-session (e.g., due to context compaction), the claim ID cannot be recovered from command output — it is a bearer token that is only revealed at claim time. The agent must wait for the stale duration (default 2 hours) to expire and then reclaim or steal the issue:

```bash
np claim <ISSUE-ID> --author <name> --steal
```

To check when the claim becomes stealable, inspect the `claim_stale_at` field:

```bash
np show <ISSUE-ID> --json | jq '.claim_stale_at'
```

### Claim Abandonment

If an agent crashes or is terminated without closing its claim, the issue remains claimed until the stale duration expires. Another agent can then steal it:

```bash
np claim <ISSUE-ID> --author <name> --steal
```

Or use `np admin doctor` to identify stale claims across the database.

### Context Window Limits

`np agent prime` output is large. If an agent's context window is small:

- Provide only the sections relevant to the current task.
- Use the brief instruction block above instead of the full `np agent prime` output.
- Re-provide instructions after context compaction.
