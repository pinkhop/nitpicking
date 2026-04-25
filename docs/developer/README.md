# Developer Documentation

Developer docs for contributors to `np`.

Start with [Onboarding](getting-started/onboarding.md) if you are new to the
repo. It gives you the shortest path from clone to first safe change.

The docs are organized by contributor job:

## Getting Started

- [Onboarding](getting-started/onboarding.md) - first-day reading order and the
  repo mental model
- [Developer Setup](getting-started/developer-setup.md) - build, test, lint,
  security, and container commands
- [First Change](getting-started/first-change.md) - the shortest path to a safe
  contribution

## Architecture

- [Architecture](architecture/architecture.md) - authoritative layering,
  dependency rules, and common mistakes to avoid
- [Package Layout](architecture/package-layout.md) - package map and "what goes
  where" decision guide
- [CLI Implementation Guide](architecture/cli-implementation.md) - practical
  command, wiring, help, and testing conventions

## Reference

- [JSONL Import Format](reference/jsonl-import-format.md) - file format and
  design rationale for `np import jsonl`
- [Claim ID Security Model](reference/claim-security.md) - threat model and
  rationale for claim token generation, storage, and output handling

## Historical Record

- [Design Decisions](decisions/README.md) - accepted records for decisions that
  need durable rationale
- [Investigations](investigations/README.md) - exploratory notes that informed
  implementation but are not normative guidance
