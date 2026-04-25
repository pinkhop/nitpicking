# Quickstart

This guide is intentionally narrow. It teaches the smallest useful `np` workflow:

1. Install `np`
2. Initialize a workspace
3. Create a task
4. Claim it
5. Comment on it
6. Close it

Ignore labels, epics, and multi-agent coordination for now. You do not need them to get value from `np`.

## Install

Build `np` from source:

```bash
$ git clone https://github.com/pinkhop/nitpicking.git
$ cd nitpicking
$ make build
```

This writes the binary to `dist/np`. Put it on your `PATH` or run it via `./dist/np`.

## Initialize a Workspace

Run `np init` at the root of the project where you want the tracker to live:

```bash
$ cd ~/projects/my-app
$ np init FOO
[ok] Initialized database with prefix FOO
```

This creates `.np/` in the workspace root. `np` discovers that directory by walking up from the current directory, so later commands work from any subdirectory inside the workspace.

```bash
$ cd ~/projects/my-app/src/api
$ np admin where
/Users/you/projects/my-app/.np
```

## Create a Task

If stdin is a terminal, `np create` opens an interactive form:

```bash
$ np create
```

If stdin is a pipe, `np create` reads JSON and returns JSON:

```bash
$ np create --author alice <<'JSONEND'
{
  "title": "Add user login endpoint",
  "priority": "P1"
}
JSONEND
{
  "id": "FOO-a3bxr",
  "role": "task",
  "title": "Add user login endpoint",
  "priority": "P1",
  "state": "open"
}
```

Start with plain tasks. Do not worry about parents or labels yet.

## See What Is Ready

Use `np ready` to see work that can be picked up now:

```bash
$ np ready
FOO-a3bxr  task  open (ready)  P1  Add user login endpoint

1 ready
```

`ready` means the issue is open, unclaimed, unblocked, and not suppressed by a deferred ancestor. In the simple task-only workflow, that usually just means "available to work on."

## Claim the Task

Before you change a task or close it, claim it:

```bash
$ np claim ready --author alice
Claimed FOO-a3bxr
  Claim ID: a4dace30e46eb1ec14019c79a59c6b27
  Author: alice
  Stale at: 2026-03-28 16:30:00
```

Save the claim ID. You need it for later mutations.

Even if you are the only person using `np`, claiming is still useful. It marks the task as actively in progress and forces you to finish cleanly by closing or releasing it.

## Add a Comment

Comments are the audit trail. Use them to record what you found, what you changed, or why you stopped.

Interactive:

```bash
$ np form comment FOO-a3bxr
```

Scripted:

```bash
$ np json comment FOO-a3bxr --author alice <<'JSONEND'
{
  "body": "Implemented the handler and added request validation."
}
JSONEND
```

Comments do not require a claim.

## Close the Task

When the work is done, close it with a reason:

```bash
$ np close \
    --claim a4dace30e46eb1ec14019c79a59c6b27 \
    --reason "Login endpoint implemented and covered by tests."
Closed FOO-a3bxr
```

`np close` adds the reason as a comment and closes the issue in one step.

## Release Instead of Closing

If you claimed a task but are not finishing it now, release it:

```bash
$ np issue release --claim a4dace30e46eb1ec14019c79a59c6b27
Released FOO-a3bxr
```

That returns the task to the ready queue.

## The Whole Loop

For the basic task-only workflow, this is the whole habit:

1. `np ready`
2. `np claim ready --author <name>`
3. Do the work
4. `np json comment <id> --author <name>` or `np form comment <id>`
5. `np close --claim <claim-id> --reason "..."`

## What To Read Next

- Read [Daily Work](daily-work.md) for the commands you will use once the workspace has real traffic.
- Read [Labels](labels.md) when filtering, routing, or lightweight grouping becomes useful.
- Read [Epics](epics.md) only when flat tasks plus labels are not enough and you need explicit hierarchy.
- Read [Agent Setup](agents/setup.md) if an AI assistant will operate the tracker.
