# Blackbox Component Tests — Developer Guide

np's blackbox component tests use [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript), the Go community standard for CLI testing. Each test is a `.txtar` file in `testdata/` containing a script of np commands with inline assertions — the CLI invocations are front and centre, not buried in Go boilerplate.

## Quickstart

A blackbox component test is a plain-text file that runs np commands and checks their output:

```
# Initialize a workspace, create a task, and verify it appears in list.

np-init TEST
np-seed task 'My first task' test-agent TASK

exec np list --json
stdout 'My first task'
```

Save this as `test/blackbox/testdata/my_test.txtar` and run:

```bash
make test-blackbox
```

That's it. The testscript framework provides each `.txtar` file with an isolated temporary directory (`$WORK`), registers the np binary, and runs every file in `testdata/` as a separate subtest.

## Annotated Example

```
# Verify that a blocked task becomes ready after its blocker is closed.
#
# Lines starting with # are comments — use them to explain the test's
# purpose, not to restate what each command does.

# --- Given: two tasks where BLOCKED is blocked by BLOCKER ---

np-init WF                                                    # 1
np-seed task 'Prerequisite work' block-agent BLOCKER           # 2
np-seed task 'Dependent work' block-agent BLOCKED              # 3
np-seed-rel BLOCKED blocked_by BLOCKER block-agent             # 4

# --- When: check readiness before closing the blocker ---

exec np show $BLOCKED --json                                   # 5
np-json-has is_ready false                                     # 6

# --- When: close the blocker ---

exec np claim id $BLOCKER --author block-agent --json          # 7
np-capture BLOCKER_CLAIM claim_id                              # 8
exec np close $BLOCKER --claim $BLOCKER_CLAIM \          # 9
    --reason 'done' --json

# --- Then: the blocked task is now ready ---

exec np show $BLOCKED --json                                   # 10
np-json-has is_ready true                                      # 11
```

| Line | What it does |
|------|-------------|
| 1 | `np-init` initialises an np workspace in the test's isolated directory. |
| 2–3 | `np-seed` creates issues and captures their IDs in `$BLOCKER` and `$BLOCKED`. |
| 4 | `np-seed-rel` adds a `blocked_by` relationship using the captured IDs. |
| 5 | `exec np ...` runs the np binary. This is how you invoke np in testscript. |
| 6 | `np-json-has` asserts a field in the last command's JSON stdout has the expected value. |
| 7–8 | Claim the blocker and capture the claim ID into `$BLOCKER_CLAIM`. |
| 9 | Close the blocker using the captured claim. Lines can be continued with `\`. |
| 10–11 | After the blocker is closed, verify the blocked task is now ready. |

## Running Tests

```bash
make test-blackbox                                      # All blackbox component tests
go test -tags blackbox ./test/blackbox/                  # Same, via go test

# Run a specific test (substring match on .txtar filename):
go test -tags blackbox -run 'TestBlackbox_Script/workflow_atomic_edit' -v ./test/blackbox/
```

## Custom Commands

Custom commands handle setup and assertions. They are defined in `testscript_cmds_test.go` and available in every `.txtar` file.

### np-init

Initialise an np workspace with the given prefix.

```
np-init PREFIX
```

Runs `np init PREFIX` in `$WORK`. Fails the test if initialisation does not succeed.

### np-seed

Create an issue and set environment variables for subsequent commands.

```
np-seed ROLE TITLE AUTHOR ENV_VAR [flags...]
```

On success, sets `$ENV_VAR` to the created issue ID.

**Flags:**

| Flag | Effect |
|------|--------|
| `--parent $VAR` | Sets the parent issue (pass an env var from a prior seed). |
| `--claim` | Also claims the issue; sets `${ENV_VAR}_CLAIM` to the claim ID. |
| `--priority P0\|P1\|P2\|P3` | Sets the priority (default: P2). |
| `--label key:val` | Adds a label. Repeatable. |

**Examples:**

```
# Simple task.
np-seed task 'Fix login bug' agent TASK

# Epic with children.
np-seed epic 'Auth overhaul' agent EPIC
np-seed task 'Add OAuth' agent CHILD --parent $EPIC

# Claimed task with priority and label.
np-seed task 'Urgent fix' agent TASK --claim --priority P0 --label kind:bug
```

### np-seed-comment

Add a comment to an issue.

```
np-seed-comment ISSUE_ENV BODY AUTHOR
```

`ISSUE_ENV` is the name of the environment variable (not `$ISSUE_ENV` — the command reads the env var itself).

### np-seed-rel

Add a relationship between two issues.

```
np-seed-rel SOURCE_ENV REL_TYPE TARGET_ENV AUTHOR
```

Both `SOURCE_ENV` and `TARGET_ENV` are environment variable names (not `$` references). `REL_TYPE` is one of: `blocked_by`, `blocks`, `refs`, `cites`, `cited_by`, `parent_of`, `child_of`.

### np-capture

Extract a field from the last command's JSON stdout and set an environment variable.

```
np-capture ENV_VAR JSON_FIELD
```

Parses stdout from the most recent `exec` as JSON, extracts the named top-level field, and sets `$ENV_VAR` to its string value. Numbers without fractional parts are formatted as integers (e.g., `"1"` not `"1.000000"`).

**Example:**

```
exec np claim id $TASK --author agent --json
np-capture CLAIM_ID claim_id

