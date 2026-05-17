---
name: np-commenting
description: Use when the agent needs to add a comment to an `np` (nitpicking) issue — to record reasoning, capture an investigation finding, leave a handoff note, document a trade-off considered, or annotate any issue (including closed ones). Triggers on prompts like "add a comment to FOO-a3bxr saying ...", "leave a note on the issue", "record that we tried X", "comment on FOO-12345 that ...". Comments do not require a claim.
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np agent name:*) Bash(np close:*) Bash(np json comment:*) Bash(np json update:*) Bash(np show:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-commenting

## Overview

Comments are the audit trail. They capture context that code and commit history cannot — reasoning, trade-offs considered, dead ends explored, handoff notes for the next reader. **Comments do not require a claim** and can be added to any issue, including closed ones.

## Author identity

Commenting still requires `--author <name>`. If no name has been chosen for this session, generate one:

```bash
$ np agent name --seed=$PPID
agent-blue-seal-echo
```

Reuse that name for the rest of the session.

## Adding a comment

Use the JSON entry point — agents prefer it over the interactive form:

```bash
$ np json comment FOO-a3bxr --author agent-blue-seal-echo <<'JSONEND'
{
  "body": "Approach taken: routed all session lookups through the new cache layer; benchmarked at 18% lower p99."
}
JSONEND
```

The positional argument is the issue ID. The body is Markdown — paragraphs, lists, code fences are all fine.

## When to comment

| Situation | What to capture |
|---|---|
| Investigation finding | What was discovered and where (file paths, line numbers) |
| Decision or trade-off | The options considered, what was chosen, and why |
| Handoff to a future reader | What is done, what is left, where to pick up |
| Observation on a related issue | Anything an agent claiming the related issue should know |
| Pre-close summary | What changed and what was verified (if more than `np close --reason` will hold) |

Write for someone who has not seen the conversation. Avoid restating the issue title; lead with the substance.

## Commenting on issues without claiming

Comments require no claim, so the agent can leave context anywhere — a closed issue, an issue claimed by someone else, a deferred issue. This is the right tool when the user says "leave a note on FOO-12345" without that being a state transition.

```bash
$ np json comment FOO-12345 --author agent-blue-seal-echo <<'JSONEND'
{
  "body": "While working FOO-67890, noticed that the retry helper in `internal/retry` ignores context cancellation. Likely related."
}
JSONEND
```

## Never include the claim ID

If the agent currently holds a claim on the issue, **do not paste the claim ID into the comment body**. The claim ID is a bearer credential. Comments are readable to anyone with access to the workspace.

## What this skill does not cover

- **Closing the issue with a reason** — use `np-finishing-work`. `np close --reason` already records the reason as a closing comment, so a separate `np-commenting` step is only needed when more than the reason is worth capturing.
- **Updating fields like title or description** — escape-hatch via `np-help-discipline` for `np json update --help`.
- **Reading prior comments before adding one** — use `np-reading-issues` (`np show <ID>` includes the comment history).

## Common mistakes

- **Skipping `--author`.** Comments still require the author flag.
- **Pasting the claim ID into the body.** Never. The body is shared; the claim ID is private.
- **Commenting when a closing reason would suffice.** If the agent is about to close the issue anyway, prefer `np close --reason "..."` — it records the same content with one fewer command.
