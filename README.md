# Nitpicking

A local-only, non-invasive, single-machine, CLI-driven issue tracker designed for AI agent workflows. Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project, Nitpicking deliberately avoids beads' invasiveness (global hooks, git lifecycle coupling, database servers) and complexity (multi-machine agent collaboration, scope creep).

----

## Getting Started

### Prerequisites

- Go 1.26+

### Build and Development Commands

#### Make

A `Makefile` is provided to implement the common build and development operations.

| Command | Description |
|---------|-------------|
| `make build` | Build the binary into `dist/` |
| `make build VERSION=1.2.3` | Build with a baked-in version string |
| `make test` | Run unit tests |
| `make lint` | Run all linters (vet, gofumpt, goimports, errcheck, staticcheck) |
| `make sec` | Run security scanners (gosec, govulncheck) |
| `make fmt` | Auto-fix formatting (gofumpt + goimports) |
| `make coverage` | Generate HTML coverage report |
| `make ci` | Run build + lint + sec + test (full local CI) |
| `make clean` | Remove build and coverage artifacts |

#### Go Tools

Linters and security scanners are managed as [tool dependencies](https://go.dev/doc/modules/managing-dependencies#tools) in `go.mod` and invoked via `go tool <name>`. No separate installation is required.

| Tool | Make target | Purpose |
|------|-------------|---------|
| `errcheck` | `make lint-errcheck` | Detect unchecked error returns |
| `gofumpt` | `make gofumpt` | Strict formatting superset of `gofmt` |
| `goimports` | `make goimports` | Import organization and cleanup |
| `gosec` | `make sec-gosec` | Security issue scanning |
| `govulncheck` | `make sec-govulncheck` | Known vulnerability detection in dependencies |
| `ineffassign` | `make lint-ineffassign` | Detect assignments to never-read variables |
| `staticcheck` | `make lint-staticcheck` | Advanced static analysis |

#### Container Image

The project includes a multi-stage `Dockerfile` that builds a statically-linked binary in a Chainguard Go image and copies it into a minimal distroless runtime image.

```bash
docker build -t np .                              # Build with default version (dev)
docker build -t np --build-arg VERSION=1.2.3 .    # Build with a specific version
docker run --rm np <cmd>                          # Run a command; e.g. `version`
```

The final image is based on `cgr.dev/chainguard/static` — nonroot, no shell, no package manager, minimal attack surface.

----

## Architecture Overview

For a comprehensive explanation of the architectural decisions, patterns, and conventions used in this codebase, see [docs/design-guide.md](./docs/design-guide.md).

### Project Layout

```
cmd/np/main.go                   # Thin shell: calls internal/app.Main(), os.Exit()
internal/app/                    # Main() → exitCode; factory construction, error classification
internal/cmd/root/               # Root command: global flags, Before hook
internal/cmd/<command>/          # One package per leaf command
internal/cmd/<parent>/shared/    # Logic shared between sibling subcommands
internal/cmdutil/                # Factory, typed errors, flag helpers
internal/iostreams/              # Terminal abstraction (TTY detection, color, pager)
```

Everything lives in `internal/` — the binary is the product; there are no external consumers.

### How a Command Runs

When the binary runs, execution follows a structured path: `main()` calls `app.Main()`, which constructs a Factory (lazy dependency injection), builds the command tree via `root.NewRootCmd()`, and dispatches to the matched command. The root command's `Before` hook handles process-wide concerns (log level, signal handling, gops) and enriches the context; each command's `Action` validates flags and calls `run()` with a fully-populated Options struct. If the command returns an error, centralized classification maps it to an exit code.

See [docs/launch-process.md](./docs/launch-process.md) for the full launch trace.

### Key Patterns

- **Factory (dependency injection)** — the `Factory` struct holds eager fields (`IOStreams`, `Logger`, `BuildInfo`) constructed once at startup, plus a `SignalCancelIsError` flag for lifecycle control. Function-typed fields (like `Logger`) double as testing seams — tests replace the closure without touching the production wiring.

- **Options struct** — every command defines an Options struct holding its Factory dependencies and user input from flags. `NewCmd()` wires flags via `Destination`; `run()` receives the fully-populated struct. This separates flag parsing from business logic, enabling independent testing of each.

- **Typed errors + centralized exit codes** — commands return typed errors (`ErrSilent`, `FlagError`) instead of calling `os.Exit`. A single `classifyError()` function maps error types to exit codes (0 OK, 1 error, 2 cancel, 8 pending) and prints appropriate messages.

### Configuration

`urfave/cli/v3` handles flag layering natively: **flags > environment variables > defaults**. Use the `Sources` field on flags for env var binding.

### Version Injection

The build version is injected at compile time via `-ldflags` into `internal/app.version`. When not set, it defaults to `"dev"`.

```bash
make build VERSION=1.2.3
make build VERSION=$(git describe --tags --always --dirty)
```
----

## Adding Commands

Create a new package under `internal/cmd/` following the Options struct pattern:

1. **Create the package:** `internal/cmd/<parent>/<command>/` (e.g., `internal/cmd/widget/delete/`)

2. **Define the Options struct.** Include only the Factory fields this command needs. Include user input fields for every flag. If the command displays timestamps, include `Now func() time.Time`.

3. **Write `NewCmd()`.** Accept the Factory and optional `runFn` for test injection. Wire flags to Options fields using `Destination`. Put flag validation in the `Action` function, before calling `run()`.

4. **Write `run()`.** This is where business logic lives. It receives a fully-populated Options struct. It should not read flags from `*cli.Command` — everything it needs is in `opts`.

5. **Register in the parent command.** Add to the `Commands` slice in the parent's `NewCmd()` or in `root.NewRootCmd()`.

6. **Write tests:**
   - Flag parsing test: use `runFn` to capture the Options struct, verify flag values
   - Business logic test: construct Options directly, provide HTTP stubs, assert on IOStreams output
   - Both TTY and non-TTY output tests if the command formats differently for terminals
   - Error path tests: verify the command returns the right error types

7. **Keep logic in the command package.** API calls, formatting, domain-specific logic — all within the command package. If two commands need the same logic, extract it to a `shared/` sibling package. Do not add command-specific logic to `cmdutil/` or top-level packages.

See [docs/design-guide.md](./docs/design-guide.md) for detailed explanations of the Factory, Options struct, and testing patterns referenced above.
