# Workflow: Multi-Agent Coordination

How multiple AI agents (or developers) work concurrently on issues from the same `np` database on a single machine.

---

## Why Claiming Exists

When two agents try to modify the same issue simultaneously, one will overwrite the other's changes. Claiming prevents this — it is mutual exclusion without file locks or database locks.

An agent must claim an issue before it can update fields or transition state. Only the holder of the claim ID can modify the issue. This guarantee holds as long as the claim has not expired.

---

## Each Agent Gets a Name

At session start, each agent generates a unique author name:

```
$ np agent name
blue-seal-echo
```

This name appears in history entries and comments, making it clear who did what. Agents should reuse the same name for the duration of their session.

---

## Atomic Find-and-Claim

The key command for multi-agent workflows is `np claim ready`:

```
$ np claim ready --author blue-seal-echo --json
{
  "issue_id": "MYAPP-2e22n",
  "claim_id": "a4dace30e46eb1ec14019c79a59c6b27",
  "stolen": false
}
```

This atomically finds the highest-priority ready issue and claims it in a single step. Two agents running this command simultaneously will each get a different issue — there is no race condition.

---

## Handling Claim Conflicts

When an agent tries to claim an issue that is already claimed, `np` returns exit code 3:

```
$ np claim MYAPP-2e22n --author kind-comet-quest
# Exit code 3: claim conflict
```

**What to do:**

- **Try another issue** — use `np claim ready` to claim the next available.
- **Wait** — the claim expires after the stale duration (default 2 hours).
- **Steal** — if the other agent is gone, use `--steal` (see below).

---

## Stale Claims and Stealing

Claims expire after a duration (default: 2 hours). If an agent crashes or is terminated without releasing its claim, the claim goes stale and other agents can steal it.

### Checking for Stale Claims

```
$ np admin doctor
```

The doctor diagnostic reports stale claims.

### Stealing a Stale Claim

```
$ np claim MYAPP-2e22n --author kind-comet-quest --steal
```

Or, when claiming the next ready issue:

```
$ np claim ready --author kind-comet-quest --steal
```

The `--steal` flag with `ready` first tries to find an unclaimed ready issue. Only if none exist does it fall back to stealing a stale claim.

### Adjusting the Duration

For long-running work, set a longer stale duration at claim time:

```
$ np claim MYAPP-2e22n --author blue-seal-echo --duration 4h
```

---

## Partitioning Work with Labels

Labels allow agents to specialize without explicit coordination:

```
# Agent A: bugs only
np claim ready --author agent-a --with-label kind:bug --json

# Agent B: features only
np claim ready --author agent-b --with-label kind:feature --json

# Agent C: documentation only
np claim ready --author agent-c --with-label kind:docs --json
```

Each agent uses a different label filter, naturally partitioning work. No orchestration layer needed — the label filters and priority ordering handle it.

See [Label-Driven Issue Selection](workflow-labels.md) for more detail.

---

## Blocking and Unblocking

When Agent A closes an issue that blocks another issue, the blocked issue becomes ready. Agent B — running `np claim ready` in a loop — will automatically pick it up.

```
# Agent A closes the blocker:
np close --claim <claim-id> --reason "Blocker resolved."

# Agent B's next claim ready picks up the unblocked issue:
np claim ready --author agent-b --json
# → Claims the previously-blocked issue
```

This creates natural work ordering without explicit agent-to-agent communication.

---

## Communicating Through Comments

Agents do not talk to each other directly. Comments on issues are the communication channel — they persist in the tracker and are visible to any agent that reads the issue later.

### Recording Decisions

When an agent makes a non-obvious choice during implementation, it should add a comment explaining the reasoning:

```
np json comment MYAPP-2e22n --author blue-seal-echo <<'JSONEND'
{
  "body": "Chose approach B over A because A requires a schema migration that would block NP-abc12."
}
JSONEND
```

This helps the next agent working on a related issue understand *why* the codebase looks the way it does — context that the code and commit history alone do not capture.

### Cross-Issue Context

When work on one issue affects another, add a comment to both:

```
# On the issue you are working on:
np json comment MYAPP-2e22n --author blue-seal-echo <<'JSONEND'
{
  "body": "The fix here also resolves the edge case described in MYAPP-3f44p."
}
JSONEND

# On the related issue:
np json comment MYAPP-3f44p --author blue-seal-echo <<'JSONEND'
{
  "body": "Edge case resolved by the fix in MYAPP-2e22n."
}
JSONEND
```

Comments do not require a claim — any agent can comment on any issue at any time, including closed issues.

