---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Go Idioms

## Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` — always add operational context.
- Check wrapped errors with `errors.Is(err, ErrTarget)` for sentinel errors and `errors.AsType[*MyError](err)` for custom error types — never use `==` or type assertions directly, as wrapped errors will not match.
- Use sentinel errors (`var ErrNotFound = errors.New(...)`) for expected conditions callers need to branch on.
- Use custom error types when callers need to extract structured data from the error.
- Use `errors.Join` to combine multiple errors in batch operations or cleanup paths where more than one error can occur.
- Never `panic` in library code; reserve for truly unrecoverable states in `main` or init.
- Every function and method call that returns an error MUST assign the error — no exceptions. Even when the error will be discarded, it must be assigned to `_`.
- All errors should be handled: wrapped with context, returned to the caller, or acted upon. **Exceptions:** errors from best-effort tasks (e.g., closing a database connection during shutdown, printing a diagnostic to stderr before exiting) may be discarded by assigning to `_`. When discarding:
  - The justification must be captured in a comment.
  - Discard at the highest level possible. A low-level component (e.g., a repository) must propagate the error; the caller responsible for the broader operation (e.g., a shutdown manager) decides whether the error is worth acting on or whether the process is about to terminate anyway.

### Examples

Assign errors, even when discarded:

```go
// Bad - ignores that fmt.Fprintf returns an error.

fmt.Fprintf(stderr, "unable to authenticate: %s\n", err.Error())
```

```go
// Good - explicitly assigns the error returned, even though it is discarded,
// and comments why it is discarded.

// Printing the error message before exiting is a best-effort attempt. We
// discard the error because there is no good way to handle it — the next
// statement is to exit with an error code anyway.
_, _ = fmt.Fprintf(stderr, "unable to authenticate: %s\n", err.Error())
```

## Naming

- Follow Go conventions for acronyms (`HTTPClient`, `userID`, not `HttpClient`, `userId`).
- Receiver names: short (1–2 chars), consistent across all methods on a type.
- Prefer descriptive function/method names and parameter names that convey intent, even if slightly longer than typical Go style — clarity at the call site matters more than brevity (e.g., `SendNotificationToSubscribers(ctx, alertThreshold, escalationPolicy)` over `Send(ctx, thresh, pol)`).
- Exported names should make sense when read with their package prefix: `alert.NewEscalationPolicy`, not `alert.NewAlertEscalationPolicy`.
- Boolean variables and fields: use `is`/`has`/`should` prefixes when it improves readability at the call site.
- Functions, types, package constants, and package variables (collectively: symbols) must never have a prefix matching the package name. A symbol may have the same name as the package when it is the key symbol in the package.

### Examples

Receiver names — short and consistent:

```go
// Bad - receiver name changes across methods on the same type.

func (ep *EscalationPolicy) Evaluate(ctx context.Context) error { ... }
func (policy *EscalationPolicy) Reset() { ... }
```

```go
// Good - same short receiver name on every method.

func (e *EscalationPolicy) Evaluate(ctx context.Context) error { ... }
func (e *EscalationPolicy) Reset() { ... }
```

Symbols must not have prefixes matching their package name:

```go
// Bad - type's name has a prefix matching the package name:
// version.VersionOptions

package version
type VersionOptions struct {}
```

```go
// Good - type's name does not have a prefix matching the package name; it is
// obvious that the Options are for a "version" because they are in the
// `version` package: version.Options

package version
type Options struct{}
```

## Interfaces

- Define interfaces in the **consuming** package, not the implementing package.
- Keep interfaces small — 1–3 methods is ideal.
- Avoid creating interfaces preemptively; extract them when a second consumer or a testing seam requires it.
- Accept interfaces, return concrete types. **Exceptions:** factories that return varied implementations, and APIs where the concrete type is deliberately hidden to preserve encapsulation.

## Structs & Constructors

- Use `New` constructors when initialization requires validation or has invariants.
- Use functional options pattern (`WithTimeout(d)`, `WithLogger(l)`) for constructors with more than 2–3 optional parameters.
- Prefer value receivers for immutable types, pointer receivers for mutable types. If any method on a type requires a pointer receiver, use pointer receivers on all methods — mixing receiver types causes confusing interface satisfaction behavior.
- Zero values should be useful wherever possible.

