# JSONL Import Format Specification

## Overview

The `np import jsonl` command reads a JSONL file (one JSON object per line) and creates issues in the np database. This document specifies the format of each line.

## Line Schema

Each line is a JSON object with the following fields:

| Field                 | Type             | Required | Description                                                                 |
|-----------------------|------------------|----------|-----------------------------------------------------------------------------|
| `idempotency_key`    | string           | Yes      | Caller-defined unique key; prevents duplicate imports on re-run             |
| `role`               | string           | Yes      | `"task"` or `"epic"`                                                        |
| `title`              | string           | Yes      | Issue title                                                                 |
| `description`        | string           | No       | Issue description                                                           |
| `acceptance_criteria`| string           | No       | Acceptance criteria (separate from description)                             |
| `priority`           | string           | No       | `"P0"` through `"P4"`; defaults to `"P2"` if omitted                       |
| `state`              | string           | No       | `"open"`, `"deferred"`, or `"closed"`; defaults to `"open"` if omitted     |
| `author`             | string           | No       | Author for this issue; defaults to the `--author` flag on the import command. Ignored when `--force-author` is set |
| `comment`            | string           | No       | A single comment to attach to the issue; uses the same author as the issue  |
| `labels`             | object           | No       | Key-value pairs, e.g. `{"kind": "bug", "area": "auth"}`                    |
| `parent`             | string           | No       | Parent reference — idempotency key or issue ID                              |
| `blocked_by`         | array of strings | No       | List of blockers — each is an idempotency key or issue ID                   |
| `blocks`             | array of strings | No       | List of issues this issue blocks — each is an idempotency key or issue ID   |
| `refs`               | array of strings | No       | List of informational references — each is an idempotency key or issue ID   |

### Field Details

**`idempotency_key`** — an opaque string chosen by the caller. It must be unique within a single import file. On re-import, lines whose `idempotency_key` has already been imported are skipped. The key is stored in the `issues.idempotency_key` column and is queryable through the label interface as `idempotency-key:<value>` (see "Virtual Labels" below).

**`state`** — the initial state of the imported issue. Valid values are `"open"`, `"deferred"`, and `"closed"`. The states `"claimed"` and `"blocked"` are not valid for import: `"claimed"` requires an active claim, and `"blocked"` is a secondary state. If omitted, defaults to `"open"`.

**`author`** — the author for this specific issue. If omitted, the `--author` flag on the import command is used. When `--force-author` is set on the import command, this field is ignored and all issues use the command-line author.

**`comment`** — a single comment string to attach to the issue after creation. The comment's author is the same as the issue's author (after applying `--author` / `--force-author` resolution). This is intended for consolidating context from an external system into one freeform comment — the caller is responsible for formatting the content.

**`parent`** — a single string that identifies the parent epic. The string is resolved as either an idempotency key or an issue ID (see "Reference Resolution Rules" below). The referenced issue must have role `"epic"`.

**`blocked_by`**, **`blocks`**, and **`refs`** — arrays of strings following the same resolution rules as `parent`.

**`labels`** — a flat object of string keys to string values. Keys and values follow the same validation rules as `np label add`. The key `idempotency-key` is reserved and must not appear in the `labels` object — use the top-level `idempotency_key` field instead.

### Reference Resolution Rules

A reference string in `parent`, `blocked_by`, `blocks`, or `refs` is classified as either an **idempotency key** or an **issue ID** based on its format:

- **Issue ID format:** a string matching the database's issue prefix followed by a hyphen and exactly 5 lowercase Crockford Base32 characters (e.g., `"NP-a3bxr"`). The prefix is determined by the database being imported into. Strings matching this format are resolved directly as issue IDs against the database.
- **Idempotency key format:** any string that does not match the issue ID format is treated as an idempotency key.

Idempotency key resolution proceeds in order:

1. Check intra-file `idempotency_key` values. If a match is found, the reference resolves to the corresponding line's issue.
2. If no intra-file match, check the database for an existing issue with a matching `idempotency_key` column value.
3. If neither lookup succeeds, the line fails validation.

Forward references are explicitly supported: a child may appear before its parent in the file. The validation pass collects all `idempotency_key` values before resolving references.

### Virtual Labels

The `idempotency_key` column is exposed through the label interface as the virtual label key `idempotency-key`. This means:

