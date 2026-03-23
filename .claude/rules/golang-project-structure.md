---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Project Structure

## Directory Layout

All application packages live under `internal/`. The compiled binary is the product; no external Go module imports the packages, so `internal/` enforces this at the compiler level. Do not use a `pkg/` directory.

```
cmd/<appname>/main.go              # Thin main: calls internal/app.Main(), os.Exit()
internal/app/                      # Entry point: factory construction, command dispatch, error classification
internal/cmd/root/                 # Root command: global flags, Before hook for cross-cutting concerns
internal/cmd/<command>/            # Leaf command package (one package per command)
internal/cmd/<parent>/<child>/     # Nested subcommand package
internal/cmd/<parent>/shared/      # Logic shared between sibling subcommands only
internal/cmdutil/                  # Factory struct, typed errors, shared flag helpers
internal/iostreams/                # Terminal I/O abstraction (TTY detection, color, pager)
```

## Package Boundaries

### One package per command

Each leaf command gets its own package under `internal/cmd/`. Go's package system enforces isolation: commands access collaborators only through exports, and a command's imports are a visible dependency manifest.

### Shared logic between sibling commands

When two subcommands under the same parent need common logic (display formatting, shared API calls), place it in a `shared/` sibling package — e.g., `internal/cmd/widget/shared/`. This keeps cross-command dependencies explicit. Do not put command-specific logic in `internal/cmdutil/` or other top-level packages.

### Package grouping for non-command code

Domain logic, adapters, and infrastructure that are not tied to a specific command live in their own packages under `internal/` — e.g., `internal/auth/`, `internal/api/`, `internal/storage/`. Organize by domain or responsibility, not by layer (no `internal/models/`, `internal/services/`, `internal/repositories/` groupings).

## The Thin Main

`cmd/<appname>/main.go` is the only file that calls `os.Exit`. It delegates immediately to `internal/app.Main()`, which returns a typed exit code. This keeps the entire launch path testable and ensures `defer` statements execute properly.

```go
package main

import (
    "os"
    "mymodule/internal/app"
)

func main() {
    os.Exit(int(app.Main()))
}
```

## Command File Structure

Every command package contains at minimum:

| File | Contents |
|------|----------|
| `<command>.go` | Options struct, `NewCmd()` constructor, `run()` function |
| `<command>_test.go` | Flag parsing tests and business logic tests |
| `testdata/` (optional) | Test fixture files (JSON responses, etc.); ignored by `go build` |

### Three-part command pattern

1. **Options struct** — declares exactly which Factory dependencies and user inputs the command needs
2. **`NewCmd()` constructor** — builds the `cli.Command`, wires flags to Options via `Destination`, accepts optional `runFn` for test injection
3. **`run()` function** — unexported; receives the fully-populated Options struct; contains all business logic

The `run()` function never reads flags from `*cli.Command` and never accesses the Factory directly. Everything it needs arrives through the Options struct.

## Adding a New Command

1. Create `internal/cmd/<parent>/<command>/` package
2. Define the Options struct with only the Factory fields this command uses
3. Write `NewCmd(f *cmdutil.Factory, runFn ...func(context.Context, *Options) error)`
4. Write the unexported `run(ctx context.Context, opts *Options) error`
5. Register the command in its parent's `Commands` slice
6. Write tests: flag parsing (via `runFn`) and business logic (via direct Options construction) as separate tests