exec np close $TASK --claim $CLAIM_ID --reason 'done' --json
```

### np-json-has

Assert that a JSON field in the last command's stdout has the expected value.

```
np-json-has FIELD VALUE
! np-json-has FIELD VALUE     # assert the field does NOT have that value
```

Values are compared as strings. Use `true`/`false` for booleans, integer form for whole numbers.

**Examples:**

```
exec np show $TASK --json
np-json-has state closed
np-json-has priority P0
np-json-has is_ready true
! np-json-has issue_id $OTHER_TASK
```

### np-json-array-len

Assert that a JSON array field has exactly N elements.

```
np-json-array-len FIELD N
! np-json-array-len FIELD N   # assert the array does NOT have N elements
```

**Example:**

```
exec np comment list $TASK --json
np-json-array-len comments 3
```

### np-json-no-field

Assert that a field is absent from the last command's JSON stdout.

```
np-json-no-field FIELD
! np-json-no-field FIELD      # assert the field IS present
```

**Example:**

```
exec np show $TASK --json
np-json-no-field claim_id          # bearer token must not leak
! np-json-no-field claim_author    # but claim author should be present
```

## Value Threading

Testscript uses environment variables to pass data between commands. The pattern is:

1. **`np-seed`** sets `$VAR` (and `${VAR}_CLAIM` with `--claim`).
2. **`np-capture`** extracts a field from the last exec's JSON stdout into `$VAR`.
3. Subsequent commands reference `$VAR` in their arguments.

```
np-seed task 'Task A' agent TASK_A --claim
np-seed task 'Task B' agent TASK_B

# Thread the claim ID from a manual claim.
exec np claim id $TASK_B --author agent --json
np-capture CLAIM_B claim_id

# Use both captured values.
exec np close $TASK_B --claim $CLAIM_B --reason 'done' --json
```

**Important:** `np-seed-comment` and `np-seed-rel` take *environment variable names* as arguments (e.g., `TASK`), not the expanded values (`$TASK`). This is because they read the env var internally. All other commands use the expanded `$VAR` form.

## Assertion Strategies

### JSON Output

For structured assertions on `--json` output, use the custom commands:

```
exec np show $TASK --json
np-json-has state closed           # exact field match
np-json-has is_ready false         # boolean as string
np-json-array-len items 3          # array length
np-json-no-field claim_id          # field absent
```

For nested structures or values that the custom commands cannot reach, fall back to `stdout` regex matching:

```
exec np show $TASK --json
stdout '"type": "blocked_by"'
stdout '"target_id":'
```

### Text Output

For unstructured text output, use `stdout` and `! stdout`:

```
exec np version
stdout 'np version'                     # output contains "np version"
! stdout 'unknown'                      # output does NOT contain "unknown"
```

`stdout` interprets its argument as a Go regular expression, matched against each line of stdout. Special regex characters (`[`, `]`, `.`, `*`, `(`, `)`, etc.) must be escaped with `\` when you want a literal match.

### Expected Failures

Use `!` before `exec` to assert that a command exits with a non-zero status:

```
! exec np close $EPIC --claim $CLAIM --reason 'done' --json
```

### Filesystem Assertions

Use `exists` and `! exists` to check whether files or directories were created:

```
! exec np show NP-00000
! exists .np                            # show must not create .np/
```

### Archive Sections

Files needed before the test runs can be declared in archive sections at the bottom of the `.txtar` file:

```
np-init QS
exec np admin where
stdout '.np'

-- .gitignore --
.np/
```

Archive files are created in `$WORK` before any script commands execute. Use them for fixtures like `.gitignore`, `CLAUDE.md`, or JSON input files.

## Naming Conventions

Test files use snake_case names that describe the scenario:

```
workflow_task_lifecycle_defer.txtar
readiness_childless_epic_is_ready.txtar
history_claim_redact_json.txtar
seed_blocked_pair.txtar
```

Group related tests with a shared prefix (e.g., `workflow_`, `readiness_`, `seed_`, `admin_`).

## Test Isolation

Each `.txtar` file runs in its own temporary directory (`$WORK`). Tests do not share state. The np binary is registered via `testscript.RunMain` in `testscript_test.go` and invoked with `exec np ...`. No external binary is needed — the test binary itself acts as the np binary when re-invoked by testscript.
