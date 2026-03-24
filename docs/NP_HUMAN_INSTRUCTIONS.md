# np Welcome Command — Proposed Content

This document contains the terminal output and file modifications that humans see
when running `np welcome`. This is the human-facing setup guide, not the agent-facing
workflow instructions (which live in `np agent prime`).

---

## Table of Contents

1. [`np welcome` — Terminal output](#np-welcome--terminal-output)
2. [`np welcome` — What it does](#np-welcome--what-it-does)
3. [Design rationale](#design-rationale)

---

## `np welcome` — Terminal output

`np welcome` walks a human through first-time project setup. It detects what has
already been done and skips completed steps. Formatting markers are indicated with
`[bold]` and `[accent]` below.

### Fresh project (no `.np/` directory)

```
[bold]Welcome to np — the local-only issue tracker for AI agent workflows.[/bold]

Let's set up your project. This takes about 30 seconds.

[bold]Step 1: Initialize the database[/bold]

  np needs a ticket prefix — a short tag that appears in all ticket IDs
  (e.g., prefix "NP" produces tickets like NP-a3bxr).

  Choose something short and project-specific. Convention: 2–4 uppercase letters.

  Run:
    [accent]np init <PREFIX>[/accent]

  Example:
    [accent]np init NP[/accent]
```

After `np init` succeeds, `np welcome` continues (or the user re-runs `np welcome`):

### Database exists, `.gitignore` not configured

```
[bold]Step 2: Add .np/ to .gitignore[/bold]

  np stores its database locally — it should not be committed to version control.
  Add this line to your .gitignore:

    [accent].np/[/accent]

  Or run:
    [accent]echo '.np/' >> .gitignore[/accent]
```

### Database exists, `.gitignore` configured

```
[bold]Step 3: Tell your AI agent about np[/bold]

  np works best when your AI agent knows it exists. Run:

    [accent]np agent prime[/accent]

  This prints Markdown instructions your agent can follow. Paste the output into
  your agent's instruction file:

    • [accent]CLAUDE.md[/accent]   — for Claude Code
    • [accent]AGENTS.md[/accent]   — for GitHub Copilot and other agents
    • [accent].github/copilot-instructions.md[/accent] — Copilot alternate location

  Alternatively, tell your agent to run [accent]np agent prime[/accent] itself at the
  start of each session. No hooks or integrations required — np is just a CLI.

[bold]Step 4: Pick an author name[/bold]

  Every np command that changes data requires an [accent]--author[/accent] flag. You can use
  any stable identifier (your name, handle, etc.), or generate one:

    [accent]np agent name[/accent]

  Agents should generate their own name at session start. Humans can use whatever
  they like — consistency across a session is what matters.

[bold]You're all set.[/bold]

  Quick reference:
    [accent]np create --role task --title "..." --author <name>[/accent]  Create a ticket
    [accent]np list --ready[/accent]                                      Find available work
    [accent]np claim ready --author <name>[/accent]                       Claim next ready ticket
    [accent]np close <ID> --claim <CLAIM-ID>[/accent]                  Complete a task
    [accent]np doctor[/accent]                                            Run diagnostics

  For the full command reference, run:
    [accent]np help[/accent]

  For agent workflow instructions:
    [accent]np agent prime[/accent]
```

### Already set up (database exists, `.gitignore` has `.np/`)

If `np welcome` is re-run after setup is complete:

```
[bold]np is already configured for this project.[/bold]

  Database:   .np/ (prefix: NP)
  .gitignore: ✓ .np/ excluded

  Useful commands:
    [accent]np agent prime[/accent]   — Print agent workflow instructions
    [accent]np agent name[/accent]    — Generate an author name
    [accent]np doctor[/accent]        — Run diagnostics
    [accent]np help[/accent]          — Full command reference
```

---

## `np welcome` — What it does

`np welcome` is **read-only** — it does not modify any files or create the database.
It inspects the current state and tells the user what to do next. This avoids
surprising side effects and keeps the human in control.

### Detection logic

| Check | How |
|-------|-----|
| Database exists | Look for `.np/` directory (same discovery walk as all np commands) |
| `.gitignore` configured | Check if `.gitignore` exists and contains a line matching `.np/` or `.np` |
| Prefix | Read from database metadata (if database exists) |

### State machine

```
                    ┌─────────────┐
                    │  No .np/    │──→ Show Step 1 (init)
                    └─────────────┘
                          │
                     np init runs
                          │
                          ▼
                    ┌─────────────┐
                    │  .np/ exists│
                    │  no ignore  │──→ Show Step 2 (gitignore)
                    └─────────────┘
                          │
                   user adds ignore
                          │
                          ▼
                    ┌─────────────┐
                    │  .np/ exists│
                    │  ignored    │──→ Show Steps 3–4 (agent + author)
                    └─────────────┘
                          │
                     setup complete
                          │
                          ▼
                    ┌─────────────┐
                    │  All done   │──→ Show "already configured" summary
                    └─────────────┘
```

The command always shows from the first incomplete step through the end. A user who
runs `np welcome` after `np init` but before updating `.gitignore` sees Steps 2–4.

---

## Design rationale

### Why not auto-create the database?

`np init` requires a prefix argument — a deliberate human choice. `welcome` could
prompt for it interactively, but np avoids interactive prompts in favor of explicit
CLI invocations. This keeps the tool scriptable and predictable.

### Why not auto-modify `.gitignore`?

Modifying tracked files without explicit consent violates np's "non-invasive"
principle. The user may have a global gitignore, a different ignore strategy, or
simply want to know what is happening before it happens.

### Why not install hooks or write to CLAUDE.md?

This is the core philosophical difference from beads. np is a CLI tool, not a
platform integration. It does not:

- Install hooks into Claude Code, Copilot, or any other tool
- Write to or modify instruction files (CLAUDE.md, AGENTS.md)
- Register background processes or daemons
- Modify global configuration outside the project directory

The user pastes `np agent prime` output where they see fit. This keeps np a
single-purpose tool with no coupling to any specific AI agent platform.

### Why share concepts with agent instructions?

The core concepts (claiming, readiness, ticket types, state transitions) are the
same whether a human or an agent is using np. Rather than maintaining two parallel
explanations, `welcome` teaches the setup steps and points to `np agent prime` for
the full workflow reference. This keeps `welcome` short and avoids documentation
drift.

### Overlap with README / user guide

`welcome` is not a substitute for documentation. It covers only:

1. One-time project setup (init, gitignore, agent instructions)
2. A quick-reference cheat sheet
3. Pointers to deeper resources (`np help`, `np agent prime`)

Architecture, design decisions, contribution guidelines, and build instructions
belong in the README or a dedicated user guide — not in `welcome` output.
