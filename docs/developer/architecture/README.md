# Architecture Docs

These documents explain how the codebase is structured and how contributor
changes should fit into that structure.

## Read In This Order

1. [Architecture](architecture.md) for layering and dependency rules
2. [Package Layout](package-layout.md) for "what goes where"
3. [CLI Implementation Guide](cli-implementation.md) for command, wiring, and
   testing conventions

## Use This Folder When

- you are deciding which layer owns a change
- you need to place new code in the right package
- you are changing commands, help output, startup, or error handling