## Context

- Always the first parameter: `func Foo(ctx context.Context, ...)`.
- Never store `context.Context` in a struct field.
- Use context values only for request-scoped cross-cutting data (trace IDs, tenant IDs) — never for function parameters or optional behavior.
- Use `context.WithoutCancel` when spawning background work that should outlive a request but still needs to carry request-scoped values (trace IDs, etc.). Avoid using a bare `context.Background()` for this purpose, as it discards all values from the parent.

## Package Design

- Packages should provide, not contain — name by what they do, not what's in them (`alert`, not `models` or `utils`).
- Avoid `package utils`, `package helpers`, `package common` — find a real domain name or inline the code.
- Keep the `internal/` boundary intentional; move things out of `internal` only when you have a real external consumer.
- Resolve circular imports through dependency inversion (introduce an interface in the consuming package), merging closely related packages, or extracting a shared types package. Never work around circular imports by stuffing unrelated concerns into a single package.

## Control Flow

- Early returns over deep nesting — handle the error/edge case and return.
- Prefer `switch` over `if/else` chains when branching on more than two conditions.
- Use guard clause style: validate preconditions at the top of a function.

## Channels & Concurrency

### Channel Pipelines

- **Producers** own context cancellation: the pipeline source must monitor `ctx.Done()` and stop sending new values when the context is cancelled, then close its outbound channel.
- **Consumers** own channel draining: a receiver must always read from its inbound channel until the channel is closed, regardless of context cancellation — this prevents goroutine leaks from blocked senders.
- **Graceful degradation**: a receiver may monitor context cancellation and skip expensive processing on received values while still draining the channel. This is useful when processing is costly, or when processing a value would result in sending to downstream channels that may also be shutting down.
- The combination of these rules ensures that cancellation propagates cleanly: producers stop producing, consumers finish draining, and no goroutine is left blocked on a channel operation.

### Buffered vs. Unbuffered Channels

- Default to unbuffered channels; use buffered channels only when you can justify the buffer size with a concrete throughput or backpressure argument.
- Use a buffered channel of size 1 for "future" or "promise" patterns — where a goroutine sends exactly one value (or closes without sending) to signal completion or deliver an asynchronous result (e.g., an asynchronous error).

### Concurrency Primitives

- Use `sync.WaitGroup` to wait for a group of goroutines to finish.
- Use `errgroup.Group` when goroutines can fail and the caller needs to observe the first error.
- Use `sync.Once` for lazy, thread-safe initialization.
- Prefer channels for communication between goroutines. Prefer `sync.Mutex` for protecting shared state that is read/written but not communicated (e.g., an in-memory cache).

## Type Design

- Use custom types for domain identifiers to prevent accidental misuse (`type TenantID string` over bare `string`).
- Use `iota` for enumerations where the values have no inherent meaning outside the program.
  - Always include an explicit zero-value that represents "unspecified" or "unknown" to avoid accidental defaults.
  - When enumerated values have natural external representations (e.g., HTTP status codes, protocol identifiers), use explicit constants with those values instead of `iota`.
- Prefer composition over embedding unless the promoted method set is genuinely the right API for the outer type.

## Comments & Godoc

- Every type, function, method, and named constant gets a doc comment — not just exported symbols. Unexported symbols should explain their purpose and role within the package.
- Every package has a `doc.go` file containing the package-level documentation comment. The comment must follow Go convention: begin with `Package <name>` followed by a description of the package's purpose and responsibilities (e.g., `// Package telemetry provides OpenTelemetry instrumentation for outbound gRPC calls and Kafka producers.`).
- Exported symbols follow Go convention: doc comments start with the symbol name.
- Implementation comments should explain the **why** — especially when the approach deviates from the naive or obvious solution. Assume the reader has no prior context for the codebase. If a future maintainer would look at the code and ask "why is it doing it this way?", that's a signal a comment is needed.
- Use `// TODO(owner): description` format with an owner for actionable items.
