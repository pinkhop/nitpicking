---
globs:
  - "*.go"
  - "go.mod"
  - "go.sum"
---

# Go Testing

These rules cover Go-specific testing mechanics. General testing philosophy (Given/When/Then structure, test doubles policy, test pyramid, pre-commit checklist) is defined in user-level rules and applies here without repetition.

## Test Organization

- Co-locate test files with the code they test: `foo_test.go` alongside `foo.go`.
- Use the `_test` package (e.g., `package alert_test`) by default to enforce black-box testing of the public API. Use same-package tests only when testing unexported behavior that cannot be reached through the public surface.
- Shared test helpers and fakes live in a dedicated package (e.g., `internal/testutil` or `internal/fake`), not scattered across test files.
- Call `t.Helper()` on every test helper function so failure output points to the caller, not the helper.

## Table-Driven Tests

- Use table-driven tests when multiple scenarios exercise the same code path with varying inputs and expected outputs.
- Name the slice `tests` or `cases`. Use descriptive subtest names via `t.Run(tc.name, ...)`.
- Each table entry maps to Given (inputs/preconditions) and Then (expected outputs). The When is the shared code path executed for every entry.
- Do not force table-driven structure when scenarios have meaningfully different setup or assertions. Separate test functions are clearer in those cases.

## Test Naming

- Convention: `TestTypeName_Scenario_ExpectedOutcome` (e.g., `TestEscalationPolicy_ExpiredThreshold_ReturnsError`).
- For package-level functions: `TestFunctionName_Scenario_ExpectedOutcome`.
- Subtest names in table-driven tests should read naturally in test output.

## Test Doubles (Go-Specific Mechanics)

- Implement fakes and stubs as concrete types satisfying interfaces — hand-written, not generated.
- Fakes live in a dedicated package (e.g., `internal/fake`) and are shared across tests.
- No mock generation tools (`mockgen`, `moq`, etc.).
- Use function-type fields in fakes when a test needs to control specific return values per call.

## Assertions

- Use the stdlib `testing` package only — no `testify` or other assertion libraries.
- Use `go-cmp` (`cmp.Diff`) for complex struct comparisons where manual field-by-field checks would be noisy.
- Preconditions in Given: fail immediately with `t.Fatalf` so the test does not continue into the When phase with invalid setup.
- Assertions in Then: use `t.Errorf` for non-fatal checks when you want to see all failures, `t.Fatalf` when subsequent assertions would be meaningless.
- Error assertions: use `errors.Is` and `errors.As` — never string-match on error messages.

## Integration Tests

- Gate with `//go:build integration` build tag.
- Use `testcontainers-go` for database and broker dependencies.
- One `TestMain` per integration test package to manage container lifecycle.
- Integration tests still follow one-scenario-per-test. The external system is real; the test structure is identical to unit tests.

## Parallel Execution

- Call `t.Parallel()` in unit tests by default.
- Do not use `t.Parallel()` in integration tests unless the test is explicitly designed for concurrent execution against shared infrastructure.
- Table-driven subtests: call `t.Parallel()` in the subtest closure and capture the loop variable.

## Test Data

- Static fixtures belong in a `testdata/` directory (Go tooling ignores this directory automatically).
- Use builder functions for constructing domain objects with sensible defaults and optional overrides (functional options or explicit field setting — not reflection-based magic).
