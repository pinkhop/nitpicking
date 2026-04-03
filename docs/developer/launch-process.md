# Application Launch Process

This document traces the real startup path of the `np` binary, from `main()` through command execution and error classification.

It is intentionally specific to this repository. It does not describe a generic CLI template.

---

## 1. Process Entry

`cmd/np/main.go` owns the executable entry point:

```go
func main() {
    os.Exit(int(run()))
}
```

`os.Exit` is called only here. That keeps the rest of the launch path testable and ensures deferred cleanup in the call stack runs before the process exits.

The local `run()` function does four things:

1. Assemble dependencies with `wiring.NewCore`.
2. Construct the root command with `root.NewRootCmd`.
3. Execute the command tree with `rootCmd.Run(...)`.
4. Translate any returned error into a typed exit code with `cmdutil.ClassifyError`.

---

## 2. Dependency Assembly

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

`IOStreams` is built eagerly via `iostreams.System()` because it is cheap and used by nearly every command.

### Lazy dependency: `Store`

`Store` is a function field:

```go
Store func() (*sqlite.Store, error)
```

It memoizes the SQLite store on first use. That matters because some commands do not need database access at all, such as:

- `np version`
- `np agent name`
- `np agent prime`

### Workspace resolution

The root command exposes a global `--workspace` flag, which writes into `Factory.Workspace`.

Store resolution behaves as follows:

- If `Workspace` is set, the SQLite adapter looks only there.
- Otherwise, the adapter walks upward from the current working directory looking for `.np/`.
- If no database is found and the command is `np init`, wiring allows the store path to be created so init can finish schema setup.

This is why the same binary can support both workspace discovery and explicit workspace targeting without extra wiring layers.

---

## 3. Root Command Construction

`internal/cmd/root/root.go` builds the root `*cli.Command`.

The root command defines:

- The app name and usage string
- The global `--workspace` flag
- The ordered command categories shown in help output
- A `Before` hook that installs signal-aware context cancellation
- An `After` hook that deregisters the signal handler

### Command registration

The command tree is grouped into the categories actually shipped today:

- `Setup`
- `Core Workflow`
- `Issues`
- `Agent Toolkit`
- `Admin`
- `Info`

These categories are rendered in a custom help template so the help output stays workflow-first rather than alphabetical.

---

## 4. Before / After Hooks

The root `Before` hook wraps the incoming context with:

```go
signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
```

That means every command action receives a context that is cancelled on `SIGINT` or `SIGTERM`.

The `After` hook calls the `stopSignals` function returned by `NotifyContext`, which deregisters the signal handler.

There is no logging setup, diagnostics server, gops agent, or extra process-wide lifecycle machinery in the current launch path.

---

## 5. Command Execution

Once `rootCmd.Run(context.Background(), os.Args)` is called, `urfave/cli/v3` takes over:

1. It parses arguments and flags.
2. It selects the target subcommand.
3. It runs the root `Before` hook.
4. It runs the selected command's `Action`.
5. It runs the root `After` hook.

Command packages generally follow this shape:

1. `NewCmd` declares flags, args, and description text.
2. The `Action` closure validates CLI input.
3. The command constructs only the dependencies it needs, usually through `cmdutil.NewTracker(f)` or directly from the `Factory`.
4. The command delegates the behavior to a helper such as `Run`, `JSONLRun`, or a focused package-level function.

Examples of this pattern are spread across:

- `internal/cmd/create`
- `internal/cmd/claim`
- `internal/cmd/done`
- `internal/cmd/formcmd`
- `internal/cmd/jsoncmd`
- `internal/cmd/admincmd`

---

## 6. Error Classification

If command execution returns `nil`, `run()` returns `cmdutil.ExitOK`.

If command execution returns an error, `cmdutil.ClassifyError` maps it to the repository's exit-code model. This keeps command packages focused on returning meaningful errors rather than manually choosing process exit integers.

The important architectural rule is:

- Commands return errors.
- `run()` classifies errors.
- `main()` owns `os.Exit`.

That keeps exit policy centralized and testable.

---

## 7. Why This Shape Matters

This launch path supports the repo's main constraints:

- Commands that do not need the database avoid opening it.
- Workspace discovery is centralized in wiring and the SQLite adapter.
- Signal handling is process-wide and consistent across commands.
- CLI behavior stays in driving adapters, while the core and adapters remain independently testable.

For package placement and dependency-direction rules, see [Architecture](architecture.md). For contributor workflow and Make targets, see [Developer Setup](developer-setup.md).
