# Developer Documentation

Architecture, build tooling, and internals for contributors to `np`.

---

## Setup

- **[Developer Setup](developer-setup.md)** — Build tooling, Make targets, Go tools, container image, and adding commands.

## Architecture

- **[Architecture](architecture.md)** — Authoritative layering, dependency rules, package map, and anti-patterns.
- **[Design Guide](design-guide.md)** — Repo-specific code conventions, command structure, and testing guidance.
- **[Launch Process](launch-process.md)** — How the `np` binary starts up, from `main()` through command dispatch.

## Formats

- **[JSONL Import Format](jsonl-import-format.md)** — Line schema, field details, reference resolution rules, and worked examples for `np import jsonl`.

## Security

- **[Claim ID Security Model](claim-security.md)** — Threat model for claim IDs, randomness and hashing rationale, and output redaction policy.

## Investigations

The `investigations/` directory contains research notes on specific technical topics explored during development.
