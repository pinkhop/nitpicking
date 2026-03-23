# Application Launch Process

This document traces every step from `os.Exit` down to the moment a command's
`run()` function begins executing. Where the launch path sets up machinery used
later during shutdown, those preparations are called out explicitly.

---

## 1. Process Entry — `cmd/np/main.go`

The binary's `main()` is a one-liner:

```go
func main() {
    os.Exit(int(app.Main()))
}
```

**Why this matters.** `os.Exit` is called here and *nowhere* else. Because
`os.Exit` terminates without running deferred functions, isolating it at the very
top of the call stack guarantees that every `defer` inside `app.Main()` — and
inside every function it transitively calls — will execute before the process
exits. This is the single design decision that makes deferred cleanup (closing
files, flushing logs, draining connections) reliable.

`app.Main()` returns a typed `ExitCode`, which converts to an `int` for the OS.

---

## 2. Application Orchestrator — `internal/app/app.go : Main()`

`Main()` has three responsibilities: construct the dependency graph, run the
command tree, and translate the outcome into an exit code.

```go
func Main() ExitCode {
    f := newFactory(appName, version)   // Step 2a: build dependency graph
    rootCmd := root.NewRootCmd(f)       // Step 3:  build the command tree
    err := rootCmd.Run(                 // Step 4:  execute the command
        context.Background(), os.Args,
    )
    if err == nil {
        return ExitOK
    }
    return classifyError(f.IOStreams.ErrOut, err, f.SignalCancelIsError)  // Step 5: classify errors
}
```

The `version` variable is set at build time via `-ldflags`:

```go
var version = "dev"
```

```bash
go build -ldflags "-X github.com/pinkhop/nitpicking/internal/app.version=1.2.3" -o dist/np ./cmd/np/
```

During development it defaults to `"dev"`.

---

## 2a. Factory Construction — `internal/app/app.go : newFactory()`

The Factory is the application's dependency injection container. It is built in a
specific order because later phases may reference earlier ones (e.g., the Logger
phase depends on IOStreams for TTY detection).

```go
func newFactory(appName, appVersion string) *cmdutil.Factory {
    f := &cmdutil.Factory{
        AppName:    appName,
        AppVersion: appVersion,
        BuildInfo:  cmdutil.ReadBuildInfo(),
    }

    // Phase 1: IOStreams — constructed eagerly (cheap, universally needed).
    f.IOStreams = iostreams.System()

    // Phase 2: Logger — constructed eagerly with a mutable LevelVar.
    f.LogLevel, f.Logger = newLogger(f.IOStreams)

    return f
}
```

### The Factory Struct

```go
type Factory struct {
    AppName    string                          // binary name for version output and messages
    AppVersion string                          // injected at build time via -ldflags
    BuildInfo  BuildInfo                       // VCS metadata (commit, timestamp, dirty flag)
    IOStreams   *iostreams.IOStreams            // eager — cheap, used by everything
    Logger     func() *slog.Logger             // eager — constructed once, mutable LevelVar
    LogLevel   *slog.LevelVar                  // controls Logger severity; set by root Before hook
    SignalCancelIsError bool                   // false = graceful shutdown (default)
}
```

The core principle: **function fields, not pre-constructed values.** Commands
only pay for dependencies they actually use; as the application grows, expensive
dependencies (HTTP clients, database pools) are added as function-typed fields
whose construction is deferred until first call. There is no app-wide
configuration — commands that need file-based config define their own types and
loading strategy.

### Phase 1: IOStreams

```go
f.IOStreams = iostreams.System()
```

Both IOStreams and Logger are *eager* dependencies — IOStreams is cheap (just
stat calls on file descriptors) and needed by virtually every code path,
including error reporting.

`System()` probes each standard file descriptor for TTY status:

