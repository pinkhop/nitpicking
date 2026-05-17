---
name: np-reading-issues
description: Use when the agent needs to understand a specific `np` (nitpicking) issue (typically right after claiming it), explore its parent, siblings, or `refs` neighbours for context, view an issue's history, free-text search across issues, or list/filter issues by label, state, role, or parent. Triggers on prompts like "what does FOO-a3bxr cover", "what do I need to do for this issue", "do we already have an issue for X", "show me the history of FOO-a3bxr", "search issues for 'login timeout'", "list open epics", "list issues with label kind:bug". Not for ready-queue filtering (use `np-finding-work`) or inspecting the label vocabulary itself (use `np-labeling`).
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np epic children:*) Bash(np issue history:*) Bash(np issue search:*) Bash(np list:*) Bash(np rel:*) Bash(np show:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-reading-issues

## Overview

`np` exposes three reading activities, in priority order:

1. **Understand a specific issue** — typically after claiming it, to know what to work on.
2. **Discover the issue's key relationships** — parent, siblings, and `refs` neighbours often carry context the issue body assumes.
3. **Find arbitrary issues** by text or label — duplicate checks, "do we already have an issue covering X", workspace surveys.

All read commands work without a claim.

## Understanding a specific issue

`np show <ID>` is the workhorse. It returns full detail: state, readiness, priority, labels, parent, relationships, comments, and the audit-relevant fields.

```bash
$ np show FOO-a3bxr
$ np show FOO-a3bxr --json        # for machine parsing
```

When the issue's evolution matters — for example, when reasoning about decisions captured in earlier comments — read its history:

```bash
$ np issue history FOO-a3bxr
```

Together, `np show` and `np issue history` are usually sufficient for "what do I need to do for this issue".

## Exploring an issue's key relationships

The issue alone often does not carry enough context. Three readings around it are usually worth doing:

**Parent.** If `np show <ID>` reports a parent, read it for the broader goal:

```bash
$ np show <PARENT-ID>
```

**Siblings.** Other children of the same parent often share assumptions, ordering, or vocabulary:

```bash
$ np rel parent children <PARENT-ID>    # direct children of any parent (epic or task)
$ np rel parent tree <PARENT-ID>        # full descendant hierarchy
$ np epic children <PARENT-ID>          # equivalent when the parent is known to be an epic
```

Prefer `np rel parent children` in general — it works regardless of whether the parent is an epic or a task. `np epic children` is the older, epic-specific form.

**`refs` neighbours.** `refs` is the informational link `np` uses for "see also" relationships. The issue may point at related-but-not-blocking work that explains intent:

```bash
$ np rel issue <ID>                        # all relationships for this issue (includes a refs section)
```

Decide which of the three to read based on what the issue's text leaves unexplained. Reading the parent matters more for child tasks of a focused epic; reading siblings matters more when ordering or shared design is implied; reading `refs` matters when the issue cites a precedent.

## Finding arbitrary issues

When the goal is not "understand this issue" but "find issues about X" — for duplicate checks, surveys, or answering "do we have an issue for this?":

```bash
$ np issue search "login timeout"
$ np issue search "timeout" --label area:api
$ np list --label kind:bug --label area:auth
$ np list --state deferred --label defer-reason:unclear
$ np list --role epic --state open
```

`np list` accepts `--role`, `--state`, `--parent`, and repeated `--label` flags (AND-combined; `key:*` matches any value for a key). `--columns` and `--order` control table output:

```bash
$ np list --columns ID,PRIORITY,TITLE
$ np list --order MODIFIED:desc
```

Prefer `--json` whenever a script will parse the result:

```bash
$ np list --label kind:bug --json
$ np show <ID> --json
```

## What this skill does not cover

- **Claiming work** — use `np-finding-work`. This skill is read-only.
- **Modifying labels** to make filtering work better — use `np-labeling`.
- **Adding relationships** discovered during reading — use `np-managing-relationships`.
- **Diagnosing why nothing is ready** — out of scope; the agent's stance is "either work is ready or it isn't".

## Common mistakes

- **Stopping at `np show <ID>` when the issue's body is thin.** Many tasks assume parent or sibling context. Spend a minute reading the parent and at least one sibling before working.
- **Searching with `np list` and forgetting `np issue search`.** `np list` filters by structured fields and labels; `np issue search` is the free-text path. Use both when looking for prior work.
- **Calling read commands with `--author` flags.** Reads do not take `--author` and do not require a claim.
