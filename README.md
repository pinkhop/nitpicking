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
- Claims gate mutations. Updating, deferring, deleting, or closing an issue requires its claim ID, which gives humans and agents an explicit coordination primitive.
- Readiness is built in. `np claim ready`, `np ready`, and `np blocked` help the tool choose work based on blockers and state instead of forcing everything through manual assignment.
- Humans and agents share the same CLI. Interactive forms exist for people, while JSON commands and `agent` utilities support non-interactive workflows without a separate API.
- It stays out of the way. No git hooks, no daemons, no server process, and no hidden state outside your workspace.

---

## Installation

### Download a release binary

Prebuilt static binaries are available on the [Releases page](https://github.com/pinkhop/nitpicking/releases). Set `VERSION` to the release you want to install, then follow the instructions for your platform.

```bash
VERSION=0.1.1
```

**macOS (Apple Silicon):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_darwin_arm64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
xattr -d com.apple.quarantine np
sudo mv np /usr/local/bin/
rm np.tar.gz
```

**macOS (Intel):**

```bash
curl -fsSL "https://github.com/pinkhop/nitpicking/releases/download/v${VERSION}/nitpicking_${VERSION}_darwin_amd64.tar.gz" -o np.tar.gz
tar xzf np.tar.gz np
xattr -d com.apple.quarantine np
sudo mv np /usr/local/bin/
rm np.tar.gz
```

Remove the quarantine attribute that macOS GateKeeper applies to downloaded files by running `xattr -d com.apple.quarantine /usr/local/bin/np` after moving the binary into place. Without this step, macOS will block `np` with an "unidentified developer" warning. The commands above already include this step before the `mv`; if you move the binary to a different location, run `xattr -d com.apple.quarantine /path/to/np` against the final path.

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

- Users: start with the [Getting Started guide](./docs/user/getting-started.md).
- Contributors: see [Developer Setup](./docs/developer/developer-setup.md) for build, lint, test, and release-related commands.

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
np init PKHP

# Pipe JSON into create, or run `np create` with a TTY for the interactive form
np create --author "$USER" <<'JSONEND'
{
  "role": "task",
  "title": "My first task"
}
JSONEND

# See what is available
np list

# Claim the next ready issue and note the returned claim ID
np claim ready --author "$USER"

# Close the claimed issue
np close \
  --claim <CLAIM-ID> \
  --reason "Done."
```

`np` discovers the workspace by walking up from the current directory, so after `np init` you can run commands from the repo root or any subdirectory below it. If you need to bypass parent traversal, use the global `--workspace` flag or `NP_WORKSPACE`.

### For Agents

Agents should prefer machine-readable output:

```bash
np claim ready --author "$AUTHOR" --json
np show <ISSUE-ID> --json
np list --ready --json
```

For stdin-driven workflows, `np` also ships explicit JSON subcommands:

```bash
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

Claude Code example:

```bash
mkdir -p .claude/rules/
np agent prime > .claude/rules/issue-tracking.md
```

---

## How It Works

- Each workspace gets its own `.np/` directory with the embedded SQLite database and related metadata.
- Issues are either `task` or `epic`. Use labels for further categorization such as bug, feature, or area.
- Claiming is the concurrency gate. Comments and most non-hierarchical relationships can be added without a claim, but field updates and state transitions require one.
- Readiness is computed. A task is ready only when it is open and unblocked; an empty epic can be ready when it needs decomposition.
- `np create` auto-detects its mode: piped stdin reads JSON, while a TTY launches the interactive form UI.
- Backup and restore are built in, and `np import jsonl` supports bulk creation from structured JSONL input.

---

## Main Command Groups

The root CLI is organized into these workflow-first categories:

- `Setup`: `init`
- `Core Workflow`: `create`, `claim`, `close`, `show`, `list`, `ready`, `blocked`
- `Issues`: `issue`, `epic`, `rel`, `label`, `comment`, `form`
- `Agent Toolkit`: `json`, `agent`
- `Admin`: `admin`, `import`
- `Info`: `version`

Within `admin`, the most important maintenance commands are `admin where`, `admin doctor`, `admin backup`, `admin restore`, `admin gc`, `admin completion`, `admin tally`, and `admin reset`.

Run `np --help` for the current command tree.

---

## Documentation Map

Start with the [Getting Started Guide](./docs/user/getting-started.md).

From there, the main user entry points are:

- **[User Documentation Index](./docs/user/README.md)** — Overview of all user-focused guides
- **[Key Concepts](./docs/user/key-concepts.md)** — Claims, readiness, issue roles, relationships, and priorities
- **[Daily Use Guide](./docs/user/daily-use.md)** — The commands you will repeat most often once the tracker becomes part of your workflow
- **[Workflow Guides](./docs/user/workflow-simple.md)** — Task-only, epic-driven, label-driven, and multi-agent workflows
- **[Agent Integration Guide](./docs/user/agent-integration.md)** — How to configure coding agents to use `np`
- **[Command Reference](./docs/user/command-reference.md)** — Full CLI reference
- **[Troubleshooting](./docs/user/troubleshooting.md)** — Diagnostics and recovery steps
- **[FAQ](./docs/user/faq.md)** — Common questions and operational tradeoffs

See the broader [docs index](./docs/README.md) for the full documentation map.

---

## Contributor Links

Contributor and implementation material is available, but it is secondary to the user onboarding path above:

- **[Developer Setup](./docs/developer/developer-setup.md)** — Build tooling, Make targets, and local setup
- **[Architecture](./docs/developer/architecture.md)** — The authoritative layering and dependency rules
- **[Design Guide](./docs/developer/design-guide.md)** — Code patterns and implementation conventions
- **[Launch Process](./docs/developer/launch-process.md)** — How `main()` dispatches to commands

---

## License

Nitpicking is licensed under the terms of the MIT license. See [LICENSE](./LICENSE) for the full text.

---

## AI Disclosure

This project's source code and documentation were created in part or in whole using generative AI under the direction of the author.
