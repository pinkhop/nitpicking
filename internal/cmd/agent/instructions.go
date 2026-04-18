package agent

// AgentInstructions returns a Markdown block describing how to use np,
// suitable for pasting into agent configuration per §8.2. The instructions
// are written for AI agents that operate autonomously against the issue
// tracker. This function lives in the driving adapter layer because the
// content is CLI-specific; a web or TUI adapter would produce guidance in
// a different format.
func AgentInstructions() string {
	return `This workspace uses the nitpicking (` + "`np`" + `) command line tool for issue tracking. Nitpicking is local-only — no network, no remote sync, no background daemons. It manages issues in a manner safe for parallel access.

` + "`np`" + ` is the **exclusive** tool for task management in this workspace. Do not use your platform's built-in task tracking (TodoWrite, TaskCreate, markdown checklists, etc.).

## Choosing an Author Name

Every mutation requires an ` + "`--author`" + ` flag identifying who is acting. If you do not already have a name, generate one with ` + "`np agent name`" + `. Pick a stable name and reuse it for your entire session.

## Issue Types

| Role | Purpose | How to work on it |
|------|---------|-------------------|
| **Task** | Leaf-node work item | Implement what it describes, then close it |
| **Epic** | Organizes children; carries a ` + "`completed`" + ` display badge when all children are closed, but remains open until explicitly closed | Decompose into child tasks (and sub-epics if large), then release it |

The ` + "`completed`" + ` badge is a display indicator only — it does not change the epic's primary state. An epic remains open until it is explicitly closed, typically via ` + "`np epic close-completed`" + ` (which handles the claim-close-release cycle in batch). Both epics and tasks may have children; ` + "`np epic close-completed --include-tasks`" + ` closes parent tasks in the same completed-by-children condition.

## Core Workflow

### 1. Find work

` + "```" + `
np claim ready --author <your-name>   # claim the highest-priority ready issue
np ready                              # browse the ready queue without claiming
np list --ready                       # equivalent to np ready (longer form)
` + "```" + `

` + "`np ready`" + ` supports the same filter flags as ` + "`np list`" + `: ` + "`--role`" + `, ` + "`--state`" + `, ` + "`--parent`" + `, and ` + "`--label`" + `. Use them to narrow the ready queue without falling back to ` + "`np list --ready`" + `:

` + "```" + `
np ready --role task                        # only ready tasks
np ready --label kind:bug                   # only ready bugs
np ready --parent NP-abc12                  # only ready children of NP-abc12
np ready --role task --label kind:bug       # combine filters (AND semantics)
` + "```" + `

Filter which issue gets claimed with ` + "`--with-label`" + ` and ` + "`--with-role`" + `:

` + "```" + `
np claim ready --with-label kind:bug --author <your-name>         # only claim bugs
np claim ready --with-role task --author <your-name>              # only claim tasks
np claim ready --with-label kind:bug --with-role task --author <your-name>  # combine filters
` + "```" + `

` + "`--with-label`" + ` uses ` + "`key:value`" + ` or ` + "`key:*`" + ` format (repeatable, AND semantics). ` + "`--with-role`" + ` accepts ` + "`task`" + ` or ` + "`epic`" + `.

Control claim staleness timing with ` + "`--duration`" + ` or ` + "`--stale-at`" + `:

` + "```" + `
np claim ready --duration 4h --author <your-name>                 # claim expires in 4 hours
np claim ready --stale-at 2026-04-02T18:00:00Z --author <your-name>  # claim expires at specific time
` + "```" + `

` + "`--duration`" + ` sets how long until the claim goes stale (default 2h, max 24h). ` + "`--stale-at`" + ` sets an absolute RFC3339 UTC timestamp (must be in the future, max 24h from now). They are mutually exclusive.

### 2. Work on the issue

**If you claimed a task:** implement it. Use the claim ID for all updates via ` + "`np json update`" + `:

` + "```" + `
np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title"
}
JSONEND
` + "```" + `

**If you claimed an epic:** plan and decompose it into child tasks using ` + "`np json create`" + `:

` + "```" + `
np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Subtask A",
  "parent": "<EPIC-ID>"
}
JSONEND

np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "Subtask B",
  "parent": "<EPIC-ID>"
}
JSONEND
` + "```" + `

Use the ` + "`parent`" + ` field to attach children to the epic. For large epics, decompose into sub-epics and leave further planning to a future implementor. Add ` + "`blocked_by`" + ` relationships between children to indicate required ordering.

To create a deferred issue (so it does not appear as ready work until explicitly undeferred), use the ` + "`--deferred`" + ` flag:

` + "```" + `
np json create --deferred --author <your-name> <<'JSONEND'
{
  "title": "Step 2",
  "parent": "<EPIC-ID>"
}
JSONEND
` + "```" + `

### 3. Document your work with comments

**Before transitioning state, add a comment to the issue.** Comments record context that the code and commit history cannot capture — your reasoning, trade-offs considered, dead ends explored, or anything a future reader would find useful.

` + "```" + `
np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Approach taken: ..."
}
JSONEND
` + "```" + `

Comments do not require claiming and can be added to any issue, including closed ones.

### 4. Transition state when done

**You MUST transition state when you are done.** Abandoned claims block other agents.

| Transition | When to use |
|------------|-------------|
| ` + "`np close --claim <CID> --reason \"...\"`" + ` | Task is complete (can be reopened if needed) |
| ` + "`np issue release --claim <CID>`" + ` | Epic has been decomposed; or task cannot be completed now — deletes the local claim record without changing the issue's primary state |
| ` + "`np issue defer --claim <CID>`" + ` | Shelve for later (can be restored with undefer) |

## Discovering Command Structure

**Always run ` + "`--help`" + ` before guessing a command's structure or arguments.** Every command and subcommand supports ` + "`--help`" + `, which shows valid subcommands, flags, and usage examples. Never fabricate a subcommand or flag name — consult ` + "`--help`" + ` first.

` + "```" + `
np rel --help               # list rel subcommands
np rel add --help           # show usage and flags for rel add
np rel remove --help        # show usage and flags for rel remove
` + "```" + `

## Managing Relationships

### Adding a relationship

` + "```" + `
np rel add <YOUR-ISSUE> blocked_by <BLOCKER-ID> --author <your-name>
np rel add <YOUR-ISSUE> refs <OTHER-ID> --author <your-name>
` + "```" + `

### Removing a relationship

Use ` + "`np rel remove`" + ` to delete a relationship that is no longer relevant. The argument syntax mirrors ` + "`np rel add`" + ` exactly — the same ` + "`<rel>`" + ` values are accepted, and the same positional order applies.

` + "```" + `
np rel remove <ISSUE-A> blocked_by <ISSUE-B> --author <your-name>
np rel remove <ISSUE-A> blocks <ISSUE-B> --author <your-name>
np rel remove <ISSUE-A> refs <ISSUE-B> --author <your-name>
` + "```" + `

## Handling Incidentals

If you discover something unrelated to your current issue (e.g., a failing test, a bug, a missing feature):

1. Search for an existing issue: ` + "`np issue search \"description\"`" + `
2. If none found, create a new issue:

` + "```" + `
np json create --author <your-name> <<'JSONEND'
{
  "role": "task",
  "title": "..."
}
JSONEND
` + "```" + `
3. If the incidental blocks your current work, add a relationship:
   ` + "`np rel add <YOUR-ISSUE> blocked_by <BLOCKER-ID> --author <your-name>`" + `

## Stale Claims

If no ready issues exist and there are stale claims, stale claims are automatically
overwritten when you run the normal claim command. Run ` + "`np admin doctor`" + ` to identify
stale claims blocking ready work, then claim normally:

` + "```" + `
np claim ready --author <your-name>
` + "```" + `

## Backups

Run ` + "`np admin backup`" + ` before any destructive operation (resets, restores, schema experiments). The backup is a gzip-compressed JSONL file written to ` + "`.np/`" + ` by default. Use ` + "`--output`" + ` to specify a file or directory.

## Diagnostics

` + "```" + `
np admin backup    # create a backup in .np/ (default filename includes the database prefix)
np admin doctor    # detect stale claims, no-ready-issues analysis, suggest unblock actions
np show <ID>       # full issue detail including readiness and relationships
np issue history <ID> # audit trail of all changes
` + "```" + `

## JSON Agent API

The ` + "`np json`" + ` command tree provides structured JSON input/output for all mutation operations. These commands read a JSON object from stdin and write JSON to stdout. Identity and context flags remain on the command line; the JSON object provides content fields only. All ` + "`json`" + ` subcommands output JSON unconditionally — there is no ` + "`--json`" + ` flag.

### json create

Create an issue from a JSON object on stdin. The ` + "`role`" + ` field defaults to ` + "`task`" + ` when omitted.

` + "```" + `
np json create --author <your-name> <<'JSONEND'
{
  "title": "Fix auth bug",
  "priority": "P1"
}
JSONEND
` + "```" + `

**CLI flags:** ` + "`--author`" + ` (required), ` + "`--with-claim`" + ` (optional, immediately claims the new issue), ` + "`--deferred`" + ` (optional, creates the issue in the deferred state; mutually exclusive with ` + "`--with-claim`" + `).
**JSON fields:** ` + "`title`" + ` (required), ` + "`role`" + ` (defaults to ` + "`task`" + `), ` + "`description`" + `, ` + "`acceptance_criteria`" + ` (**string**, not an array — Markdown-formatted text), ` + "`priority`" + `, ` + "`parent`" + `, ` + "`labels`" + ` (array of ` + "`key:value`" + ` strings). Unknown fields are rejected.

### json update

Update fields on a claimed issue. Missing fields mean "no change"; null fields mean "unset/clear".

` + "```" + `
np json update --claim <CLAIM-ID> <<'JSONEND'
{
  "title": "Revised title",
  "priority": "P0"
}
JSONEND
` + "```" + `

**CLI flags:** ` + "`--claim`" + ` (required).
**JSON fields:** ` + "`title`" + `, ` + "`description`" + `, ` + "`acceptance_criteria`" + ` (**string**, not an array — Markdown-formatted text), ` + "`priority`" + `, ` + "`parent`" + `, ` + "`labels`" + ` (array of ` + "`key:value`" + ` strings), ` + "`label_remove`" + ` (array of key strings), ` + "`role`" + ` (errors if different from current role). All fields are optional. Unknown fields are rejected.

### json comment

Add a comment to an issue from a JSON object on stdin.

` + "```" + `
np json comment <ISSUE-ID> --author <your-name> <<'JSONEND'
{
  "body": "Found the root cause in auth.go"
}
JSONEND
` + "```" + `

**Positional args:** ` + "`<ISSUE-ID>`" + ` (required).
**CLI flags:** ` + "`--author`" + ` (required).
**JSON fields:** ` + "`body`" + ` (required).

## Tabular Output

Commands that list issues (` + "`np list`" + `, ` + "`np ready`" + `, ` + "`np blocked`" + `, ` + "`np issue search`" + `, ` + "`np epic children`" + `) display results in a columnar table when outputting text.

### Default columns

The default column set is: **ID, PRIORITY, ROLE, STATE, TITLE**. These are the columns shown when no ` + "`--columns`" + ` flag is provided.

### Selecting columns with --columns

Use ` + "`--columns`" + ` to select and reorder which columns appear. Pass a comma-separated list of column names:

` + "```" + `
np list --columns ID,PRIORITY,TITLE              # compact view
np list --columns ID,PRIORITY,PARENT_ID,PARENT_CREATED,CREATED,ROLE,STATE,TITLE  # full view
` + "```" + `

Valid column names: **ID**, **CREATED**, **PARENT_ID**, **PARENT_CREATED**, **PRIORITY**, **ROLE**, **STATE**, **TITLE**. Column names are case-insensitive.

### Sorting with --order

Use ` + "`--order`" + ` to control the sort order. Append ` + "`:asc`" + ` or ` + "`:desc`" + ` to set the direction:

` + "```" + `
np list --order CREATED:desc    # newest first
np list --order MODIFIED:desc   # most recently modified first
np list --order PRIORITY:asc    # highest priority first (P0 before P1)
` + "```" + `

Valid order values match the column names plus **MODIFIED**. The default order varies by command: ` + "`np list`" + ` defaults to ID ascending; ` + "`np ready`" + ` and ` + "`np blocked`" + ` default to PRIORITY ascending.

## Key Rules

- **Run ` + "`--help`" + ` before guessing.** Any ` + "`np`" + ` command or subcommand supports ` + "`--help`" + `; use it to discover valid subcommands, flags, and argument formats rather than fabricating them.
- **Use ` + "`np claim ready`" + ` to find work.** Do not browse and cherry-pick issues.
- **Document your work.** Add a comment before transitioning state — capture reasoning, trade-offs, and findings.
- **Always transition state when done.** Close, release, or defer — never abandon a claim.
- **Closed issues can be reopened.** Use ` + "`np issue reopen <ID> --author <name>`" + ` to restore them.
- **Epics are typically closed via ` + "`np epic close-completed`" + `.** When an epic's children are all closed, it acquires the ` + "`completed`" + ` display badge but stays open. Run ` + "`np epic close-completed --author <name>`" + ` (add ` + "`--include-tasks`" + ` to also close parent tasks in the same condition) to batch-close all eligible issues.
- **Use ` + "`np`" + ` exclusively.** Do not track work outside of ` + "`np`" + `.
`
}