- `np list --label idempotency-key:*` matches all issues with an idempotency key.
- `np list --label idempotency-key:<value>` matches issues with that specific key.
- The `idempotency-key` key must not be stored in the `labels` table — the column is the sole source of truth. The `admin doctor` command checks for and flags violations of this invariant.

## Worked Examples

### Example 1: Minimal Single-Issue Import

A single task with only the required fields:

```jsonl
{"idempotency_key": "fix-login-001", "role": "task", "title": "Fix login timeout on slow connections"}
```

**Result:** One task created with state `open`, default priority P2, no description, no relationships.

### Example 2: Epic with Children, Labels, and Per-Line Author

An epic with two child tasks, each carrying labels. The children reference the parent by `idempotency_key`, demonstrating forward references (the epic appears last). One child overrides the import command's author:

```jsonl
{"idempotency_key": "auth-unit-tests", "role": "task", "title": "Add unit tests for token refresh", "parent": "auth-overhaul", "labels": {"kind": "test", "area": "auth"}, "priority": "P1", "author": "test-team"}
{"idempotency_key": "auth-integration", "role": "task", "title": "Add integration tests for OAuth flow", "parent": "auth-overhaul", "labels": {"kind": "test", "area": "auth"}, "priority": "P1"}
{"idempotency_key": "auth-overhaul", "role": "epic", "title": "Authentication overhaul", "description": "Modernize the authentication stack to support OAuth 2.0 and token refresh.", "labels": {"kind": "refact", "area": "auth"}, "priority": "P1"}
```

**Result:** Three issues created. The epic `auth-overhaul` is the parent of both tasks. All three carry the `area:auth` label. `auth-unit-tests` is authored by `test-team`; the other two use the `--author` flag from the import command. The children appear before the parent in the file — this is valid because forward references are resolved during the validation pass.

### Example 3: Multi-Issue File with All Relationship Types

A file with blocking dependencies, blocks, and informational references, mixing intra-file keys with an existing np issue ID:

```jsonl
{"idempotency_key": "design-schema", "role": "task", "title": "Design database schema for notifications", "priority": "P1", "refs": ["NP-adhmr"]}
{"idempotency_key": "impl-storage", "role": "task", "title": "Implement notification storage adapter", "blocked_by": ["design-schema"], "priority": "P1"}
{"idempotency_key": "impl-api", "role": "task", "title": "Implement notification API endpoints", "blocked_by": ["design-schema", "impl-storage"], "refs": ["impl-email"], "priority": "P1"}
{"idempotency_key": "impl-email", "role": "task", "title": "Implement email notification sender", "blocked_by": ["impl-storage"], "blocks": ["impl-api"], "priority": "P2"}
```

**Result:** Four tasks created.
- `impl-storage` is blocked by `design-schema` (intra-file reference).
- `impl-api` is blocked by both `design-schema` and `impl-storage` (intra-file), and refs `impl-email` (intra-file).
- `design-schema` refs `NP-adhmr` (existing issue ID — detected by matching the database prefix and Crockford Base32 format).
- `impl-email` is blocked by `impl-storage` and blocks `impl-api`. The `blocks` relationship on `impl-email` and the `blocked_by` on `impl-api` referencing `impl-storage` are both valid ways to express the same dependency direction.

### Example 4: Migration with State, Comments, and Deferred Issues

Importing issues from an external tracker, preserving state and consolidating discussion history into single comments:

```jsonl
{"idempotency_key": "migrated-101", "role": "task", "title": "Upgrade database driver to v3", "state": "closed", "priority": "P1", "comment": "Migrated from Jira PROJ-101.\n\nOriginal discussion:\n- Alice: driver v3 fixes the connection pool leak\n- Bob: confirmed, tested in staging"}
{"idempotency_key": "migrated-102", "role": "task", "title": "Investigate flaky CI on ARM runners", "state": "deferred", "priority": "P3", "comment": "Migrated from Jira PROJ-102. Deferred pending ARM runner availability."}
{"idempotency_key": "migrated-103", "role": "epic", "title": "Q2 reliability improvements", "state": "open", "priority": "P2"}
```

**Result:** Three issues created with their respective states. The two tasks carry comments consolidating their external discussion history. The comment author for each issue matches the issue's author.

## Mapping from `np show --json`

The `np show --json` output includes fields that overlap with the import format:

