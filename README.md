# Nitpicking

Nitpicking (`np`) is a local-only, single-machine CLI issue tracker for AI-agent and solo-developer workflows. It keeps issue tracking inside your workspace, stores data in a local SQLite database under `.np/`, gates mutations behind explicit claims, and avoids hosted services, background daemons, global git hooks, or repo-coupled automation.

Inspired by Steve Yegge's [beads](https://github.com/steveyegge/beads) project, Nitpicking keeps the useful local workflow ideas while deliberately staying narrower and less invasive.

---

## What It Is

- A workspace-local issue tracker rooted at `.np/`
- A CLI designed for both humans and agents using the same command surface
- A claim-based coordination model instead of hidden locks or background processes
- A lightweight planning and execution tool for local work

## What It Is Not

- Not a hosted issue tracker
- Not a multi-machine sync system
- Not a background service or daemon
- Not a git hook manager
- Not an agent orchestrator

---

## Why It Is Different

- Local-only by design. Issues live in a SQLite database inside `.np/`; there is no remote service, account, or network dependency.
- Claims gate mutations. Updating, deferring, deleting, or closing an issue requires its claim ID. Claims primarily exist for multi-agent coordination, but they also give solo humans a visible "work is in progress" marker.
- Readiness is built in. `np claim ready`, `np ready`, and `np blocked` help the tool choose work based on blockers and state instead of forcing everything through manual assignment.
- Humans and agents share the same CLI. Interactive forms exist for people, while JSON commands and `agent` utilities support non-interactive workflows without a separate API.
- It stays out of the way. No git hooks, no daemons, no server process, and no hidden state outside your workspace.

---

## The Basic Loop

For day-to-day use, start with one habit:

```bash
np ready
np claim ready --author "$USER"
# do the work
np close --claim <CLAIM-ID> --reason "Done."
```

That is enough to use `np` without adopting labels, epics, or agent setup on day one. When the backlog needs light structure, keep the workflow flat and add labels such as `kind:bug`, `area:auth`, or `task-group:auth-overhaul`.

---

## Installation

### Download a release binary