---

## Handling Incidental Discoveries

During implementation, an agent will often notice problems unrelated to its current task — a failing test, a bug in adjacent code, a missing validation. These are *incidentals* and should be captured without derailing the current work.

### The Pattern

1. **Search** for an existing issue that covers the problem:
   ```
   np issue search "failing auth test"
   ```

2. **Create a new issue** if none exists:
   ```bash
   np create --author blue-seal-echo <<'JSONEND'
   {
     "title": "Fix flaky auth test in login_test.go",
     "labels": ["kind:bug"]
   }
   JSONEND
   ```

3. **Add a blocking relationship** if the incidental blocks your current work:
   ```
   np rel add MYAPP-2e22n blocked_by MYAPP-5h77q --author blue-seal-echo
   ```
   This makes your issue not ready until the blocker is resolved. Another agent running `np claim ready` will pick up the blocker when it becomes the highest-priority ready work.

4. **Continue your current task** if the incidental does not block you. The new issue enters the ready pool for another agent to claim.

### Why This Matters for Multi-Agent Setups

Without incidental capture, discovered problems either get fixed inline (risking scope creep and muddling the commit history) or forgotten. Creating a tracked issue ensures the work is visible to all agents and prioritized alongside everything else.

---

## Diagnosing Stuck State

When no agent is making progress:

```
$ np admin doctor --verbose
```

Common findings:

- **All issues blocked** — a circular dependency or a long chain of blockers.
- **All issues claimed** — every issue is held by an agent. Some may be stale.
- **Deferred ancestor** — an epic is deferred, making all its descendants not ready.

See [Troubleshooting](troubleshooting.md) for resolution steps.

---

## Failure Recovery

Multi-agent setups need mechanisms to recover from agent failures, premature closures, and work that cannot proceed. `np` provides three recovery paths.

### Stale Claim Recovery

When an agent crashes or is terminated without releasing its claim, the claim goes stale after the duration (default 2 hours). Another agent recovers the work:

```
# Check if the claim is stealable:
np show MYAPP-2e22n --json | jq '.claim_stale_at'

# Steal the stale claim:
np claim MYAPP-2e22n --author kind-comet-quest --steal
```

Or let `np claim ready --steal` handle it automatically — it steals a stale claim only when no unclaimed ready issues exist.

### Deferring Work That Cannot Proceed

When an agent discovers that its current task cannot be completed — a missing dependency, unclear requirements, or an architectural blocker — it should defer rather than abandon:

```
# Defer the issue (optionally with a target date):
np issue defer --claim <claim-id>
np issue defer --claim <claim-id> --until 2026-04-15
```

Deferring records *why* the work stopped and removes the issue from the ready pool. Unlike abandoning a claim (which blocks other agents until the stale duration expires), deferring is an explicit, immediate signal.

**Important:** a deferred epic makes all its descendants not ready. Use `np admin doctor` to check for this cascading effect.

To restore a deferred issue when the blocker is resolved:

```
np issue undefer MYAPP-2e22n --author kind-comet-quest
```

### Reopening Prematurely Closed Issues

If an agent closes an issue and the fix turns out to be incomplete or incorrect, any agent can reopen it:

```
np issue reopen MYAPP-2e22n --author kind-comet-quest
```

Reopening returns the issue to the open state and makes it ready for claiming again. Add a comment explaining why the closure was premature so the next agent has context:

```
np json comment MYAPP-2e22n --author kind-comet-quest <<'JSONEND'
{
  "body": "Reopened: the fix missed the edge case where the input is empty."
}
JSONEND
```

---

## Best Practices

1. **Always close or release.** An abandoned claim blocks other agents for up to 2 hours. Use `np close` to close, or `np issue release` if you cannot complete the work.

2. **Use comments liberally.** When multiple agents work on related issues, comments create an audit trail that helps each agent understand context set by others.

3. **Never abandon claims.** If an agent needs to stop working on an issue, release the claim explicitly:
   ```
   np issue release --claim <claim-id>
   ```

4. **Use `--json` for all agent interactions.** Human-readable output is for humans; agents should parse JSON.

5. **Handle exit code 2 gracefully.** When `np claim ready` returns exit code 2 (no ready issues), the agent should stop or wait — not spin in a tight loop.

6. **Set appropriate stale durations.** Default 2 hours is fine for most tasks. For long-running work (large refactors, complex investigations), set a longer duration at claim time with `--duration`.

7. **Use `np admin doctor` proactively.** Run it periodically to catch stale claims and stuck state before agents run out of work.
