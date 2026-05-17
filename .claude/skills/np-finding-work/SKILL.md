---
name: np-finding-work
description: Use when the agent needs to pick up the next ready issue from the `np` (nitpicking) tracker, claim a specific issue by ID, browse the ready queue, or filter ready work by label or role (e.g., "claim the next ready bug", "pick up the next task in area:auth", "claim FOO-a3bxr"). Does not cover reading the claimed issue's content or finishing work.
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np agent name:*) Bash(np claim:*) Bash(np close:*) Bash(np issue defer:*) Bash(np issue release:*) Bash(np json update:*) Bash(np ready:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-finding-work

## Overview

`np` gates all mutation behind a claim. Finding work means: identify a ready issue, claim it atomically, and capture the returned claim ID. The claim ID is the bearer credential for every subsequent mutation on that issue.

## Author identity

Every `np` mutation requires `--author <name>`. If no name has been chosen for this session, generate a stable one:

```bash
$ np agent name --seed=$PPID
agent-blue-seal-echo
```

Reuse that name for the rest of the session. Seeding with `$PPID` keeps the name stable across restarts of the same shell.

## Browsing the ready queue

```bash
$ np ready                                  # all ready issues
$ np ready --role task                      # only ready tasks
$ np ready --label kind:bug                 # only ready bugs
$ np ready --label area:auth --role task    # AND-combined filters
$ np ready --parent FOO-c4npt               # only ready children of FOO-c4npt
```

`--label` accepts `key:value` or `key:*` (any value for that key). Multiple `--label` flags AND together. There is no OR.

## Claiming the next ready issue

This is the standard work-pickup mechanism. It claims atomically — two agents racing for the same ready issue cannot both win.

```bash
$ np claim ready --author <your-name>
$ np claim ready --author <your-name> --label kind:bug
$ np claim ready --author <your-name> --role task --label skill:go
```

Optional staleness controls (mutually exclusive):

```bash
$ np claim ready --author <your-name> --duration 4h
$ np claim ready --author <your-name> --stale-at 2026-04-28T18:00:00Z
```

`--duration` defaults to 2h, max 24h. `--stale-at` is RFC3339 UTC.

Prefer JSON when another tool will parse the result:

```bash
$ np claim ready --author <your-name> --json
```

## Claiming a specific issue by ID

When the issue ID is already known:

```bash
$ np claim FOO-a3bxr --author <your-name>
```

If exit code `3` comes back, someone else holds the claim. Either claim something else, wait for staleness, or ask the user.

## Capturing the claim ID

`np claim` returns a claim ID. **Save it immediately and preserve it across context compactions.** Every later mutation on this issue needs it.

- Treat the claim ID as a bearer credential — anyone holding it can act on the claim.
- Never write it into a comment, commit message, log, or any shared location.
- If the claim ID is lost, the claim is effectively abandoned; it will eventually go stale and another agent can pick it up.

## Stale claims

If `np claim ready` reports no ready issues but stale claims exist, running it again normally will reclaim them — `np` overwrites stale claims atomically. No special flag is needed.

## What this skill does not cover

- **Reading the claimed issue's content** — use `np-reading-issues`. After claiming, read the issue (and often its parent, siblings, and `refs`) to understand what to do.
- **Finishing the work** — use `np-finishing-work` for `np close`, `np issue release`, or `np issue defer`.
- **Modifying the issue's fields** — use `np-help-discipline` to find `np json update --help`.
- **Initializing the workspace** — never run `np init` without explicit user direction.

## Common mistakes

- **Forgetting `--author`.** Mutation without `--author` fails. Generate a name once at session start and reuse it.
- **Browsing instead of claiming.** Do not cherry-pick by ID from `np ready` output unless explicitly directed; use `np claim ready` so claims are atomic.
- **Letting the claim ID drop out of working memory.** Re-record it any time the conversation gets compacted or the agent loses focus.
