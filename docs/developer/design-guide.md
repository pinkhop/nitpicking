# Design Guide

This guide documents the main implementation patterns used in this repository today. It is intentionally narrower than the architecture document: it focuses on the code-level conventions contributors will touch most often.

For authoritative layering and dependency rules, see [Architecture](architecture.md).

---

## 1. Scope

Nitpicking is a local-only CLI application. The design guidance in this repo should optimize for:

- Clear command behavior
- Strong separation between CLI wiring and business logic
- Fast unit tests using in-memory adapters
- Explicit workspace and database behavior

This repo does not currently ship an HTTP server, daemon mode, structured logging stack, or generic service framework. The guidance below reflects that reality.

---

## 2. Thin `main`

`cmd/np/main.go` is intentionally small:

- Build the `Factory` with `wiring.NewCore`
- Build the root command with `root.NewRootCmd`
- Execute the CLI
- Classify the returned error into an exit code
- Call `os.Exit` exactly once

That keeps the launch path testable and makes error-to-exit-code policy a single, centralized concern.

---

## 3. Factory Pattern

`internal/cmdutil/factory.go` holds shared dependencies for commands.

Current fields include:

- App metadata: `AppName`, `AppVersion`, `BuildInfo`
- Terminal I/O: `IOStreams`
- Global workspace override: `Workspace`
- Lazy SQLite access: `Store`
- Exit policy hint: `SignalCancelIsError`

### Why `Store` is a function

The store is exposed as:

```go
Store func() (*sqlite.Store, error)
```

That means:

- Commands that do not need the database do not pay to discover or open it.
- Tests can substitute store construction.
- Wiring owns the details of database discovery and init-time creation.

This is the main dependency-injection seam in the current codebase.

---

## 4. IOStreams and TTY-Aware Commands

`internal/iostreams` abstracts stdin, stdout, stderr, and terminal capabilities.

Use it when command behavior depends on whether the process is interactive.

The clearest example is `np create`:

- If stdin is a TTY, it delegates to the interactive form flow.
- If stdin is not a TTY, it reads JSON from stdin and behaves like the machine-oriented create path.

The repo uses this same split elsewhere:

- `form` commands for humans
- `json` commands for agents and scripts

When adding commands that behave differently in TTY and non-TTY contexts, prefer explicit branching on `IOStreams` rather than ad hoc terminal probing.

---

## 5. Command Package Shape

Each command or command group lives under `internal/cmd/`.

Typical structure:

1. `NewCmd(f *cmdutil.Factory)` defines flags and help text.
2. The `Action` closure validates CLI input.
3. The command obtains a tracker or other dependency from the `Factory`.
4. The command delegates real behavior to a package-local helper.

Common helper names in this repo:

- `Run`
- `JSONLRun`
- `Reopen`
- `Defer`
- `RunCreate`
- `RunUpdate`

This split keeps flag parsing and framework code separate from business behavior, which makes tests simpler.

### What not to do

- Do not bury business rules inside `Action` closures.
- Do not let commands reach into driven adapters directly.
- Do not move shared business logic into `cmdutil`; `cmdutil` is for CLI infrastructure, not domain rules.

---

## 6. Service Access from Commands

Most commands build the application service through:

```go
svc, err := cmdutil.NewTracker(f)
```

That keeps the command talking to the driving port rather than directly to storage adapters.

If a command needs lower-level access for a true adapter concern, keep that scoped and explicit. The normal path should still be:

- CLI command
- driving service
- core
- driven ports/adapters

---

## 7. Form vs JSON Workflows

This repo deliberately supports two user-facing mutation styles:

- `form` commands for interactive terminal use
- `json` commands for agents and scripts

The root `create` command adds a third layer: auto-detection based on stdin.

When adding or changing mutation flows:

- Keep interactive concerns under `internal/cmd/formcmd`
- Keep structured stdin/stdout flows under `internal/cmd/jsoncmd`
- Preserve the semantic distinction between human-oriented and agent-oriented paths

That separation is one of the main usability properties of the CLI.

---

## 8. Tests

The preferred testing model is behavior-first, not framework-first.

### Use the right test tier

- Unit tests for command helpers, domain logic, and core behavior
- Boundary tests for SQLite adapter behavior
- Blackbox tests for end-to-end CLI behavior

### Keep tests close to the code

Command packages already model this well:

- Flag parsing and command behavior tests in the command package
- Domain validation tests in `internal/domain`
- Core use-case tests in `internal/core`
- Adapter tests next to the adapter implementation

### Prefer fakes and real in-memory adapters

The in-memory storage adapter is a real implementation, not a mock. Use it to keep core tests fast and honest.

---

## 9. Documentation-Sensitive Areas

Some parts of the CLI are especially sensitive to drift between code and docs:

- The top-level command tree in `internal/cmd/root`
- Agent bootstrap text in `internal/cmd/agent/instructions.go`
- The dual `form` / `json` mutation model
- Workspace discovery behavior and `--workspace`

If you change any of those, update the corresponding user and developer docs in the same change.

The agent-prime output is especially important. When command names or flags change, verify that:

- `np agent prime` still emits valid commands
- `.claude/rules/issue-tracking.md` can be regenerated from it without manual fixes

---

## 10. When in Doubt

Use this order of preference when deciding where new code belongs:

1. Follow the current command package pattern.
2. Keep CLI concerns in driving adapters.
3. Keep business rules in the core or domain.
4. Keep storage-specific behavior in adapters.
5. Re-check the dependency rules in [Architecture](architecture.md).

If a new abstraction only makes sense in a hypothetical future server or daemon, it probably does not belong in this repo yet.
