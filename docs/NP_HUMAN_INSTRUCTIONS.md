# np Welcome Command — Proposed Content

This document contains the terminal output that humans see when running
`np welcome`. This is the human-facing setup guide, not the agent-facing
workflow instructions (which live in `np agent prime`).

---

## Table of Contents

1. [`np welcome` — Terminal output](#np-welcome--terminal-output)
2. [`np welcome` — What it does](#np-welcome--what-it-does)
3. [Design rationale](#design-rationale)

---

## `np welcome` — Terminal output

`np welcome` displays the full setup guide every time it is run. Each step has a
checkbox that reflects the current project state: `[green ✓]` for complete, `[ ]`
for not yet done. The guide is always shown in full regardless of status — np does
not gate steps or enforce ordering. Formatting markers are indicated with `[bold]`
and `[accent]` below.

### Example: partially configured project

```
[bold]np — local-only issue tracker for AI agent workflows[/bold]

[bold]Setup[/bold]

  - [green ✓] Initialize database                     np init <PREFIX>
  - [ ] Exclude .np/ from version control              add .np/ to .gitignore
  - [ ] Tell your AI agent how to use np               paste np agent prime output
  - [ ] Choose an author name for yourself              np agent name or pick your own

[bold]Initialize database[/bold]

  Ticket IDs use a project prefix (e.g., prefix "NP" produces NP-a3bxr).
  Choose something short and project-specific — convention is 2–4 uppercase letters.

    [accent]np init <PREFIX>[/accent]

  [dim]Current: initialized with prefix NP[/dim]

[bold]Add .np/ to .gitignore[/bold]

  np stores its database locally in .np/ — you probably don't want to commit it.
  Add this line to your .gitignore:

    [accent].np/[/accent]

  Or run:
    [accent]echo '.np/' >> .gitignore[/accent]

[bold]Add agent instructions to your project[/bold]

  np works best when your AI agent knows it exists. Run:

    [accent]np agent prime[/accent]

  This prints Markdown workflow instructions. Paste the output into your agent's
  instruction file:

    • [accent]CLAUDE.md[/accent]   — for Claude Code
    • [accent]AGENTS.md[/accent]   — for GitHub Copilot and other agents
    • [accent].github/copilot-instructions.md[/accent] — Copilot alternate location

  Or tell your agent to run [accent]np agent prime[/accent] at the start of each session.
  No hooks or integrations required — np is just a CLI.

[bold]Pick an author name[/bold]

  Every np command that changes data requires an [accent]--author[/accent] flag. Use any
  stable identifier (your name, handle, etc.), or generate one:

    [accent]np agent name[/accent]

  Agents should generate their own name at session start. Humans can use whatever
  they like — consistency across a session is what matters.

[bold]Quick reference[/bold]

    [accent]np create --role task --title "..." --author <name>[/accent]  Create a ticket
    [accent]np list --ready[/accent]                                      Find available work
    [accent]np claim ready --author <name>[/accent]                       Claim next ready ticket
    [accent]np state close <ID> --claim <CLAIM-ID>[/accent]               Complete a task
    [accent]np doctor[/accent]                                            Run diagnostics
    [accent]np help[/accent]                                              Full command reference
    [accent]np agent prime[/accent]                                       Agent workflow instructions
```

### Example: fully configured project

The same output, but all checkboxes are checked:

```
[bold]Setup[/bold]

  - [green ✓] Initialize database                     np init <PREFIX>
  - [green ✓] Exclude .np/ from version control        add .np/ to .gitignore
  - [green ✓] Tell your AI agent how to use np         paste np agent prime output
  - [green ✓] Choose an author name for yourself       np agent name or pick your own
```

The rest of the output is identical. The full guide is always shown — it doubles
as a reference card the user can return to any time.

### Example: no database yet

All checkboxes are unchecked. The "Current: ..." status line under "Initialize
database" is omitted.

---

## `np welcome` — What it does

`np welcome` is **read-only** — it does not modify any files or create the database.
It inspects the current state, marks completed steps, and shows the full guide.

### Detection logic

| Check | How | Checkmark when |
|-------|-----|----------------|
| Database initialized | Look for `.np/` directory (same discovery walk as all np commands) | `.np/` directory found |
| `.gitignore` configured | Check if `.gitignore` exists and contains a line matching `.np/` or `.np` | Match found |
| Agent instructions | Check if `CLAUDE.md`, `AGENTS.md`, or `.github/copilot-instructions.md` contains `np` references | Any file mentions np workflow |
| Author name | Not automatically detectable | Always unchecked (informational step) |

The "author name" step is always shown unchecked because there is no persistent
author configuration — it is a per-command flag. This step exists for orientation,
not for status tracking.

---

## Design rationale

### Why show everything every time?

np is non-opinionated about setup order and does not enforce prerequisites. A
developer who wants to commit `.np/` to version control can do so — np does not
second-guess that decision. Showing the full guide with status indicators gives
the user a clear picture without imposing a workflow.

### Why read-only?

Modifying files without explicit consent violates np's "non-invasive" principle.
The user may have a global gitignore, a different agent instruction strategy, or
simply want to understand what is happening before acting. `np welcome` observes
and recommends; the user executes.

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

1. Project setup (init, gitignore, agent instructions, author identity)
2. A quick-reference cheat sheet
3. Pointers to deeper resources (`np help`, `np agent prime`)

Architecture, design decisions, contribution guidelines, and build instructions
belong in the README or a dedicated user guide — not in `welcome` output.
