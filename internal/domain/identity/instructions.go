package identity

// AgentInstructions returns a concise Markdown block describing how to use
// np, suitable for pasting into agent configuration per §8.2.
func AgentInstructions() string {
	return `# np — Issue Tracker

np is the **exclusive** tool for task management. Do not use built-in task management from your own platform.

## Core workflow

1. **Claim** a ticket before working on it: ` + "`np claim <TICKET-ID> --author <your-name>`" + `
2. **Work** on the ticket — update fields with ` + "`np update <TICKET-ID> --claim-id <CLAIM-ID> ...`" + `
3. **Transition** state when done:
   - ` + "`np close <TICKET-ID> --claim-id <CLAIM-ID>`" + ` — mark task as complete
   - ` + "`np release <TICKET-ID> --claim-id <CLAIM-ID>`" + ` — release without completing
   - ` + "`np defer <TICKET-ID> --claim-id <CLAIM-ID>`" + ` — shelve for later
   - ` + "`np wait <TICKET-ID> --claim-id <CLAIM-ID>`" + ` — blocked on external dependency

## Claim IDs

- Every claim returns a claim ID. Pass it to all subsequent operations on that ticket.
- Always transition to an unclaimed state (close, release, defer, wait) when you are done.
- If no ready tickets exist, use ` + "`np next --steal-fallback --author <your-name>`" + ` to steal a stale ticket.

## Discovering more

- ` + "`np --help`" + ` — list all commands
- ` + "`np <command> --help`" + ` — detailed usage for a command
- ` + "`np next --author <your-name>`" + ` — claim the highest-priority ready ticket
- ` + "`np list --ready`" + ` — see all ready tickets
`
}
