# CLI Implementation Guide

This guide documents how Nitpicking's CLI is built in practice.

Use [Architecture](architecture.md) for layering rules. Use this file when you
are changing commands, root wiring, help output, or command execution flow.

## What This Guide Covers

This repo is a local-only CLI application. The implementation style is
optimized for:

- clear command behavior
- explicit separation between CLI wiring and business logic
- fast unit tests using the in-memory adapter
- minimal framework ceremony

## Command Package Shape

Each command or command group lives under `internal/cmd/`.

Typical structure:

1. `NewCmd(f *cmdutil.Factory)` defines flags, args, and help text.
2. The `Action` closure validates CLI input.
3. The command gets the dependency it needs from the `Factory`.
4. The command delegates real behavior to a package-local helper.

Common helper names in this repo include:

- `Run`
- `JSONLRun`
- `Reopen`
- `Defer`
- `RunCreate`
- `RunUpdate`

This keeps framework code and business behavior separate, which makes tests
smaller and more stable.

## What Not To Do

- Do not bury business rules inside `Action` closures.
- Do not let commands reach into driven adapters directly.
- Do not move business behavior into `cmdutil`.

## Service Access From Commands

Most commands construct the service through:

```go
svc, err := cmdutil.NewTracker(f)
```

That keeps the command talking to the driving port rather than directly to
storage.

The normal path should be:

- CLI command
- driving port
- core
- driven ports and adapters

## TTY-Aware Flows

`internal/iostreams` abstracts stdin, stdout, stderr, and terminal capability
checks.

Use it when behavior changes between interactive and non-interactive contexts.
The clearest example is `np create`:

- if stdin is a TTY, it delegates to the interactive form flow
- if stdin is not a TTY, it reads JSON from stdin

Prefer explicit branching on `IOStreams` over ad hoc terminal probing.

## Form Versus JSON Workflows

This repo deliberately supports different mutation paths for different users:

- `form` commands for interactive terminal use
- `json` commands for agents and scripts

The root `create` command adds stdin-based auto-detection on top of that.

When changing mutation flows:

- keep interactive concerns under `internal/cmd/formcmd`
- keep structured stdin/stdout flows under `internal/cmd/jsoncmd`
- preserve the semantic distinction between human-oriented and agent-oriented
  paths

## Launch Path

`cmd/np/main.go` owns the executable entry point:

```go
func main() {
    os.Exit(int(run()))
}
```

`os.Exit` is called only there. That keeps the rest of the launch path
testable and ensures deferred cleanup in the call stack runs before the process
exits.

The local `run()` function does four things:

1. Assemble dependencies with `wiring.NewCore`.
2. Construct the root command with `root.NewRootCmd`.
3. Execute the command tree with `rootCmd.Run(...)`.
4. Translate any returned error into a typed exit code with
   `cmdutil.ClassifyError`.

## Dependency Assembly

`internal/wiring/app.go` constructs a `*cmdutil.Factory`.

Today the `Factory` carries:

- `AppName`
- `AppVersion`
- `BuildInfo`
- `IOStreams`
- `Workspace`
- `Store`
- `SignalCancelIsError`

### Eager dependency: `IOStreams`

`IOStreams` is built eagerly via `iostreams.System()` because it is cheap and
used by nearly every command.

### Lazy dependency: `Store`

`Store` is a function field:

```go
Store func() (*sqlite.Store, error)
```

It memoizes the SQLite store on first use. That matters because some commands
do not need database access at all, such as:

- `np version`
- `np agent name`
- `np agent prime`

### Workspace resolution

The root command exposes a global `--workspace` flag, which writes into
`Factory.Workspace`.

Store resolution behaves as follows:

- If `Workspace` is set, the SQLite adapter looks only there.
- Otherwise, the adapter walks upward from the current working directory
  looking for `.np/`.
- If no database is found and the command is `np init`, wiring allows the
  store path to be created so init can finish schema setup.

## Root Command Construction

`internal/cmd/root/root.go` builds the root `*cli.Command`.

The root command defines:

- the app name and usage string
- the global `--workspace` flag
- the ordered command categories shown in help output
- a `Before` hook that installs signal-aware context cancellation
- an `After` hook that deregisters the signal handler

The command tree is grouped into the categories actually shipped today:

- `Setup`
- `Core Workflow`
- `Issues`
- `Agent Toolkit`
- `Admin`
- `Info`

These categories are rendered in a custom help template so the help output
stays workflow-first rather than alphabetical.

## Before / After Hooks

The root `Before` hook wraps the incoming context with:

```go
signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
```

That means every command action receives a context that is cancelled on
`SIGINT` or `SIGTERM`.

The `After` hook calls the `stopSignals` function returned by `NotifyContext`,
which deregisters the signal handler.

There is no logging setup, diagnostics server, gops agent, or extra
process-wide lifecycle machinery in the current launch path.

## Error Classification

If command execution returns `nil`, `run()` returns `cmdutil.ExitOK`.

If command execution returns an error, `cmdutil.ClassifyError` maps it to the
repository's exit-code model. This keeps command packages focused on returning
meaningful errors rather than manually choosing process exit integers.

The important architectural rule is:

- commands return errors
- `run()` classifies errors
- `main()` owns `os.Exit`

That keeps exit policy centralized and testable.

## Tests

The preferred testing model is behavior-first, not framework-first.

### Use the right tier

- Unit tests for command helpers, domain logic, and core behavior
- Boundary tests for SQLite adapter behavior
- Blackbox tests for end-to-end CLI behavior

### Keep tests close to the code

- command behavior tests stay in the command package
- domain validation tests stay in `internal/domain`
- core use-case tests stay in `internal/core`
- adapter tests stay next to the adapter

### Prefer real in-memory behavior over mocks

The in-memory storage adapter is a real implementation, not a mock. Use it to
keep core tests fast and honest.

## Documentation-Sensitive Areas

Some parts of the repo are especially prone to doc drift:

- the top-level command tree in `internal/cmd/root`
- agent bootstrap text in `internal/cmd/agent/instructions.go`
- the dual `form` and `json` mutation model
- workspace discovery behavior and `--workspace`

If you change any of those, update docs in the same change.

When command names or flags change, also verify:

- `np agent prime` still emits valid commands
- `.claude/rules/issue-tracking.md` can be regenerated without manual fixes

## Related Documents

- [First Change](../getting-started/first-change.md)
- [Architecture](architecture.md)
- [Package Layout](package-layout.md)
