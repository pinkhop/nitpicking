package identity

// AgentInstructions returns a Markdown block describing how to use np,
// suitable for pasting into agent configuration per §8.2. The instructions
// are written for AI agents that operate autonomously against the issue
// tracker.
func AgentInstructions() string {
	return `# np — Issue Tracker

np is the **exclusive** tool for task management in this project. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

np is local-only — no network, no remote sync, no background daemons. It stores issues in an embedded SQLite database under the ` + "`.np/`" + ` directory.

## Choosing an Author Name

Every mutation requires an ` + "`--author`" + ` flag identifying who is acting. Pick a stable name and reuse it for your entire session. Generate one with ` + "`np agent name`" + ` if you need a fresh identifier.

## Issue Types

| Role | Purpose | How to work on it |
|------|---------|-------------------|
| **Task** | Leaf-node work item | Implement what it describes, then close it |
| **Epic** | Organizes children; completion is derived | Decompose into child tasks (and sub-epics if large), then release it |

An epic is complete when all its children are closed or complete. You never close an epic directly.

## Core Workflow

### 1. Find work

` + "```" + `
np claim ready --author <your-name>   # claim the highest-priority ready issue
np list --ready                       # browse all ready issues without claiming
` + "```" + `

### 2. Work on the issue

**If you claimed a task:** implement it. Use the claim ID for all updates.

` + "```" + `
np update <ISSUE-ID> --claim <CLAIM-ID> --title "Revised title"
` + "```" + `

**If you claimed an epic:** plan and decompose it into child tasks.

` + "```" + `
np create --role task --title "Subtask A" --author <your-name> --parent <EPIC-ID>
np create --role task --title "Subtask B" --author <your-name> --parent <EPIC-ID>
` + "```" + `

Use ` + "`--parent`" + ` to attach children to the epic. For large epics, decompose into sub-epics and leave further planning to a future implementor. Add ` + "`blocked_by`" + ` relationships between children to indicate required ordering.

### 3. Document your work with comments

**Before transitioning state, add a comment to the issue.** Comments record context that the code and commit history cannot capture — your reasoning, trade-offs considered, dead ends explored, or anything a future reader would find useful.

` + "```" + `
np comment add --issue <ISSUE-ID> --body "Approach taken: ..." --author <your-name>
` + "```" + `

Comments do not require claiming and can be added to any issue, including closed ones.

### 4. Transition state when done

**You MUST transition state when you are done.** Abandoned claims block other agents.

| Transition | When to use |
|------------|-------------|
| ` + "`np state close <ID> --claim <CID>`" + ` | Task is complete (can be reopened if needed) |
| ` + "`np release <ID> --claim <CID>`" + ` | Epic has been decomposed; or task cannot be completed now |
| ` + "`np state wait <ID> --claim <CID>`" + ` | Blocked on a human or stakeholder decision |

## Handling Incidentals

If you discover something unrelated to your current issue (e.g., a failing test, a bug, a missing feature):

1. Search for an existing issue: ` + "`np search \"description\"`" + `
2. If none found, create a new issue: ` + "`np create --role task --title \"...\" --author <your-name>`" + `
3. If the incidental blocks your current work, add a relationship:
   ` + "`np relate add <YOUR-ISSUE> blocked_by <BLOCKER-ID> --author <your-name>`" + `

## Stale Claims and Stealing

If no ready issues exist, steal a stale one:

` + "```" + `
np claim ready --steal-if-needed --author <your-name>
` + "```" + `

## Diagnostics

` + "```" + `
np admin doctor    # detect stale claims, no-ready-issues analysis, suggest unblock actions
np show <ID>       # full issue detail including readiness and relationships
np issue history <ID> # audit trail of all changes
` + "```" + `

## Key Rules

- **Use ` + "`np claim ready`" + ` to find work.** Do not browse and cherry-pick issues.
- **Document your work.** Add a comment before transitioning state — capture reasoning, trade-offs, and findings.
- **Always transition state when done.** Close, release, or wait — never abandon a claim.
- **Closed issues can be reopened.** Claim a closed issue and release it to reopen.
- **Epics are never closed directly.** They complete when all children resolve.
- **Use ` + "`np`" + ` exclusively.** Do not track work outside of ` + "`np`" + `.
`
}