| `show --json` field      | Import field           | Notes                                                    |
|--------------------------|------------------------|----------------------------------------------------------|
| `role`                   | `role`                 | Direct mapping                                           |
| `title`                  | `title`                | Direct mapping                                           |
| `description`            | `description`          | Direct mapping                                           |
| `acceptance_criteria`    | `acceptance_criteria`  | Direct mapping                                           |
| `priority`               | `priority`             | Direct mapping                                           |
| `state`                  | `state`                | Direct mapping; only `open`, `deferred`, and `closed` are valid for import |
| `author`                 | `author`               | Direct mapping; overridden by `--force-author` if set    |
| `labels`                 | `labels`               | Direct mapping (same `{"key": "value"}` structure)       |
| `parent_id`              | `parent`               | Renamed; `show` uses `parent_id`, import uses `parent`   |
| `id`                     | —                      | Ignored; np assigns new IDs on import                    |
| `revision`               | —                      | Ignored                                                  |
| `relationships`          | —                      | Not directly usable; see below                           |
| `created_at`             | —                      | Ignored                                                  |

**Practical transformation:** To clone an existing issue via import, pipe `show --json` through `jq` to add the required `idempotency_key` and strip server-assigned fields:

```bash
np show NP-abc12 --json \
  | jq '{idempotency_key: "clone-of-\(.id)", role, title, description, acceptance_criteria, priority, state, author, labels, parent: .parent_id}' \
  >> import.jsonl
```

The `parent` field rename (`parent_id` → `parent`) is deliberate: the import format uses `parent` to signal that the value may be either an idempotency key or an issue ID, whereas `show --json` always emits `parent_id` with a resolved np issue ID. Extra fields from `show --json` (such as `id`, `revision`, `created_at`) are silently ignored by the importer.

**Relationships** from `show --json` use a different structure (an array of `{type, target_id}` objects) and are not directly portable. To re-create relationships, extract the relevant IDs and populate the `blocked_by`, `blocks`, or `refs` arrays in the import line.

## Design Decisions

**Why `idempotency_key` instead of reusing `id`?** Import creates new issues with np-assigned IDs. The source system's identifiers are meaningless to np. A separate, caller-chosen `idempotency_key` makes deduplication explicit and avoids conflating identity across systems.

**Why is the idempotency key stored in a column rather than as a label?** The `idempotency_key` is stored in the `issues.idempotency_key` column and exposed through the label interface as the virtual key `idempotency-key`. Storing it in a dedicated column keeps deduplication lookups fast (indexed column vs. label table scan) while the virtual label projection provides a uniform query interface. The `labels` table must not contain rows with the key `idempotency-key` — `admin doctor` checks for this invariant.

**Why flat arrays for `blocked_by`, `blocks`, and `refs`?** Flat arrays of strings are jq-friendly and avoid nested objects. The resolution logic (idempotency key vs. issue ID) is the same for every element, keeping parsing simple.

**Why `parent` instead of `parent_id`?** The field name `parent` signals that its value might be an intra-file idempotency key — not necessarily a resolved issue ID. This distinguishes it from `show --json`'s `parent_id`, which is always a resolved ID.

**Why are extra fields silently ignored?** This enables piping `show --json` output with minimal transformation. Only recognized fields are processed; everything else is discarded without error.

**Why per-line `author` with `--force-author`?** Per-line authors support migration scenarios where different issues were created by different people. The `--force-author` boolean flag overrides all per-line authors with the `--author` value, which is useful when re-importing into a fresh workspace under a single identity or when the source system's author names are not meaningful in np.

**Why a single `comment` field instead of an array?** The import format is not designed to replicate another system's comment history. A single comment provides a place to consolidate context from an external system — the caller formats the content to meet their needs. This avoids the complexity of mapping between systems' author identities, timestamps, and threading models.

**Why does `closed` not require a reason?** Closed issues in np may optionally carry a closing reason, but it is not a data integrity requirement. Mandating a reason on import would make migration from systems that do not track close reasons unnecessarily difficult. A `comment` can serve as the closing rationale when one is available.

**How are issue IDs distinguished from idempotency keys?** A reference string is classified as an issue ID if it matches the database's prefix followed by a hyphen and exactly 5 lowercase Crockford Base32 characters (e.g., `"NP-a3bxr"`). All other strings are treated as idempotency keys. This format-based classification eliminates ambiguity — there is never a question of whether a string is an issue ID or an idempotency key.
