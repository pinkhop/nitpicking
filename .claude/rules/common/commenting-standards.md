# Documentation Comment Standards

Every exported and unexported type, function, method, constant, variable, property, and interface must have a documentation comment using the language's idiomatic style (e.g., `///` in Rust, `/** */` in Java/TypeScript, `//` preceding declarations in Go, `"""` docstrings in Python, `///` with XML tags in C#).

## Function and Method Documentation

Document the **what** and **why**, not the **how**:
- State what the function does and its purpose — why and when a caller would use it
- Do not describe internal implementation unless the algorithm or approach is part of the function's contract and matters to callers
- Document all parameters, including units, valid ranges, and whether nil/null is accepted
- Document return values, including whether nil/null can be returned — except when language convention makes it obvious (e.g., Go's `(T, error)` pattern, Rust's `Option<T>`)
- Document errors, exceptions, or failure modes the caller must handle
- Document any side effects (I/O, state mutation, cache invalidation, goroutine/thread spawning)
- Document thread-safety and concurrency guarantees when relevant
- Note preconditions or invariants the caller is responsible for

## Inline Comments (Within Function Bodies)

- Use section comments to delineate logical steps in multi-step operations; group related statements together with whitespace separating steps
- Beyond step delineation, comment sparingly — focus on **why**, not what
- Explain non-obvious decisions: workarounds, performance tradeoffs, edge cases, regulatory or business-rule motivations
- Do not restate what the code already expresses clearly

## General Principles

- Never generate placeholder or stub comments (e.g., `// TODO: add docs`); write the actual documentation or flag it for the user
- Keep comments current — when modifying code, update affected documentation in the same change
- Prefer self-documenting names and types to reduce comment burden, but do not use this as a reason to omit documentation comments
