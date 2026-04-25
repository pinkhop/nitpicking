# Developer Setup

Build tooling and local development commands for the Nitpicking codebase.

If you are new to the repo, read [Onboarding](onboarding.md) first.

## Prerequisites

- Go 1.26+
- `make`

## Fast Start

```bash
make build
make test
make lint
```

Use that as the default local loop unless your change needs a broader test
tier.

## Build And Development Commands

The `Makefile` is the canonical entry point for local development tasks.

| Command | Description |
|---------|-------------|
| `make build` | Build the binary into `dist/np` |
| `make build VERSION=1.2.3` | Build with a baked-in version string |
| `make test` | Run unit tests (`test-units`) |
| `make test-boundary` | Run SQLite boundary tests (`-tags=boundary`) |
| `make test-blackbox` | Run blackbox tests (`-tags=blackbox`) |
| `make lint` | Run `go vet`, `gofumpt`, `goimports`, `ineffassign`, `errcheck`, and `staticcheck` |
| `make sec` | Run `gosec` and `govulncheck` |
| `make fmt` | Run `gofumpt` and `goimports` in write mode |
| `make coverage` | Generate unit-test coverage output under `coverage/` |
| `make ci` | Run the full local CI sequence: build, lint, sec, test |
| `make clean` | Remove build output, coverage output, and Go test cache |

## Build Notes

- `make build` writes the binary to `dist/np`.
- The build uses `CGO_ENABLED=0`.
- The Makefile enables the `netgo` and `osusergo` build tags and uses
  `-trimpath`.
- The build version is injected into `internal/wiring.version` via `-ldflags`
  when `VERSION` is set.

Examples:

```bash
make build
make build VERSION=1.2.3
make build VERSION=$(git describe --tags --always --dirty)
```

## Test Tiers

This repo uses three distinct test tiers:

| Tier | Command | Scope |
|------|---------|-------|
| Unit | `make test` | Fast tests with in-memory fakes and no external systems |
| Boundary | `make test-boundary` | Adapter tests against real SQLite behavior |
| Blackbox | `make test-blackbox` | End-to-end CLI behavior using compiled-command flows |

Use the unit suite by default. Boundary and blackbox tests are intentionally
separate because they are slower and exercise more of the real stack.

## Go Tools

Linters and security scanners are managed as tool dependencies in `go.mod` and
invoked through `go tool`. No separate global installation is required.

| Tool | Make target | Purpose |
|------|-------------|---------|
| `gofumpt` | `make gofumpt` | Strict Go formatting |
| `goimports` | `make goimports` | Import organization and cleanup |
| `ineffassign` | `make lint-ineffassign` | Detect never-read assignments |
| `errcheck` | `make lint-errcheck` | Detect unchecked errors |
| `staticcheck` | `make lint-staticcheck` | Advanced static analysis |
| `gosec` | `make sec-gosec` | Static security scanning |
| `govulncheck` | `make sec-govulncheck` | Dependency vulnerability scanning |

There is no `golangci-lint` wrapper in this project. The individual tools and
targets are the source of truth.

## Container Image

The repo includes a multi-stage `Dockerfile` that builds `np` and copies the
binary into a minimal runtime image.

```bash
docker build -t np .
docker build -t np --build-arg VERSION=1.2.3 .
docker run --rm np version
```

The image is useful for smoke-testing packaging or running the binary in a
minimal environment, but the normal development loop is still `make build`
plus the local test and lint targets.

## Where To Read Next

- For the fastest path to a safe change, read [First Change](first-change.md).
- For package placement rules, read
  [Architecture](../architecture/architecture.md) and
  [Package Layout](../architecture/package-layout.md).
- For command and wiring behavior, read
  [CLI Implementation Guide](../architecture/cli-implementation.md).
