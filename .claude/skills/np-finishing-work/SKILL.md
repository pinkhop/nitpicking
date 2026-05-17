---
name: np-finishing-work
description: Use when the agent has a claimed `np` (nitpicking) issue and needs to close it as complete, release the claim without changing the issue's state, or defer the issue for later. Triggers on prompts like "close the issue", "mark it done", "release the claim", "defer this for now". Does not cover reopening or undeferring (escape-hatch via `np-help-discipline`); commenting is handled separately by `np-commenting`.
license: MIT
compatibility: Requires the nitpicking `np` CLI (>= 0.4.0) on PATH; no network access needed.
allowed-tools: Bash(np claim:*) Bash(np close:*) Bash(np epic close-completed:*) Bash(np issue defer:*) Bash(np issue release:*) Bash(np issue reopen:*) Bash(np issue undefer:*)
metadata:
  author: nitpicking (np)
  version: "0.4.0"
---

# np-finishing-work

## Overview

A claimed `np` issue must end in one of three transitions: closed, released, or deferred. Abandoning a claim blocks other agents until it goes stale, so always transition explicitly when work stops.

## Prerequisites

- The agent holds a valid claim ID for the issue (returned by `np claim`).
- The claim ID has been preserved across any conversation compaction.

The claim ID is a bearer credential. Never write it into the closing reason, a comment, a commit message, or any shared location.

## The three transitions

| Transition | Command | When to use |
|---|---|---|
| **Close** | `np close --claim <CID> --reason "..."` | The work is complete. The issue can be reopened later if needed. |
| **Release** | `np issue release --claim <CID>` | The claim should drop without changing the issue's state — e.g., an epic was decomposed; the agent paused; the work cannot be completed now. |
| **Defer** | `np issue defer --claim <CID>` | Shelve the issue for later. It will not appear as ready work until explicitly undeferred. |

## Closing with a reason

```bash
$ np close \
    --claim 5rvb5d3dhbx9081bmzcc5nccd8 \
    --reason "Implemented the login endpoint and verified with the new contract tests."
Closed FOO-a3bxr
```

`np close --reason` (short form `-r`) records the reason as a closing comment and closes the issue in one step. Write the reason for a future reader who has not seen the conversation: what changed, what was verified, and any non-obvious caveat.

## Releasing without state change

```bash
$ np issue release --claim 5rvb5d3dhbx9081bmzcc5nccd8
Released FOO-a3bxr
```

Release is the right move when:
- An epic has just been decomposed into children — the epic itself stays open.
- The agent picked up a task but cannot finish it now and wants it returned to the ready queue.
- The work was misclaimed and should be available to another agent immediately.

## Deferring for later

```bash
$ np issue defer --claim 5rvb5d3dhbx9081bmzcc5nccd8
Deferred FOO-a3bxr
```

Defer hides the issue from the ready queue until someone undefers it. Use it for work that should resume later, not for work that should be picked up by someone else.

## What this skill does not cover

- **Leaving an implementation note before transition.** That is a separate decision. When desired, invoke `np-commenting` first to record the note, then return here for the transition. `np close --reason` already produces a closing comment, so a separate note is optional unless the agent wants to capture more than the reason.
- **Reopening a closed issue (`np issue reopen`)** or **undeferring a deferred issue (`np issue undefer`)** — use `np-help-discipline` to look these up; they do not need a claim.
- **Batch-closing completed epics (`np epic close-completed`)** — escape-hatch via `np-help-discipline`.

## Common mistakes

- **Closing without a reason.** Always pass `--reason` so the closing comment captures why the work stopped.
- **Choosing release when defer was meant, or vice versa.** Release returns the issue to the ready queue immediately. Defer hides it. Pick deliberately.
- **Forgetting to transition at all.** A claim left open blocks ready work until it goes stale — usually two hours.