```go
func System() *IOStreams {
    stdoutTTY := isTerminal(os.Stdout)
    stderrTTY := isTerminal(os.Stderr)
    return &IOStreams{
        In:           os.Stdin,
        Out:          os.Stdout,
        ErrOut:       os.Stderr,
        stdinIsTTY:   isTerminal(os.Stdin),
        stdoutIsTTY:  stdoutTTY,
        stderrIsTTY:  stderrTTY,
        colorEnabled: stdoutTTY,
    }
}
```

Terminal detection uses `os.File.Stat` and the `ModeCharDevice` bit — no cgo
or external dependencies:

```go
func isTerminal(f *os.File) bool {
    fi, err := f.Stat()
    return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
```

These TTY flags drive downstream decisions: color output, human-readable vs.
machine-parseable formatting, and whether interactive prompts are safe.

### Phase 2: Logger

```go
func newLogger(ios *iostreams.IOStreams) (*slog.LevelVar, func() *slog.Logger) {
    level := &slog.LevelVar{} // defaults to Info

    var handler slog.Handler
    if ios.IsStderrTTY() {
        handler = slog.NewTextHandler(ios.ErrOut, &slog.HandlerOptions{Level: level})
    } else {
        handler = slog.NewJSONHandler(ios.ErrOut, &slog.HandlerOptions{Level: level})
    }

    logger := slog.New(handler)
    return level, func() *slog.Logger { return logger }
}
```

The logger is constructed eagerly with a mutable `LevelVar`. The root command's
Before hook adjusts the level after flag parsing by calling `LevelVar.Set`.
Because the handler references the `LevelVar`, the change takes effect
immediately for all subsequent log calls. The handler selection — human-readable
text for terminals, structured JSON when piped — means the same binary works
for both interactive use and log-aggregation pipelines.

---

## 3. Command Tree Construction — `internal/cmd/root/root.go : NewRootCmd()`

With the Factory built, `Main()` constructs the command tree:

```go
rootCmd := root.NewRootCmd(f)
```

`NewRootCmd` creates the root `*cli.Command` and wires every subcommand:

```go
func NewRootCmd(f *cmdutil.Factory) *cli.Command {
    var env, logLevel string
    var noGops bool
    var gopsStarted bool
    var stopSignals func()

    return &cli.Command{
        Name:  "np",
        Usage: "A well-structured Go application quick-start",
        Flags: []cli.Flag{ /* env, log-level, no-gops */ },
        Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
            if err := setLogLevel(f.LogLevel, logLevel); err != nil {
                return ctx, err
            }
            logger := f.Logger()
            ctx = WithLogger(ctx, logger)

            // Signal handling — process-wide, one handler for all subcommands.
            ctx, stopSignals = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

            // Gops diagnostics agent.
            if !noGops {
                if err := agent.Listen(agent.Options{}); err != nil {
                    LoggerFrom(ctx).Warn("gops agent failed to start", "error", err)
                } else {
                    gopsStarted = true
                }
            }

            return ctx, nil
        },
        After: func(ctx context.Context, cmd *cli.Command) error {
            if gopsStarted {
                agent.Close()
            }
            if stopSignals != nil {
                stopSignals()
            }
            return nil
        },
        Commands: []*cli.Command{
            version.NewCmd(f),
        },
    }
}
```

Four things happen here:

1. **Subcommand registration.** Each leaf command's `NewCmd()` receives the
   Factory and returns a `*cli.Command`. The Factory is *not stored* on the
   command — each `NewCmd` extracts the specific dependencies it needs into its
   own Options struct. As the application grows, additional subcommands are
   registered in the `Commands` slice — including grouping nodes (pure parents
   with no Action of their own) for organizing related subcommands.

2. **The `Before` hook.** This runs before *every* subcommand, not just the
   root. It is the right place for cross-cutting, process-wide concerns:
   - Applies the `--log-level` flag to the Factory's `LevelVar`, adjusting
     logger severity for all subsequent log calls.
   - Stores the logger in the context via a typed key, making it accessible to
     subcommands without passing the Factory through every function signature.
   - Installs `signal.NotifyContext` for SIGINT/SIGTERM — every subcommand
     receives a signal-aware context automatically.
   - Starts the gops diagnostics agent (unless `--no-gops` is set).