Prebuilt static binaries are available on the [Releases page](https://github.com/pinkhop/nitpicking/releases). Set `VERSION` to the release version (without the leading `v`) you want to install; for example:

```bash
VERSION=0.2.0
```

**macOS (Apple Silicon):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_darwin_arm64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
xattr -d com.apple.quarantine np   # IMPORTANT: remove quarantine attribute
mv np ~/.local/bin/                # or:  sudo mv np /usr/local/bin/
rm np.tar.gz
```

**macOS (Intel):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_darwin_amd64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
xattr -d com.apple.quarantine np   # IMPORTANT: remove quarantine attribute
mv np ~/.local/bin/                # or:  sudo mv np /usr/local/bin/
rm np.tar.gz
```

**Linux (x86_64):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_linux_amd64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
sudo mv np /usr/local/bin/
rm np.tar.gz
```

**Linux (ARM64):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_linux_arm64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
sudo mv np /usr/local/bin/
rm np.tar.gz
```

### Build from source

Prerequisites:

- Go 1.26+
- `make`

```bash
git clone https://github.com/pinkhop/nitpicking.git
cd nitpicking
make build
```

This produces a static binary at `dist/np`. You can invoke it directly with `./dist/np` or copy/symlink it somewhere on your `PATH`.

- Users: start with the [Quickstart guide](./docs/user/quickstart.md).
- Contributors: start with [Developer Onboarding](./docs/developer/getting-started/onboarding.md) for the shortest path from clone to first safe change.

---

## Quickstart

This is the shortest useful end-to-end flow:

1. Initialize a workspace-local issue database.
2. Create an issue.
3. Inspect the backlog.
4. Claim work before mutating it.
5. Close the claimed issue with a reason.

```bash
# In your workspace directory
np init FOO

# Create an issue in the interactive form
np create

# See what is available
np ready

# Claim the next ready issue and note the returned claim ID
np claim ready --author "$USER"

# Close the claimed issue
np close \
  --claim <CLAIM-ID> \
  --reason "Done."
```

`np close --reason` records the reason as a comment and closes the issue in one atomic step.

`np` discovers the workspace by walking up from the current directory, so after `np init` you can run commands from the repo root or any subdirectory below it. If you need to bypass parent traversal, use the global `--workspace` flag or `NP_WORKSPACE`.

### Human And Agent Paths

Humans and agents use the same CLI, but they typically use it differently:

| Action | Human | Agent |
|---|---|---|
| Create an issue | `np create` in a terminal launches the interactive form | `np json create --author "$AUTHOR"` reads JSON from stdin |
| Inspect work | `np list`, `np show <ISSUE-ID>`, `np ready` | Add `--json` for machine-readable output |
| Claim work | `np claim ready --author "$USER"` | `np claim ready --author "$AUTHOR" --json` |
| Update a claimed issue | `np form update --claim <CLAIM-ID>` | `np json update --claim <CLAIM-ID>` |
| Comment on an issue | `np form comment --author "$USER" <ISSUE-ID>` | `np json comment --author "$AUTHOR" <ISSUE-ID>` |

For humans, the TTY-driven commands are the default path:

```bash
np create
np list
np claim ready --author "$USER"
np show <ISSUE-ID>
```

For agents and scripts, prefer machine-readable output and the stdin-driven JSON commands:

```bash
np claim ready --author "$AUTHOR" --json
np show <ISSUE-ID> --json
np list --ready --json
np json create --author "$AUTHOR"
np json update --claim <CLAIM-ID>
np json comment --author "$AUTHOR" <ISSUE-ID>
```

To bootstrap an agent session, use:

```bash
np agent name --seed=$PPID
np agent prime
```

The `--seed=$PPID` flag ties the generated name to the agent's process ID, so the same process always produces the same author identity. Omit `--seed` only when you want a fresh random name each time.

---

## How It Works

- Each workspace gets its own `.np/` directory with the embedded SQLite database and related metadata.
- Start with flat `task` issues. Use labels for grouping, routing, and metadata such as `kind:bug`, `area:auth`, or `task-group:auth-overhaul`.
- Epics are optional. Add them only when flat tasks plus labels are not enough and you need explicit hierarchy, progress tracking, or grouped closure.
- Claiming is the concurrency gate. Comments and most non-hierarchical relationships can be added without a claim, but field updates and state transitions require one.
- Readiness is computed. In the default flat workflow, a task is ready when it is open and unblocked.
- `np create` auto-detects its mode: piped stdin reads JSON, while a TTY launches the interactive form UI.
- Backup and restore are built in for moving or recovering a local workspace.

---

## Main Command Groups

The root CLI is organized into these workflow-first categories:

- `Setup`: `init`
- `Core Workflow`: `create`, `claim`, `close`, `show`, `list`, `ready`, `blocked`
- `Issues`: `issue`, `epic`, `rel`, `label`, `comment`, `form`
- `Agent Toolkit`: `json`, `agent`
- `Admin`: `admin`
- `Info`: `version`

Run `np --help` for the current command tree.

Within `admin`, the most important maintenance commands are `admin doctor`, `admin backup`, and `admin restore`.

---

## Documentation Map

Start with the [Quickstart Guide](./docs/user/quickstart.md).

From there, the main user entry points are:

- **[Quickstart](./docs/user/quickstart.md)** — The smallest useful task-first workflow
- **[Agent Setup](./docs/user/agents/setup.md)** — How to configure coding agents to use `np`
- **[Daily Work](./docs/user/daily-work.md)** — The flat task workflow humans and agents repeat once the tracker becomes part of daily work
- **[Core Concepts](./docs/user/reference/core-concepts.md)** — Claims, readiness, issue roles, relationships, and priorities
- **[Labels Guide](./docs/user/labels.md)** — Label naming, flat grouping, filtering, routing, and day-to-day usage
- **[Epics](./docs/user/epics.md)** — Optional hierarchy when flat tasks and labels are not enough
- **[Multi-Agent Operations](./docs/user/agents/multi-agent.md)** — Coordinating multiple agents in one workspace
- **[Command Reference](./docs/user/reference/command-reference.md)** — Full CLI reference
- **[Troubleshooting](./docs/user/reference/troubleshooting.md)** — Diagnostics and recovery steps
- **[FAQ](./docs/user/reference/faq.md)** — Common questions and operational tradeoffs

See the broader [docs index](./docs/README.md) for the full documentation map.

---

## Contributor Links

Contributor and implementation material is available, but it is secondary to the user onboarding path above:

- **[Developer Onboarding](./docs/developer/getting-started/onboarding.md)** — First-day reading order and the shortest contribution path
- **[Developer Setup](./docs/developer/getting-started/developer-setup.md)** — Build tooling, Make targets, and local setup
- **[Architecture](./docs/developer/architecture/architecture.md)** — The authoritative layering and dependency rules
- **[CLI Implementation Guide](./docs/developer/architecture/cli-implementation.md)** — Command, wiring, and testing conventions

---

## License

Nitpicking is licensed under the terms of the MIT license. See [LICENSE](./LICENSE) for the full text.

---

## AI Disclosure

This project's source code and documentation were created in part or in whole using generative AI under the direction of the author.
