# Testing

## Test-Driven Development

Use red/green TDD.

## Test Structure: Given/When/Then

All automated tests must follow strict Given/When/Then structure with these rules:

### 1. One scenario per test (single "When" block)
- Each test validates exactly one behavior with one execution phase
- "One behavior" may have multiple steps, such as putting a value into a map and getting it back out
- No chaining additional When/Then phases after assertions
- If you need to test another behavior that depends on the post-state, write a separate test

### 2. "Given" is for preconditions only
- May call anything needed to establish preconditions (create objects, seed fakes, populate repos)
- If Given fails, fail the test with clear indication that preconditions failed, not the operation under test

### 3. "Then" must not call methods used in When
- Assertions cannot invoke functions/methods that were part of the When workflow
- Capture observations during When; assert on captured values in Then
- Use independent observation paths (fake state, domain events, returned values)

### 4. "Then" contains only assertions and simple inspection
- Assert on values captured from When
- Inspect fake/stub state or verify emitted events
- No new operations, branching workflows, or re-calling When methods

### 5. Need another "When"? Write another test
- Split scenarios into separate tests rather than extending existing ones

## Test Doubles: Avoid Mocks

Use stubs, spies, fakes, and dummies instead of mocks.

**Why no mocks:**
- Mocks verify interactions (how collaborators are called) instead of outcomes (what the system does)
- Creates dual responsibility: tests validate both behavior AND interaction contracts
- Brittle under refactoring: harmless internal changes break tests
- Couples tests to implementation details rather than observable behavior

**Use instead:**
- **Stubs**: Validate behavior through outputs
- **Spies**: Only when interaction observation is unavoidable
- **Fakes**: Higher-fidelity behavior without over-specifying interactions
- **Dummies**: Satisfy irrelevant dependencies without coupling

This ensures tests assert observable outcomes, remain resilient to internal restructuring, and avoid encoding incidental implementation details.

## Test Pyramid: Unit > Collaboration > Boundary > Component Blackbox

The vast majority of tests should be unit and collaboration tests. Boundary and component blackbox tests provide value but should be used sparingly.

### Definitions

**Unit test:**
- Runs without external systems (no databases, message queues, external services, etc.)
- Exercises a single unit — a function, struct, or class — using test doubles for all collaborators
- The foundation of test confidence
- **Default choice for new tests**

**Collaboration test** (also known as a "sociable test"):
- Runs without external systems (no databases, message queues, external services, etc.)
- Exercises multiple units wired together, using test doubles only at the boundary of the collaborating group
- Use when testing behavior that emerges from in-process collaboration rather than from any single unit

**Boundary test** (also known as an "adapter test"):
- Requires external systems (file system, database, message broker, web service, etc.)
- External systems are test-managed: each test controls setup and teardown of its own data
- Exercises the code that translates between in-process abstractions and external system protocols (e.g., a repository implementation, an HTTP client wrapper)
- Use when testing that an adapter faithfully implements its port contract against the real external system

**Blackbox component test** ("blackbox test" for short):
- Assembles and runs the full component as it would be deployed — a containerized service in Docker Compose, a compiled CLI binary invoked from a shell, etc.
- External systems are test-managed: each test controls setup and teardown of its own data
- Treats the component as opaque; interacts only through its public surface (HTTP endpoints, CLI commands, message queues)
- Usually exercises multi-step workflows that cross internal boundaries

### Test Execution

- Unit and collaboration tests run by default
- Boundary and blackbox component tests should **only run when explicitly requested** by the test runner
- Keep the boundary and blackbox suites separate and opt-in

### Rationale

Heavily testing components in isolation builds confidence in system behavior. Unit and collaboration tests provide:
- Fast feedback
- Clear failure localization
- Easy maintenance
- No environmental dependencies
- Foundation for refactoring confidence

Boundary and blackbox component tests complement but do not replace comprehensive unit and collaboration test coverage.

## Pre-commit Checklist
- [ ] Exactly one "When" block
- [ ] No additional "When"/"Then" after assertions
- [ ] No method on the object under test in "When" is called in "Then"
- [ ] "Then" only asserts on captured values
- [ ] Additional behavior verified in separate tests
- [ ] No mock test doubles
- [ ] New test is a unit test or collaboration unless testing external system boundaries
