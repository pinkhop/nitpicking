# Design Guide

This document explains the architectural decisions, patterns, and conventions used in this application template. It is informed by studying GitHub CLI (`cli/cli`) — a widely-recognized example of a well-structured, testable Go CLI — and adapts its patterns for use with [`urfave/cli/v3`](https://github.com/urfave/cli) instead of Cobra/Viper.

The template supports two application modes:
- **CLI tools**: short-lived commands that run, produce output, and exit (like `gh`, `kubectl`, `terraform`)
- **Long-running services**: daemons, HTTP servers, queue workers, and scheduled jobs where the CLI is the entry point for starting and configuring the service

Both modes share the same dependency injection, configuration, and error handling patterns. The difference is in lifecycle management: CLI tools exit after a command completes; services run until signalled to stop.

## Table of Contents

1. [Architectural Philosophy](#1-architectural-philosophy)
2. [Project Structure](#2-project-structure)
3. [The Thin Main](#3-the-thin-main)
4. [The Factory: Lazy Dependency Injection](#4-the-factory-lazy-dependency-injection)
5. [IOStreams: Terminal Abstraction for CLI Mode](#5-iostreams-terminal-abstraction-for-cli-mode)
6. [Command Structure with urfave/cli/v3](#6-command-structure-with-urfavecliv3)
7. [The Options Struct Pattern](#7-the-options-struct-pattern)
8. [Commands with Substantial Infrastructure](#8-commands-with-substantial-infrastructure)
9. [Configuration Layering](#9-configuration-layering)
10. [Signal Handling & Graceful Shutdown](#10-signal-handling--graceful-shutdown)
11. [Structured Logging with slog](#11-structured-logging-with-slog)
12. [Error Handling & Exit Codes](#12-error-handling--exit-codes)
13. [Diagnostic Servers (pprof)](#13-diagnostic-servers-pprof)
14. [Testing Patterns](#14-testing-patterns)
15. [Application Launch Process](#15-application-launch-process)

---

## 1. Architectural Philosophy

Three principles drive every design decision in this template:

### Pay for what you use

A CLI application has dozens of commands, but each invocation runs exactly one. A command like `version` should not pay the cost of constructing an HTTP client, reading git state, or connecting to a database. Dependencies should be constructed only when — and if — the executing command actually needs them.

This is accomplished by representing dependencies as **functions that return values** rather than pre-constructed values. The HTTP client function is allocated at startup (cheap — it's just a closure); the HTTP client itself is constructed only when a command calls that function.

### Testability through substitution, not mocking

Every external dependency — HTTP, database, filesystem, terminal, clock — should be substitutable in tests without mocking frameworks. The template achieves this through:
- **Function fields** on the Factory (swap the function, swap the dependency)
- **Interface-based I/O** (IOStreams backed by buffers in tests)
- **Transport-layer HTTP stubs** (substitute `http.RoundTripper`, not API client methods)
- **Options structs** that decouple flag parsing from business logic

The testing strategy is: **verify what the code does (observable outcomes), not how it does it (interaction sequences).** Use stubs, fakes, and spies — not mocks. Stubs return canned data. Fakes implement real behavior with in-memory state. Spies record what happened for later assertion. Mocks prescribe expected interactions and fail if the code doesn't call things in the right order — this couples tests to implementation details and makes refactoring painful.

### Commands own their dependencies

Each command declares exactly which dependencies it needs via an Options struct. The command's package imports reveal its dependency graph. The Factory provides everything; each command takes only what it uses. This means:
- Tests only provide what the command under test actually calls
- Adding a new dependency to one command doesn't affect any other command's tests
- A command's Options struct is a readable manifest of its external dependencies

## 2. Project Structure

```
np/
├── cmd/
│   └── np/
│       └── main.go              # Thin shell: calls internal/app.Main(), os.Exit()
├── internal/
│   ├── app/
│   │   ├── app.go               # Main() → exitCode; factory construction, dispatch, error classification
│   │   ├── exit_codes.go        # Typed exit codes
│   │   └── name_version.go     # App name and version (injected via -ldflags)
│   ├── cmd/
│   │   ├── root/
│   │   │   └── root.go          # Root command: global flags, Before hook (auth gate, etc.)
│   │   └── version/
│   │       └── version.go       # Example: simple command, no dependencies beyond IOStreams
│   ├── cmdutil/
│   │   ├── factory.go           # Factory struct definition
│   │   ├── errors.go            # Typed errors (ErrSilent, FlagError)
│   │   └── flags.go             # Shared flag helpers
│   └── iostreams/
│       ├── iostreams.go         # IOStreams struct, System(), Test()
│       └── color.go             # Color scheme, TTY-aware formatting
├── go.mod
└── go.sum
```

As the application grows, new commands and shared packages are added following the conventions below.

### Why everything is in `internal/`

Go's `internal/` directory has compiler-enforced import restrictions: packages under `internal/` can only be imported by code rooted at the parent of `internal/`. For a CLI application — where the compiled binary is the product and no external Go module imports your packages — every package belongs in `internal/`. This gives you complete freedom to restructure without breaking external consumers, because there are no external consumers.

The `pkg/` convention (popularized by Kubernetes-era projects) signals "these packages are library-quality and could be imported externally". For a CLI application, that signal is misleading. If you later extract a reusable library (as GitHub CLI did with `go-gh`), create a separate module for it rather than exposing CLI internals.

### Package-per-command isolation

Each command lives in its own Go package (e.g., `internal/cmd/widget/list/`, `internal/cmd/widget/create/`). This is not just an organizational preference — Go's package system enforces boundaries:

- Commands can only access what other packages export
- When two commands need the same logic, it must live in a `shared/` package with a deliberate public API
- Dependencies between commands are visible in import statements
- A command's test file can only test the package's exported API (or its package-internal functions)

When a parent command has subcommands, shared logic (display formatting, common API calls) lives in a sibling `shared/` package. This makes cross-command dependencies explicit rather than hidden in a monolithic package.

## 3. The Thin Main

```go
// cmd/np/main.go
package main

import (
    "os"

    "mymodule/internal/app"
)

func main() {
    os.Exit(int(app.Main()))
}
```

This is the **only place `os.Exit` is called.** Everything else returns a typed exit code.

### Why this matters

1. **`os.Exit` terminates the process immediately.** Go's `defer` statements don't run. If `os.Exit` were called deeper in the stack — say, inside a command handler — deferred cleanup (flushing logs, closing database connections, cancelling background goroutines) would be silently skipped.

2. **`os.Exit` is untestable.** Any test that exercises code calling `os.Exit` also terminates the test process. By pushing `os.Exit` to the thinnest possible shell, the entire remaining launch path — including error classification — becomes a testable function that returns a value.

3. **Typed exit codes carry meaning.** Rather than scattering `os.Exit(1)` calls throughout the codebase (where every failure looks the same), `app.Main()` returns a semantically meaningful `exitCode` value that the caller maps to an integer. The error classification logic is centralized in one place.

## 4. The Factory: Lazy Dependency Injection

The Factory is the central design element. It is a struct whose fields are **functions that return dependencies** rather than the dependencies themselves.

```go
// internal/cmdutil/factory.go
package cmdutil

import (
    "log/slog"
    "net/http"

    "mymodule/internal/iostreams"
)

// Factory provides lazy-loaded, substitutable dependencies to commands.
// Function fields are not called until a command actually needs the dependency,
// and they can be replaced in tests with trivial stubs.
type Factory struct {
    // AppVersion is the build version, injected at compile time via -ldflags.
    AppVersion string

    // IOStreams provides abstracted I/O with TTY awareness, color control,
    // and pager support. Constructed eagerly because it's needed by almost
    // everything and has no expensive initialization.
    IOStreams *iostreams.IOStreams

    // Logger returns a configured slog.Logger. The function form allows
    // commands to receive a logger with command-specific attributes
    // (e.g., command name) without constructing it at startup.
    Logger func() *slog.Logger

    // HttpClient returns an HTTP client configured with authentication,
    // logging middleware, and retry behavior. Not constructed until a
    // command actually needs to make HTTP requests.
    HttpClient func() (*http.Client, error)

    // Extend with your own dependencies:
    // DBConn      func() (*sql.DB, error)
    // CacheClient func() (*redis.Client, error)
    // ...
}
```

### Why function fields instead of values

Consider what happens when a Factory holds a pre-constructed `*http.Client`:

```go
// Eager construction — every command pays for every dependency
type Factory struct {
    HttpClient *http.Client  // constructed at startup, even if unused
}
```

If constructing the HTTP client requires reading configuration, authenticating, and setting up middleware, every command invocation pays that cost — even `np version`, which doesn't make HTTP requests. With function fields:

```go
// Lazy construction — pay only for what you use
type Factory struct {
    HttpClient func() (*http.Client, error)  // constructed on first call
}
```

The function is allocated at startup (a closure — essentially free), but the HTTP client is only constructed when a command calls `f.HttpClient()`. A command like `version` never calls it, so it never pays the cost.

### Why function fields instead of interfaces

You might wonder: why not use interfaces for dependency injection?

```go
type HTTPClientProvider interface {
    HTTPClient() (*http.Client, error)
}
```

Interfaces work, but function fields are simpler for this use case:
- No interface to define, no implementation struct to write
- Test stubs are plain function literals — `func() (*http.Client, error) { return myStub, nil }`
- Adding a new dependency is one struct field, not a new interface + implementation + registration

Interfaces shine when you have multiple implementations with shared contracts. Factory dependencies are simpler: "give me a thing, or an error". A function is the most direct encoding of that contract.

### Memoization for expensive dependencies

Some dependencies should be constructed once and reused. A generic `Memoized` wrapper handles this:

```go
// Memoized wraps any loader with sync.Once semantics.
loadDB := config.Memoized(config.FileLoader[DBConfig](path))
cfg, err := loadDB()  // reads file on first call, returns cached result thereafter
```

The pattern works for any `func() (*T, error)` — not just configuration. For signal-triggered refresh (e.g., SIGHUP), use a reloadable wrapper instead.

### Refreshable dependencies for long-running services

For long-running services, some dependencies must be refreshable — database credentials rotate, feature flags change, certificates renew. The function-field pattern handles this naturally:

```go
// A Factory field that re-reads credentials on each call
f.DBConn = func() (*sql.DB, error) {
    creds, err := secretManager.GetCurrent("db-credentials")
    if err != nil {
        return nil, fmt.Errorf("fetching DB credentials: %w", err)
    }
    return openDBWithCreds(creds)
}
```

For dependencies where the refresh cost is high (opening a new DB connection on every call is too expensive), use a **time-bounded cache** instead of `sync.Once`:

```go
// Refreshes the connection at most once per interval, reusing it otherwise.
// When credentials rotate, the next call after the cache expires will pick
// up the new credentials.
func refreshableDBConn(
    secretMgr SecretManager,
    refreshInterval time.Duration,
) func() (*sql.DB, error) {
    var (
        mu      sync.Mutex
        conn    *sql.DB
        expires time.Time
    )
    return func() (*sql.DB, error) {
        mu.Lock()
        defer mu.Unlock()

        if conn != nil && time.Now().Before(expires) {
            return conn, nil
        }

        creds, err := secretMgr.GetCurrent("db-credentials")
        if err != nil {
            // If refresh fails but we have an existing connection, keep using it.
            // Log the error so operators can investigate.
            if conn != nil {
                slog.Warn("credential refresh failed, reusing existing connection",
                    "error", err)
                return conn, nil
            }
            return nil, fmt.Errorf("fetching DB credentials: %w", err)
        }

        newConn, err := openDBWithCreds(creds)
        if err != nil {
            if conn != nil {
                slog.Warn("new connection failed, reusing existing connection",
                    "error", err)
                return conn, nil
            }
            return nil, fmt.Errorf("opening DB connection: %w", err)
        }

        // Close the old connection after successfully opening a new one
        if conn != nil {
            conn.Close()
        }
        conn = newConn
        expires = time.Now().Add(refreshInterval)
        return conn, nil
    }
}
```

The key insight: **the consumer doesn't know or care whether the dependency is memoized, refreshable, or constructed fresh each time.** The function signature `func() (*sql.DB, error)` is the same in all cases. The caching/refresh strategy is an implementation detail of the Factory constructor, invisible to commands.

### Live configuration: the discipline for commands

The Factory's function fields have a dual nature: **lazy initialization** for startup performance *and* **live configuration** for long-running applications. Short-lived CLI commands (like `version` or a one-shot export) can safely call a Factory function once — the pattern adds no overhead, but it doesn't provide its full benefit either.

For long-running commands — HTTP servers, queue workers, batch loops — the critical discipline is: **call Factory functions per unit of work, not once at startup.** Don't store Factory return values in long-lived variables. Each call may return a resource built from the latest configuration — rotated credentials, updated feature flags, recycled connection pools.

```go
// WRONG — caches the connection at startup; won't pick up rotated credentials.
func run(ctx context.Context, opts *ServeOptions) error {
    db, err := opts.DBConn()  // called once
    if err != nil {
        return err
    }
    for msg := range opts.Queue.Consume(ctx) {
        process(ctx, db, msg)  // stale connection after credential rotation
    }
}

// RIGHT — calls per unit of work; each call may return a fresh resource.
func run(ctx context.Context, opts *ServeOptions) error {
    for msg := range opts.Queue.Consume(ctx) {
        db, err := opts.DBConn()  // called per message
        if err != nil {
            return err
        }
        process(ctx, db, msg)  // always uses current credentials
    }
}
```

Whether the function returns a cached connection, a refreshed one, or a fresh one each time is invisible to the command. The Factory constructor decides the caching strategy; the command simply calls the function when it needs the dependency.

### Constructing the Factory

The Factory is constructed in `app.Main()` by a dedicated constructor function. Each field is wired in a specific order based on its dependencies:

```go
// internal/app/factory.go
func newFactory(appVersion string) *cmdutil.Factory {
    f := &cmdutil.Factory{
        AppVersion: appVersion,
    }

    // Phase 1: IOStreams — cheap, needed by virtually every command.
    f.IOStreams = iostreams.System()

    // Phase 2: Logger — constructed eagerly with a mutable LevelVar.
    f.LogLevel, f.Logger = newLogger(f.IOStreams)

    // Phase 3: HttpClient — closure, constructed on demand.
    f.HttpClient = newHTTPClientFunc()

    return f
}
```

Each `new*Func` returns a closure that captures `f`. Because `f` is a pointer, the closure sees the fully-constructed Factory — including fields assigned after the closure was created. This is why the ordering of assignments matters for eager fields (IOStreams) but not for deferred fields (HttpClient).

## 5. IOStreams: Terminal Abstraction for CLI Mode

CLI tools need to behave differently depending on whether they're running in an interactive terminal or being piped/scripted. IOStreams centralizes this concern.

```go
// internal/iostreams/iostreams.go
package iostreams

import (
    "bytes"
    "io"
    "os"
)

// IOStreams abstracts standard I/O with TTY awareness, color control,
// and pager support. Commands use this instead of os.Stdin/os.Stdout/os.Stderr
// directly, which allows tests to capture output and control terminal behavior.
type IOStreams struct {
    In     io.ReadCloser
    Out    io.Writer
    ErrOut io.Writer

    stdinIsTTY  bool
    stdoutIsTTY bool
    stderrIsTTY bool

    colorEnabled bool
    pagerCommand string
    // ... other terminal state
}

// System returns IOStreams connected to the real standard file descriptors
// with TTY detection, color support detection, and platform-specific
// ANSI handling (Windows).
func System() *IOStreams {
    // Probe real file descriptors for TTY status
    // Detect color support from terminal capabilities
    // Set up Windows ANSI translation if needed
    // ...
}

// Test returns IOStreams backed by in-memory buffers, suitable for tests.
// The returned buffers let tests inspect what a command wrote to stdout
// and stderr without touching the real terminal.
func Test() (streams *IOStreams, stdin *bytes.Buffer, stdout *bytes.Buffer, stderr *bytes.Buffer) {
    in := &bytes.Buffer{}
    out := &bytes.Buffer{}
    errOut := &bytes.Buffer{}
    return &IOStreams{
        In:     io.NopCloser(in),
        Out:    out,
        ErrOut: errOut,
    }, in, out, errOut
}

// IsStdoutTTY reports whether stdout is connected to an interactive terminal.
// Commands use this to decide between human-friendly output (colors, tables,
// relative timestamps) and machine-parseable output (plain text, absolute
// timestamps, tab-separated values).
func (s *IOStreams) IsStdoutTTY() bool {
    return s.stdoutIsTTY
}

// CanPrompt reports whether the application can prompt the user for input.
// This requires both stdin and stdout to be TTYs — if either is piped,
// interactive prompts would hang or produce garbled output.
func (s *IOStreams) CanPrompt() bool {
    return s.stdinIsTTY && s.stdoutIsTTY
}
```

### Why IOStreams matters

Without IOStreams, commands write directly to `os.Stdout`. This creates three problems:

1. **Tests can't capture output** without redirecting the process's actual file descriptors — which is fragile, global, and doesn't work with parallel tests.

2. **TTY decisions are scattered.** Every command independently checks `isatty.IsTerminal(os.Stdout.Fd())`, and inconsistencies creep in — one command shows colors when piped, another doesn't.

3. **Pager support requires swapping stdout.** When a command starts a pager (like `less`), it needs to redirect its output through a pipe to the pager process. With direct `os.Stdout` usage, there's no place to intercept this.

IOStreams solves all three: tests use `Test()` for in-memory buffers, TTY state is detected once and queried consistently, and the pager can swap the `Out` writer transparently.

### For long-running services

Long-running services generally don't need IOStreams — they log to structured loggers, not terminals. A service command might use `f.IOStreams.ErrOut` for startup messages ("Listening on :8080") and then switch to `slog` for all runtime logging. The IOStreams abstraction still helps during the brief interactive phase (startup, shutdown messages) and keeps the pattern consistent with CLI commands.

## 6. Command Structure with urfave/cli/v3

### Why urfave/cli/v3 instead of Cobra

Both are mature, well-maintained command frameworks. The choice comes down to API style:

**Cobra** separates command definition from flag binding. You create a `cobra.Command`, then imperatively call `cmd.Flags().StringVarP(...)` to bind flags. Configuration from environment variables requires Viper — a separate library with its own initialization lifecycle.

**urfave/cli/v3** unifies flags, environment variables, and file-based value sources in a single declarative struct:

```go
&cli.StringFlag{
    Name:    "api-url",
    Value:   "https://api.example.com",
    Usage:   "Base URL for the API",
    Sources: cli.EnvVars("MYAPP_API_URL"),
}
```

The `Sources` field defines a precedence chain: the flag value (if provided on the command line) takes priority over the environment variable, which takes priority over the `Value` default. This layering — flags > env > default — is built into the framework rather than requiring manual Viper wiring.

Additionally, `urfave/cli/v3` passes `context.Context` through the entire command chain. The root command's `Before` hook can enrich the context (injecting a logger, a trace ID, a cancellation signal), and every subcommand receives it. Cobra added context support later, and it's less deeply integrated into the API.

### Mapping gh's patterns to urfave/cli/v3

| GitHub CLI (Cobra) | Template (urfave/cli/v3) | Notes |
|---|---|---|
| `cobra.Command.RunE` | `cli.Command.Action` | Both are the command's main function. urfave/cli/v3's signature is `func(context.Context, *cli.Command) error`. |
| `cobra.Command.PersistentPreRunE` | `cli.Command.Before` | Both run before the command's action. urfave/cli/v3's `Before` returns `(context.Context, error)`, allowing context enrichment — useful for injecting auth tokens, loggers, or trace IDs. |
| `cobra.Command.PersistentPostRunE` | `cli.Command.After` | Both run after the command's action, even if the action returned an error. Useful for cleanup. |
| `cmd.Flags().StringVarP(...)` | `cli.StringFlag{...}` in `Flags` slice | urfave/cli/v3 is declarative; flags are part of the command struct. |
| Viper env binding | `Sources: cli.EnvVars(...)` | Built into the flag definition, no separate library. |

### Root command structure

```go
// internal/cmd/root/root.go
package root

import (
    "context"

    "github.com/urfave/cli/v3"

    "mymodule/internal/cmd/version"
    "mymodule/internal/cmdutil"
)

// NewRootCmd constructs the root command with all subcommands registered.
// The Factory is passed to each subcommand constructor, allowing them to
// extract only the dependencies they need.
func NewRootCmd(f *cmdutil.Factory) *cli.Command {
    return &cli.Command{
        Name:    "np",
        Usage:   "A well-structured CLI application",
        Version: f.AppVersion,
        Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
            // The Before hook on the root command runs before EVERY subcommand.
            // Use it for cross-cutting concerns:
            //   - Authentication checks
            //   - Global flag processing
            //   - Context enrichment (logger, trace ID)
            //
            // Return an enriched context; subcommands receive it automatically.
            logger := f.Logger()
            ctx = WithLogger(ctx, logger)
            return ctx, nil
        },
        Commands: []*cli.Command{
            version.NewCmd(f),
            // Register additional commands here as the application grows.
            // For grouped subcommands (e.g., "widget list", "widget create"),
            // add a parent command node:
            //
            // {
            //     Name:  "widget",
            //     Usage: "Manage widgets",
            //     Commands: []*cli.Command{
            //         widgetList.NewCmd(f),
            //         widgetCreate.NewCmd(f),
            //     },
            // },
        },
    }
}
```

### Leaf command structure

Every command follows the same three-part pattern:

1. **Options struct** — holds dependencies (from the Factory) and user input (from flags)
2. **`NewCmd()` constructor** — builds the `cli.Command`, wires flags into the Options struct
3. **`run()` function** — implements the command's business logic

The following example illustrates the pattern for a hypothetical command that calls an HTTP API:

```go
// internal/cmd/widget/list/list.go
package list

import (
    "context"
    "fmt"
    "net/http"

    "github.com/urfave/cli/v3"

    "mymodule/internal/cmdutil"
    "mymodule/internal/iostreams"
)

// ListOptions holds everything the list command needs to execute.
// Dependencies come from the Factory; user input comes from flags.
//
// This struct serves as a manifest: reading it tells you exactly
// what external systems this command touches.
type ListOptions struct {
    // Dependencies — extracted from Factory.
    // Only the dependencies this specific command uses.
    IO         *iostreams.IOStreams
    HttpClient func() (*http.Client, error)

    // User input — populated from flags in NewCmd.
    Limit  int
    Format string
}

// NewCmd constructs the urfave/cli Command for "widget list".
// The Factory provides dependencies; flags provide user input.
// The optional runFn parameter allows tests to intercept execution
// (see Section 14: Testing Patterns).
func NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, *ListOptions) error) *cli.Command {
    opts := &ListOptions{
        IO:         f.IOStreams,
        HttpClient: f.HttpClient,
    }

    return &cli.Command{
        Name:    "list",
        Aliases: []string{"ls"},
        Usage:   "List all widgets",
        Flags: []cli.Flag{
            &cli.IntFlag{
                Name:    "limit",
                Aliases: []string{"L"},
                Value:   30,
                Usage:   "Maximum number of widgets to show",
                Sources: cli.EnvVars("MYAPP_WIDGET_LIMIT"),
                Destination: &opts.Limit,
            },
            &cli.StringFlag{
                Name:        "format",
                Value:       "table",
                Usage:       "Output format: table, json, csv",
                Destination: &opts.Format,
            },
        },
        Action: func(ctx context.Context, cmd *cli.Command) error {
            // Flag validation that depends on multiple flags or
            // values not expressible with the Validator field
            if opts.Limit < 1 {
                return fmt.Errorf("invalid value for --limit: %d", opts.Limit)
            }

            // Test hook: if a test-provided runFn was passed, call it
            // instead of the real implementation. This allows tests to
            // verify flag parsing independently from business logic.
            if len(runFn) > 0 && runFn[0] != nil {
                return runFn[0](ctx, opts)
            }
            return run(ctx, opts)
        },
    }
}

// run implements the widget list business logic.
// It receives a fully-populated Options struct — all flag parsing
// and validation has already happened.
func run(ctx context.Context, opts *ListOptions) error {
    client, err := opts.HttpClient()
    if err != nil {
        return err
    }

    // ... use client to fetch widgets, format output to opts.IO.Out ...
    _ = client
    fmt.Fprintln(opts.IO.Out, "listing widgets...")
    return nil
}
```

### Why the Options struct exists

Without the Options struct, the command's `Action` function would need to extract dependencies from the Factory and flag values from the `*cli.Command` — mixing dependency wiring with business logic:

```go
// Without Options struct — dependency wiring and logic are entangled
Action: func(ctx context.Context, cmd *cli.Command) error {
    client, err := f.HttpClient()  // wiring
    limit := cmd.Int("limit")      // wiring
    format := cmd.String("format") // wiring
    // ... now the actual logic starts ...
}
```

With the Options struct, the `Action` function just validates and dispatches; the `run` function receives a clean, fully-resolved set of inputs:

```go
// With Options struct — clean separation
Action: func(ctx context.Context, cmd *cli.Command) error {
    // validation only
    return run(ctx, opts)  // opts is already populated via Destination
}
```

This separation pays for itself in testing (Section 14) — you can test flag parsing and business logic independently.

## 7. The Options Struct Pattern

This pattern deserves its own section because it is the connective tissue between the Factory, the command framework, and the testing strategy.

### Rules for Options structs

1. **Include only the Factory fields this command uses.** If the command doesn't make HTTP requests, don't include `HttpClient`. This keeps tests focused — you only need to provide what the command actually calls.

2. **Use function types for expensive dependencies.** `HttpClient func() (*http.Client, error)` — not `HttpClient *http.Client`. The function is what enables lazy construction and test substitution.

3. **Use value types for cheap, always-available dependencies.** `IO *iostreams.IOStreams` — not `IO func() *iostreams.IOStreams`. IOStreams is constructed eagerly and has no failure mode; wrapping it in a function adds ceremony without benefit.

4. **Include user input fields alongside dependencies.** `Limit int`, `Format string` — these are populated from flags. Having them in the same struct means `run()` receives everything it needs in one argument.

5. **Time-dependent commands should accept a `Now` function.** If a command formats relative timestamps ("3 hours ago"), include `Now func() time.Time` so tests can provide a fixed clock:

    ```go
    type ListOptions struct {
        // ...
        Now func() time.Time  // defaults to time.Now in NewCmd
    }
    ```

## 8. Commands with Substantial Infrastructure

Not every command is a thin wrapper around an API call. Commands with substantial infrastructure setup — database connection pools, adapter construction, domain object wiring — need a clean phase boundary between "setting up" and "running" that maintains testability.

### The problem

The template's default command pattern assumes setup is trivial:

```go
Action: func(ctx context.Context, cmd *cli.Command) error {
    // validate flags
    return run(ctx, opts)
}
```

For infrastructure-heavy commands, if `run()` builds adapters internally, tests cannot inject fakes at the adapter level without reaching into `run()`'s internals. The `runFn` test hook lets you replace `run()` entirely (for flag-parsing tests) or call `run()` directly (for logic tests), but there is no seam for "test the command logic with fake adapters without also testing adapter construction."

### Solution: injectable `configureFn`

Add a `configureFn` parameter to `NewCmd()` alongside the existing `runFn`, giving commands with substantial setup a testable configuration phase:

```go
func NewCmd(f *cmdutil.Factory, overrides ...Override) *cli.Command {
    opts := &Options{ /* ... wire from Factory ... */ }

    return &cli.Command{
        Flags: []cli.Flag{ /* ... */ },
        Action: func(ctx context.Context, cmd *cli.Command) error {
            // 1. Validate flags (return FlagError on failure)

            // 2. Configure (injectable for tests)
            cfgFn := configure  // default: real configuration
            if len(overrides) > 0 && overrides[0].ConfigureFn != nil {
                cfgFn = overrides[0].ConfigureFn
            }
            if err := cfgFn(ctx, f, opts); err != nil {
                return err
            }

            // 3. Run (injectable for tests)
            runFn := run  // default: real execution
            if len(overrides) > 0 && overrides[0].RunFn != nil {
                runFn = overrides[0].RunFn
            }
            return runFn(ctx, opts)
        },
    }
}

// Override provides test injection points for commands with substantial setup.
type Override struct {
    ConfigureFn func(context.Context, *cmdutil.Factory, *Options) error
    RunFn       func(context.Context, *Options) error
}
```

The `configure` function takes Factory functions + flag values and populates the remaining fields on Options (adapters, domain objects, cleanup functions). It is the clean "still setting up" phase.

### Three testing strategies

This pattern yields three distinct testing strategies:

1. **Flag parsing tests** — inject a `ConfigureFn` that captures Options and returns nil (never reaches `run`).
2. **Configuration logic tests** — call `ExportConfigure` directly with a fake Factory, or inject a `ConfigureFn` spy.
3. **Business logic tests** — construct Options with fakes, call `ExportRun` directly (bypasses both Action and configure).

### When to use this pattern

Not every command needs `configureFn`. Simple commands (like `version`) that have no infrastructure setup should continue using the existing `runFn`-only pattern. Use `configureFn` when the Options struct carries adapter interfaces or domain objects that require non-trivial construction — database pools, message queue consumers, external service clients that need configuration beyond what the Factory provides directly.

---

## 9. Configuration Layering

### Precedence: flags > environment > config file > defaults

`urfave/cli/v3` handles the first three layers natively through the `Sources` field on flags:

```go
&cli.StringFlag{
    Name:  "database-url",
    Value: "postgres://localhost:5432/np",  // default
    Usage: "Database connection URL",
    Sources: cli.NewValueSourceChain(
        cli.EnvVars("YOURAPP_DATABASE_URL"),   // env var
        // file-based sources can be added here
    ),
    // CLI flag --database-url overrides all of the above
}
```

The precedence is: explicit flag on the command line > first matching environment variable > first matching file source > the `Value` default. This is handled by the framework — you don't need to implement the layering yourself.

### When to use a config file

Config files are optional and should be introduced only when they provide clear value over flags and environment variables. Good reasons:

- **Complex structured configuration** that doesn't map well to flat key-value flags (nested objects, lists of objects)
- **Shared team configuration** checked into a repository (`.np.yaml` at the repo root)
- **Multiple environments** where a file per environment is more manageable than dozens of env vars

Bad reasons:
- "Applications should have config files" — they shouldn't by default
- Simple key-value configuration — use env vars
- Secrets — use a secret manager, never a config file

### Per-command config with composable primitives

There is no app-wide `AppConfig`. Commands that need file-based configuration define their own config types and accept the path via a command-level flag. When building configuration loading, consider three composable primitives:

- **File loader** — stateless YAML/JSON file reader for a typed config struct
- **Memoized loader** — wraps a loader with `sync.Once` semantics (read once, cache forever)
- **Reloadable loader** — signal-triggered refresh (e.g., SIGHUP) for long-running services

Config file paths come from command-level flags, not the Factory. Each command owns its own configuration lifecycle.

## 10. Signal Handling & Graceful Shutdown

### Process-wide signal handling in the root Before/After hooks

Signal handling is a process-wide lifecycle concern — one signal handler per process, since multiple `signal.NotifyContext` calls on the same signals interfere with each other. The root command's Before hook installs `signal.NotifyContext` for SIGINT and SIGTERM, and the After hook deregisters it. This follows the same closure-variable pattern used by gops and (on the OpenTelemetry branch) the root span:

```go
var stopSignals func()

Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
    // ... setLogLevel, WithLogger ...
    ctx, stopSignals = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
    // ... gops ...
    return ctx, nil
},

After: func(ctx context.Context, cmd *cli.Command) error {
    // ... gops cleanup ...
    if stopSignals != nil {
        stopSignals()
    }
    return nil
},
```

Every subcommand receives a signal-aware context automatically — no per-command signal setup boilerplate. Even short-lived commands can block on DNS lookups, slow APIs, or database writes; the user should always be able to interrupt with Ctrl+C.

Tests that call `run()` directly (via `ExportRun`) bypass Before entirely and pass their own cancellable context. This keeps `run()` free of OS-signal goroutines, which is required for `testing/synctest` compatibility.

The one exception where commands handle signals locally is terminal state restoration — if a command activates an alternate screen buffer or raw terminal mode, it must restore the terminal on interrupt.

### Long-running services: context-based shutdown

For services, graceful shutdown builds on the signal-aware context provided by the root Before hook:

1. **The root Before hook traps signals** using `signal.NotifyContext`
2. **The cancellable context propagates** through the entire call chain
3. **Components drain via `ctx.Done()`** — HTTP servers call `Shutdown()`, queue consumers stop accepting new work, background goroutines exit their loops

Here is an example of a service command implementing graceful shutdown:

```go
type ServeOptions struct {
    IO     *iostreams.IOStreams
    Logger func() *slog.Logger
    Port   int
}

func run(ctx context.Context, opts *ServeOptions) error {
    logger := opts.Logger()

    // Build the HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    srv := &http.Server{
        Addr:    fmt.Sprintf(":%d", opts.Port),
        Handler: mux,
    }

    // Start the server in a goroutine
    errCh := make(chan error, 1)
    go func() {
        logger.Info("server starting", "port", opts.Port)
        if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
        close(errCh)
    }()

    // Wait for either a signal or a server error
    select {
    case <-ctx.Done():
        logger.Info("shutdown signal received, draining connections")
    case err := <-errCh:
        return fmt.Errorf("server error: %w", err)
    }

    // Give in-flight requests time to complete.
    // Use a separate context because the signal context is already cancelled.
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer shutdownCancel()

    if err := srv.Shutdown(shutdownCtx); err != nil {
        return fmt.Errorf("server shutdown: %w", err)
    }

    logger.Info("server stopped cleanly")
    return nil
}
```

### Why `signal.NotifyContext` instead of `signal.Notify`

`signal.NotifyContext` returns a context that is cancelled when the signal arrives. This integrates naturally with Go's `context.Context` propagation — every function in the call chain that accepts a context automatically becomes cancellation-aware. You don't need to pass a separate `done` channel alongside the context.

The important subtlety: after the signal context is cancelled, you need a **separate, fresh context** for the shutdown grace period. The shutdown itself takes time (draining connections, flushing buffers), and that work needs a context that hasn't been cancelled yet. That's why the example creates `shutdownCtx` from `context.Background()` rather than from the cancelled `ctx`.

## 11. Structured Logging with slog

### Why slog

Go 1.21 introduced `log/slog` in the standard library. For a template intended to onboard teams of varying experience, a stdlib solution avoids a dependency decision and ensures every team member can read the documentation on `pkg.go.dev` without learning a third-party API.

`slog` provides:
- Structured key-value pairs on every log entry
- Configurable log levels (Debug, Info, Warn, Error)
- Swappable handlers (JSON for production, text for development)
- Context integration (pass loggers through context)

### Wiring the logger through the Factory

```go
// In the Factory constructor
f.Logger = newLoggerFunc(f)

func newLoggerFunc(f *cmdutil.Factory) func() *slog.Logger {
    var logger *slog.Logger
    var once sync.Once
    return func() *slog.Logger {
        once.Do(func() {
            // Use JSON output for non-TTY (production/piped), text for interactive use
            var handler slog.Handler
            if f.IOStreams.IsStderrTTY() {
                handler = slog.NewTextHandler(f.IOStreams.ErrOut, &slog.HandlerOptions{
                    Level: level,
                })
            } else {
                handler = slog.NewJSONHandler(f.IOStreams.ErrOut, &slog.HandlerOptions{
                    Level: level,
                })
            }
            logger = slog.New(handler)
        })
        return logger
    }
}
```

### Logger in commands

Commands receive the logger through the Factory and can add command-specific attributes:

```go
func run(ctx context.Context, opts *ListOptions) error {
    logger := opts.Logger().With("command", "widget-list")
    logger.Info("listing widgets", "limit", opts.Limit)
    // ...
}
```

### Logger in tests

Tests provide a discarding logger (or one that writes to a buffer for assertion):

```go
factory := &cmdutil.Factory{
    Logger: func() *slog.Logger {
        return slog.New(slog.NewTextHandler(io.Discard, nil))
    },
}
```

## 12. Error Handling & Exit Codes

### Typed exit codes

```go
// internal/app/exit_codes.go
package app

type exitCode int

const (
    exitOK      exitCode = 0
    exitError   exitCode = 1   // General error
    exitCancel  exitCode = 2   // User cancelled (Ctrl+C, prompt cancellation)
    exitPending exitCode = 8   // Operation pending (async workflows)
)
```

### Typed errors

Commands don't decide their own exit codes. They return typed errors, and a single top-level classifier maps errors to exit codes:

```go
// internal/cmdutil/errors.go
package cmdutil

import "errors"

// ErrSilent indicates the error message has already been printed;
// the top-level handler should exit with a non-zero code but not
// print the error again.
var ErrSilent = errors.New("silent error")

// FlagError indicates invalid flag usage. The top-level handler
// should print the error AND the command's usage text.
type FlagError struct {
    Err error
}

func (e *FlagError) Error() string { return e.Err.Error() }
func (e *FlagError) Unwrap() error { return e.Err }
```

### Centralized error classification

In `app.Main()`, after command execution:

```go
if err != nil {
    switch {
    case errors.Is(err, cmdutil.ErrSilent):
        return exitError

    case errors.Is(err, context.Canceled):
        // User pressed Ctrl+C or context was cancelled
        return exitCancel

    case errors.As(err, &cmdutil.FlagError{}):
        // Print error + usage text
        fmt.Fprintln(stderr, err)
        // ... print usage ...
        return exitError

    default:
        fmt.Fprintln(stderr, err)
        return exitError
    }
}
```

### Why centralized error classification matters

If each command chose its own exit code, you'd have inconsistency, duplication (every command implements the same error-to-message logic), and untestable exit paths (commands calling `os.Exit` directly).

With centralized classification:
- Commands focus on *what went wrong* (return a typed error)
- The classifier focuses on *what to do about it* (map to exit code, print helpful messages)
- Tests verify error types, not exit codes — the classifier is tested once

## 13. Diagnostic Servers (pprof)

Go's `net/http/pprof` package provides runtime profiling data (CPU, memory, goroutine dumps, block profiles) via an HTTP endpoint. For long-running commands — batch jobs, HTTP servers, queue workers — pprof is invaluable for diagnosing production performance issues.

### Pprof is a per-command opt-in

Pprof is **not** a global default and does **not** belong in the root Before hook or the Factory. Not all commands benefit from a diagnostics server, and unconditionally starting one on a well-known port creates unnecessary attack surface and port conflicts.

Instead, commands that benefit from pprof offer a `--diagnostics-port` flag (or `DIAGNOSTICS_PORT` env var):

```go
&cli.IntFlag{
    Name:        "diagnostics-port",
    Sources:     cli.EnvVars("DIAGNOSTICS_PORT"),
    Usage:       "Port for pprof diagnostics server (0 = disabled)",
    Value:       0,
    Destination: &opts.DiagnosticsPort,
}
```

When the flag is unset or zero, no pprof server starts. When set, pprof listens on the specified port. This follows the same pattern as any other command-level resource — started in `configure` or `run`, cleaned up via `defer` or shutdown logic.

### Startup and shutdown

The pprof server follows the same deferred-cleanup pattern as database connections or other command-owned resources. The command's package must blank-import `net/http/pprof` to trigger pprof's `init()` registration on `http.DefaultServeMux`:

```go
import _ "net/http/pprof"

// In run() or configure():
if opts.DiagnosticsPort > 0 {
    pprofSrv := &http.Server{
        Addr:    fmt.Sprintf(":%d", opts.DiagnosticsPort),
        Handler: http.DefaultServeMux,  // pprof registered via init()
    }
    go pprofSrv.ListenAndServe()
    defer pprofSrv.Close()
}
```

### When to offer `--diagnostics-port`

- Long-running batch jobs (data migrations, ETL pipelines) — yes
- HTTP servers — yes (pprof runs on a separate port from the application server)
- Queue workers — yes
- Short-lived CLI commands (`version`, one-shot queries) — no

---

## 14. Testing Patterns

Testing is the primary reason for every abstraction in this template. Each pattern exists because it makes a specific kind of test possible.

### Testing flag parsing independently from business logic

The `runFn` parameter on `NewCmd()` lets tests verify that flags are parsed correctly without executing the command's logic:

```go
func TestListCmd_FlagParsing(t *testing.T) {
    ios, _, _, _ := iostreams.Test()
    f := &cmdutil.Factory{
        IOStreams:   ios,
        HttpClient: func() (*http.Client, error) { panic("should not be called") },
    }

    var captured *ListOptions
    cmd := NewCmd(f, func(ctx context.Context, opts *ListOptions) error {
        captured = opts
        return nil
    })

    // Simulate: np widget list --limit 10 --format json
    err := cmd.Run(context.Background(), []string{"list", "--limit", "10", "--format", "json"})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if captured.Limit != 10 {
        t.Errorf("Limit = %d, want 10", captured.Limit)
    }
    if captured.Format != "json" {
        t.Errorf("Format = %q, want %q", captured.Format, "json")
    }
}
```

Notice: `HttpClient` panics if called. This test is verifying flag parsing — if the command accidentally calls `HttpClient`, the test fails with a clear message rather than making a real HTTP request or silently succeeding.

### Testing business logic with HTTP stubs

For testing what the command actually does, stub the HTTP transport layer:

```go
func TestListCmd_Success(t *testing.T) {
    // Given: an API that returns two widgets
    reg := &httpstub.Registry{}
    defer reg.Verify(t)  // Fails if registered stubs weren't hit
    reg.Register(
        httpstub.REST("GET", "api/v1/widgets"),
        httpstub.JSONResponse(map[string]any{
            "widgets": []map[string]any{
                {"name": "sprocket", "status": "active"},
                {"name": "cog", "status": "inactive"},
            },
        }),
    )

    ios, _, stdout, _ := iostreams.Test()
    ios.SetStdoutTTY(true)

    // When: the list command runs
    opts := &ListOptions{
        IO:    ios,
        Limit: 30,
        HttpClient: func() (*http.Client, error) {
            return &http.Client{Transport: reg}, nil
        },
    }
    err := run(context.Background(), opts)

    // Then: it produces the expected output
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !strings.Contains(stdout.String(), "sprocket") {
        t.Errorf("stdout missing %q: %s", "sprocket", stdout.String())
    }
}
```

Key points:
- **`reg.Verify(t)`** ensures the test's registered stubs were actually called. If you register a stub that the code never hits, the test fails — catching dead test setup.
- **`ios.SetStdoutTTY(true)`** simulates an interactive terminal. You should also test with `SetStdoutTTY(false)` to verify the non-interactive output path.
- The test constructs an `Options` struct directly, bypassing `NewCmd()` and flag parsing entirely. This means flag parsing bugs and business logic bugs produce failures in different tests.

### HTTP stub infrastructure

The recommended approach stubs at the `http.RoundTripper` level — below the HTTP client, above the network:

```go
// Matcher determines whether a stub applies to a given request.
type Matcher func(*http.Request) bool

// Responder produces a response for a matched request.
type Responder func(*http.Request) (*http.Response, error)

// Registry implements http.RoundTripper by matching requests against
// registered stubs. Unmatched requests cause test failures.
type Registry struct {
    stubs    []stub
    matched  []bool
}

// Register adds a stub: requests matching the Matcher receive the Responder's response.
func (r *Registry) Register(m Matcher, resp Responder) { /* ... */ }

// RoundTrip implements http.RoundTripper. It finds the first matching stub
// and returns its response. If no stub matches, it returns an error
// describing the unmatched request.
func (r *Registry) RoundTrip(req *http.Request) (*http.Response, error) { /* ... */ }

// Verify asserts that every registered stub was called at least once.
func (r *Registry) Verify(t testing.TB) { /* ... */ }
```

### Why stub at the transport layer instead of mocking an API client interface

There are two common approaches to testing HTTP-calling code:

**Approach A: Mock the API client interface**
```go
type WidgetAPI interface {
    ListWidgets(ctx context.Context, limit int) ([]Widget, error)
}
// Test provides a mock that returns canned data
```

**Approach B: Stub the HTTP transport**
```go
// Test provides an http.RoundTripper that matches requests and returns canned responses
```

Approach B is superior for CLI applications because:

1. **It tests the real HTTP code.** The code under test constructs real HTTP requests — real URLs, real headers, real query parameters, real JSON serialization. A bug in request construction (wrong path, missing header, malformed body) is caught.

2. **No API client interface to maintain.** Approach A requires defining an interface that mirrors every API method, implementing a production struct that makes real HTTP calls, and implementing a mock struct for tests. Approach B requires no interface — the `*http.Client` is the dependency, and you swap its transport.

3. **Stubs are declarative and inspectable.** `httpstub.REST("GET", "api/v1/widgets")` clearly states what request is expected. The `Verify` method catches stubs that were never hit. Mock frameworks often bury this in assertion syntax.

The tradeoff: transport-layer stubs require you to produce realistic HTTP responses (status codes, JSON bodies, headers). For REST APIs this is straightforward. For complex protocols (WebSocket, gRPC) you may need a different approach.

### Testing TTY vs. non-TTY output

Commands that format output differently for terminals vs. pipes need tests for both modes:

```go
func TestListCmd_TTY(t *testing.T) {
    ios, _, stdout, _ := iostreams.Test()
    ios.SetStdoutTTY(true)
    // ... run command ...
    // Expect human-friendly output: colors, relative timestamps, table formatting
    if !strings.Contains(stdout.String(), "3 hours ago") {
        t.Errorf("expected relative timestamp in TTY output")
    }
}

func TestListCmd_NonTTY(t *testing.T) {
    ios, _, stdout, _ := iostreams.Test()
    ios.SetStdoutTTY(false)
    // ... run command ...
    // Expect machine-parseable output: no colors, absolute timestamps, tab-separated
    if !strings.Contains(stdout.String(), "2024-01-15T10:30:00Z") {
        t.Errorf("expected absolute timestamp in non-TTY output")
    }
}
```

### Testing long-running services

Service commands need tests that verify startup and graceful shutdown:

```go
func TestServeCmd_GracefulShutdown(t *testing.T) {
    // Given: a cancellable context (simulating SIGTERM)
    ctx, cancel := context.WithCancel(context.Background())

    ios, _, _, stderr := iostreams.Test()
    opts := &ServeOptions{
        IO:     ios,
        Logger: func() *slog.Logger { return slog.New(slog.NewTextHandler(stderr, nil)) },
        Port:   0, // let the OS assign a port
    }

    // When: the server starts and we cancel the context
    errCh := make(chan error, 1)
    go func() {
        errCh <- run(ctx, opts)
    }()

    // Give the server a moment to start
    time.Sleep(100 * time.Millisecond)
    cancel() // simulate SIGTERM

    // Then: the server shuts down cleanly
    err := <-errCh
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !strings.Contains(stderr.String(), "shutdown signal received") {
        t.Errorf("expected shutdown message in stderr")
    }
}
```

### Table-driven tests

For commands with many input variations, use table-driven tests:

```go
func TestListCmd_Flags(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantLimit  int
        wantFormat string
        wantErr    string
    }{
        {
            name:       "defaults",
            args:       []string{"list"},
            wantLimit:  30,
            wantFormat: "table",
        },
        {
            name:       "custom limit",
            args:       []string{"list", "--limit", "10"},
            wantLimit:  10,
            wantFormat: "table",
        },
        {
            name:    "invalid limit",
            args:    []string{"list", "--limit", "0"},
            wantErr: "invalid value for --limit",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ios, _, _, _ := iostreams.Test()
            f := &cmdutil.Factory{IOStreams: ios}

            var captured *ListOptions
            cmd := NewCmd(f, func(ctx context.Context, opts *ListOptions) error {
                captured = opts
                return nil
            })

            err := cmd.Run(context.Background(), tt.args)

            if tt.wantErr != "" {
                if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
                    t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
                }
                return
            }

            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if captured.Limit != tt.wantLimit {
                t.Errorf("Limit = %d, want %d", captured.Limit, tt.wantLimit)
            }
            if captured.Format != tt.wantFormat {
                t.Errorf("Format = %q, want %q", captured.Format, tt.wantFormat)
            }
        })
    }
}
```

## 15. Application Launch Process

The full step-by-step trace of the application launch process — from `os.Exit` down to the moment a command's `run()` function begins executing — is documented in [launch-process.md](launch-process.md).

In brief, the launch follows this sequence:

```
os.Exit ← app.Main()
              │
              ├─ newFactory(appName, version)
              │     ├─ Phase 1: IOStreams (eager — System())
              │     └─ Phase 2: Logger (eager — mutable LevelVar)
              │
              ├─ root.NewRootCmd(f)
              │     ├─ Before hook: setLogLevel → Logger → context → signal handling → gops
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

See [launch-process.md](launch-process.md) for the complete trace with code snippets, context key patterns, and detailed explanations of each phase.
