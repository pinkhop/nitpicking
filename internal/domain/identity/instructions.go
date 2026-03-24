package identity

// AgentInstructions returns a Markdown block describing how to use np,
// suitable for pasting into agent configuration per §8.2. The instructions
// are written for AI agents that operate autonomously against the issue
// tracker.
func AgentInstructions() string {
	return `# np — Issue Tracker

np is the **exclusive** tool for task management in this project. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

np is local-only — no network, no remote sync, no background daemons. It stores tickets in an embedded SQLite database under the ` + "`.np/`" + ` directory.

## Choosing an Author Name

Every mutation requires an ` + "`--author`" + ` flag identifying who is acting. Pick a stable name and reuse it for your entire session. Generate one with ` + "`np agent-name`" + ` if you need a fresh identifier.

## Ticket Types

| Role | Purpose | How to work on it |
|------|---------|-------------------|
| **Task** | Leaf-node work item | Implement what it describes, then close it |
| **Epic** | Organizes children; completion is derived | Decompose into child tasks (and sub-epics if large), then release it |

An epic is complete when all its children are closed or complete. You never close an epic directly.

## Core Workflow

### 1. Find work

` + "```" + `
np next --author <your-name>          # claim the highest-priority ready ticket
np list --ready                       # browse all ready tickets without claiming
` + "```" + `

### 2. Work on the ticket

**If you claimed a task:** implement it. Use the claim ID for all updates.

` + "```" + `
np update <TICKET-ID> --claim-id <CLAIM-ID> --title "Revised title"
` + "```" + `

**If you claimed an epic:** plan and decompose it into child tasks.

` + "```" + `
np create --role task --title "Subtask A" --author <your-name> --parent <EPIC-ID>
np create --role task --title "Subtask B" --author <your-name> --parent <EPIC-ID>
` + "```" + `

Use ` + "`--parent`" + ` to attach children to the epic. For large epics, decompose into sub-epics and leave further planning to a future implementor. Add ` + "`blocked_by`" + ` relationships between children to indicate required ordering.

### 3. Transition state when done

**You MUST transition state when you are done.** Abandoned claims block other agents.

| Transition | When to use |
|------------|-------------|
| ` + "`np close <ID> --claim-id <CID>`" + ` | Task is complete (terminal — cannot reopen) |
| ` + "`np release <ID> --claim-id <CID>`" + ` | Epic has been decomposed; or task cannot be completed now |
| ` + "`np wait <ID> --claim-id <CID>`" + ` | Blocked on a human or stakeholder decision |

### 4. Leave notes about your work

Notes do not require claiming and can be added to any ticket, including closed ones.

` + "```" + `
np note add <TICKET-ID> --body "Approach taken: ..." --author <your-name>
` + "```" + `

Add notes about: your approach, opinions that influenced decisions, interesting findings, or anything a future reader would find useful.

## Handling Incidentals

If you discover something unrelated to your current ticket (e.g., a failing test, a bug, a missing feature):

1. Search for an existing ticket: ` + "`np search \"description\"`" + `
2. If none found, create a new ticket: ` + "`np create --role task --title \"...\" --author <your-name>`" + `
3. If the incidental blocks your current work, add a relationship:
   ` + "`np relate add <YOUR-TICKET> blocked_by <BLOCKER-ID> --author <your-name>`" + `

## Stale Claims and Stealing

If no ready tickets exist, steal a stale one:

` + "```" + `
np next --steal-fallback --author <your-name>
` + "```" + `

## Diagnostics

` + "```" + `
np doctor       # detect cycles, stale claims, epics needing decomposition
np show <ID>    # full ticket detail including readiness and relationships
np history <ID> # audit trail of all changes
` + "```" + `

## Key Rules

- **Use ` + "`np next`" + ` to find work.** Do not browse and cherry-pick tickets.
- **Always transition state when done.** Close, release, or wait — never abandon a claim.
- **Close is terminal.** Closed tasks cannot be reopened or modified (notes can still be added).
- **Epics are never closed directly.** They complete when all children resolve.
- **Use ` + "`np`" + ` exclusively.** Do not track work outside of ` + "`np`" + `.
`
}