3. **The `After` hook.** Tears down process-wide infrastructure started in
   Before: closes the gops agent and deregisters the signal handler. This
   runs after the command's Action returns, even if Action returned an error.

4. **Flag-to-Options wiring.** Inside each `NewCmd()`, flags use `Destination`
   to write directly into the Options struct. By the time the `Action` closure
   runs, the Options struct is fully populated.

### Context Key Pattern

The logger is stored in context using an unexported struct type as the key:

```go
type loggerKey struct{}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
    return context.WithValue(ctx, loggerKey{}, logger)
}

func LoggerFrom(ctx context.Context) *slog.Logger {
    if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
        return logger
    }
    return slog.Default()    // safe fallback for tests that skip Before
}
```

Using a struct type (rather than a string) for the key prevents collisions — no
other package can create a value of `loggerKey`.

---

## 4. Command Execution — `rootCmd.Run(context.Background(), os.Args)`

`urfave/cli/v3` takes over:

1. **Argument parsing.** The library matches `os.Args` against the command tree,
   identifies the target subcommand, and parses its flags. Flag values flow into
   `Destination` fields on the Options struct.

2. **Configuration layering.** `urfave/cli/v3` handles precedence natively:
   **flags > environment variables > defaults.** The `Sources`
   field on flags enables env var binding:

   ```go
   &cli.IntFlag{
       Name:        "port",
       Value:       8080,
       Sources:     cli.EnvVars("MYAPP_PORT"),
       Destination: &opts.Port,
   }
   ```

3. **`Before` hook execution.** The root command's `Before` fires, enriching the
   context (see step 3 above).

4. **`Action` execution.** The matched command's `Action` closure runs. Every
   command follows the same pattern: validate flags, configure (if needed),
   then call `run()`.

5. **`After` hook execution.** The root command's `After` fires, tearing down
   process-wide infrastructure (gops agent, signal handler).

### The Action Closure

Signal handling is managed by the root Before hook — every command receives a
signal-aware context automatically. The Action closure focuses on the command's
own concerns.

Here is a representative example of a simple command with flag validation:

```go
Action: func(ctx context.Context, cmd *cli.Command) error {
    if opts.Limit < 1 {
        return cmdutil.FlagErrorf("invalid value for --limit: %d", opts.Limit)
    }

    if len(runFn) > 0 && runFn[0] != nil {
        return runFn[0](ctx, opts)
    }
    return run(ctx, opts)
}
```

For infrastructure-heavy commands, a `configure` phase sits between validation
and execution (see the `configureFn` pattern in the design guide):

```go
Action: func(ctx context.Context, cmd *cli.Command) error {
    // 1. Validate flags
    // 2. Configure (build adapters, wire domain objects)
    // 3. Run
}
```

Every command's `Action` does these things in order:

1. **Flag validation.** Return a `FlagError` early if flags are invalid.

2. **Configuration (infrastructure-heavy commands only).** Build adapters and
   domain objects from Factory functions. Injectable via `configureFn` for
   testing.

3. **Dispatch to `run()`.** The `run()` function receives a fully-populated
   Options struct — it never reads flags from `*cli.Command`. The context
   carries the logger (from the `Before` hook) and supports cancellation via
   the signal-aware context (also from the `Before` hook).

For long-running services, the `run()` function uses the signal-cancellable
context to drive graceful shutdown. When `ctx.Done()` fires,
it creates a *separate* context from `context.Background()` for the shutdown
grace period — because the signal context is already cancelled, and passing it
to `srv.Shutdown()` would cause immediate cancellation:

```go
select {
case <-ctx.Done():
    logger.Info("shutdown signal received, draining connections")
case err := <-errCh:
    return fmt.Errorf("server error: %w", err)
}

shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
defer shutdownCancel()
if err := srv.Shutdown(shutdownCtx); err != nil {
    return fmt.Errorf("server shutdown: %w", err)
}
```

### The `runFn` Test Hook

Every command accepts an optional `runFn` parameter:

```go
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, *Options) error) *cli.Command
```

This enables two distinct testing strategies:

- **Flag parsing tests.** Pass a `runFn` that captures the Options struct and
  returns nil. The test exercises `urfave/cli`'s flag parsing, env var binding,
  and validation logic.
- **Business logic tests.** Construct the Options struct directly with stubs
  (e.g., `httpstub.Registry` at the transport level) and call `run()`, bypassing
  the command framework entirely.

---

## 5. Error Classification — `internal/app/app.go : classifyError()`

When a command returns a non-nil error, `Main()` maps it to an exit code:

```go
func classifyError(stderr io.Writer, err error, signalCancelIsError bool) ExitCode {
    switch {
    case errors.Is(err, cmdutil.ErrSilent):
        return ExitError

    case errors.Is(err, context.Canceled):
        if signalCancelIsError {
            return ExitCancel
        }
        return ExitOK

    default:
        if fe, ok := errors.AsType[*cmdutil.FlagError](err); ok {
            fmt.Fprintln(stderr, fe)
            return ExitError
        }
        fmt.Fprintln(stderr, err)
        return ExitError
    }
}
```

The classification uses:
- `errors.Is` for sentinel values (`ErrSilent`, `context.Canceled`)
- `errors.AsType[T]` (Go 1.26) for type-safe unwrapping of wrapped error chains
- The `signalCancelIsError` parameter (sourced from `Factory.SignalCancelIsError`)
  to decide whether `context.Canceled` is a graceful exit or an error

This is the single place where error types are translated into user-visible
messages and exit codes. Commands never call `os.Exit` — they return typed
errors and let this function decide the outcome.

### Exit Codes

| Code | Constant      | Meaning                                    |
|------|---------------|--------------------------------------------|
| 0    | `ExitOK`      | Command completed successfully             |
| 1    | `ExitError`   | General error (unclassified failures)      |
| 2    | `ExitCancel`  | Signal cancellation when `SignalCancelIsError` is true; otherwise exits 0 |
| 8    | `ExitPending` | Operation accepted but not yet complete    |

### Typed Errors

```go
var ErrSilent = errors.New("silent error")  // error already printed by command

type FlagError struct{ Err error }            // wraps with Unwrap() for error chains
```

`FlagErrorf` is a convenience constructor:

```go
func FlagErrorf(format string, args ...any) *FlagError {
    return &FlagError{Err: fmt.Errorf(format, args...)}
}
```

---

## Summary: Launch Sequence

```
os.Exit ← app.Main()
              │
              ├─ newFactory(appName, version)
              │     ├─ Phase 1: IOStreams (eager — System())
              │     └─ Phase 2: Logger (eager — mutable LevelVar, reads IOStreams for TTY)
              │
              ├─ root.NewRootCmd(f)
              │     ├─ Before hook: setLogLevel → Logger → context → signals → gops
              │     ├─ After hook: gops close → signal deregister
              │     └─ Subcommands: version (add more here as the app grows)
              │           └─ Each NewCmd(f) → Options struct + flag wiring
              │
              ├─ rootCmd.Run(context.Background(), os.Args)
              │     ├─ urfave/cli: parse args, match command, apply flag precedence
              │     ├─ Before hook fires: setLogLevel() → Logger → ctx → signals → gops
              │     ├─ Action fires:
              │     │     ├─ Validate flags (return FlagError on failure)
              │     │     ├─ Configure (if needed — infrastructure-heavy commands)
              │     │     └─ run(ctx, opts)
              │     ├─ After hook fires: gops close → signal deregister
              │     └─ Command returns error or nil
              │
              └─ classifyError(stderr, err, signalCancelIsError) → ExitCode
```
