# JSONL Import Format Specification

## Overview

The `np import jsonl` command reads a JSONL file (one JSON object per line) and creates issues in the np database. This document specifies the format of each line.

## Line Schema

Each line is a JSON object with the following fields:

| Field                 | Type             | Required | Description                                                                 |
|-----------------------|------------------|----------|-----------------------------------------------------------------------------|
| `idempotency_label`  | string           | Yes      | Caller-defined `key:value` label for deduplication; prevents duplicate imports on re-run |
| `role`               | string           | Yes      | `"task"` or `"epic"`                                                        |
| `title`              | string           | Yes      | Issue title                                                                 |
| `description`        | string           | No       | Issue description                                                           |
| `acceptance_criteria`| string           | No       | Acceptance criteria (separate from description)                             |
| `priority`           | string           | No       | `"P0"` through `"P4"`; defaults to `"P2"` if omitted                       |
| `state`              | string           | No       | `"open"`, `"deferred"`, or `"closed"`; defaults to `"open"` if omitted     |
| `author`             | string           | No       | Author for this issue; defaults to the `--author` flag on the import command. Ignored when `--force-author` is set |
| `comment`            | string           | No       | A single comment to attach to the issue; uses the same author as the issue  |
| `labels`             | object           | No       | Key-value pairs, e.g. `{"kind": "bug", "area": "auth"}`                    |
| `parent`             | string           | No       | Parent reference — idempotency label value or issue ID                      |
| `blocked_by`         | array of strings | No       | List of blockers — each is an idempotency label value or issue ID           |
| `blocks`             | array of strings | No       | List of issues this issue blocks — each is an idempotency label value or issue ID |
| `refs`               | array of strings | No       | List of informational references — each is an idempotency label value or issue ID |

### Field Details

**`idempotency_label`** — a caller-chosen `key:value` string that marks this import line for deduplication. It is required on every line, must be unique within a single import file, and must satisfy `domain.NewLabel` validation (see below). On re-import, any line whose `idempotency_label` matches a label already present on a non-deleted issue in the database is skipped — the existing issue wins and nothing is mutated.

The `key:value` pair is stored as an ordinary label on the created issue; there is no separate column or virtual-label machinery. The caller chooses both the key and the value freely. Common conventions include `jira:PROJ-1234`, `tracker:some-id`, or `source:internal-slug`. No label key is reserved by np for this purpose.

**Cross-field validation:** if `idempotency_label` is `"K:V1"` and the `labels` object also contains key `K` with a different value `V2`, the line is rejected with a validation error that names the key and both values. If both express the same `K:V`, the duplication is accepted as a no-op and the issue will carry exactly one label for key `K`.

**`state`** — the initial state of the imported issue. Valid values are `"open"`, `"deferred"`, and `"closed"`. The states `"claimed"` and `"blocked"` are not valid for import: `"claimed"` requires an active claim, and `"blocked"` is a secondary state. If omitted, defaults to `"open"`.

**`author`** — the author for this specific issue. If omitted, the `--author` flag on the import command is used. When `--force-author` is set on the import command, this field is ignored and all issues use the command-line author.

**`comment`** — a single comment string to attach to the issue after creation. The comment's author is the same as the issue's author (after applying `--author` / `--force-author` resolution). This is intended for consolidating context from an external system into one freeform comment — the caller is responsible for formatting the content.

**`parent`** — a single string that identifies the parent epic. The string is resolved as either an idempotency label value or an issue ID (see "Reference Resolution Rules" below). The referenced issue must have role `"epic"`.

**`blocked_by`**, **`blocks`**, and **`refs`** — arrays of strings following the same resolution rules as `parent`.

**`labels`** — a flat object of string keys to string values. Keys and values follow the same validation rules as `np label add`. No label key is reserved by np for import purposes.

### Reference Resolution Rules

A reference string in `parent`, `blocked_by`, `blocks`, or `refs` is classified as either an **idempotency label value** or an **issue ID** based on its format:

- **Issue ID format:** a string matching the database's issue prefix followed by a hyphen and exactly 5 lowercase Crockford Base32 characters (e.g., `"FOO-a3bxr"`). The prefix is determined by the database being imported into. Strings matching this format are resolved directly as issue IDs against the database.
- **Idempotency label value format:** any string that does not match the issue ID format is treated as an idempotency label value.

Idempotency label value resolution proceeds in order:

1. Check intra-file `idempotency_label` values. If a match is found (comparing the full `key:value` string), the reference resolves to the corresponding line's issue.
2. If no intra-file match, check the database for any non-deleted issue that already carries a label with the same `key:value` string.
3. If neither lookup succeeds, the line fails validation.

Forward references are explicitly supported: a child may appear before its parent in the file. The validation pass collects all `idempotency_label` values before resolving references.

## Worked Examples

### Example 1: Minimal Single-Issue Import

A single task with only the required fields:

```jsonl
{"idempotency_label": "fix:login-001", "role": "task", "title": "Fix login timeout on slow connections"}
```

**Result:** One task created with state `open`, default priority P2, no description, no relationships. The task carries the label `fix:login-001`; re-running the file is a no-op because the label is now present on a non-deleted issue.

### Example 2: Idempotent Re-Import with a Tracker Label

Using `idempotency_label` to mark each line with a Jira issue key so the import can be safely re-run:

```jsonl
{"idempotency_label": "jira:PROJ-101", "role": "task", "title": "Fix login timeout on slow connections", "priority": "P1"}
{"idempotency_label": "jira:PROJ-102", "role": "task", "title": "Investigate flaky CI on ARM runners", "state": "deferred", "priority": "P3"}
```

**Result:** Two tasks created. On a second run with the same file, both lines are skipped because the database already contains issues carrying the labels `jira:PROJ-101` and `jira:PROJ-102`. The labels are stored as ordinary labels on each issue, so `np list --label jira:PROJ-101` will find the issue.

### Example 3: Epic with Children, Labels, and Per-Line Author

An epic with two child tasks, each carrying labels. The children reference the parent by its `idempotency_label` value, demonstrating forward references (the epic appears last). One child overrides the import command's author:

```jsonl
{"idempotency_label": "import:auth-unit-tests", "role": "task", "title": "Add unit tests for token refresh", "parent": "import:auth-overhaul", "labels": {"kind": "test", "area": "auth"}, "priority": "P1", "author": "test-team"}
{"idempotency_label": "import:auth-integration", "role": "task", "title": "Add integration tests for OAuth flow", "parent": "import:auth-overhaul", "labels": {"kind": "test", "area": "auth"}, "priority": "P1"}
{"idempotency_label": "import:auth-overhaul", "role": "epic", "title": "Authentication overhaul", "description": "Modernize the authentication stack to support OAuth 2.0 and token refresh.", "labels": {"kind": "refactor", "area": "auth"}, "priority": "P1"}
```

**Result:** Three issues created. The epic `import:auth-overhaul` is the parent of both tasks. All three carry the `area:auth` label. `import:auth-unit-tests` is authored by `test-team`; the other two use the `--author` flag from the import command. The children appear before the parent in the file — this is valid because forward references are resolved during the validation pass. Each `idempotency_label` value is also stored as an ordinary label on its issue (e.g., the epic carries `import:auth-overhaul`).

### Example 4: Multi-Issue File with All Relationship Types

A file with blocking dependencies, blocks, and informational references, mixing intra-file idempotency label values with an existing np issue ID:

```jsonl
{"idempotency_label": "import:design-schema", "role": "task", "title": "Design database schema for notifications", "priority": "P1", "refs": ["FOO-adhmr"]}
{"idempotency_label": "import:impl-storage", "role": "task", "title": "Implement notification storage adapter", "blocked_by": ["import:design-schema"], "priority": "P1"}
{"idempotency_label": "import:impl-api", "role": "task", "title": "Implement notification API endpoints", "blocked_by": ["import:design-schema", "import:impl-storage"], "refs": ["import:impl-email"], "priority": "P1"}
{"idempotency_label": "import:impl-email", "role": "task", "title": "Implement email notification sender", "blocked_by": ["import:impl-storage"], "blocks": ["import:impl-api"], "priority": "P2"}
```

**Result:** Four tasks created.
- `import:impl-storage` is blocked by `import:design-schema` (intra-file reference).
- `import:impl-api` is blocked by both `import:design-schema` and `import:impl-storage` (intra-file), and refs `import:impl-email` (intra-file).
- `import:design-schema` refs `FOO-adhmr` (existing issue ID — detected by matching the database prefix and Crockford Base32 format).
- `import:impl-email` is blocked by `import:impl-storage` and blocks `import:impl-api`. The `blocks` relationship on `import:impl-email` and the `blocked_by` on `import:impl-api` referencing `import:impl-storage` are both valid ways to express the same dependency direction.

### Example 5: Migration with State, Comments, and Deferred Issues

Importing issues from an external tracker, preserving state and consolidating discussion history into single comments:

```jsonl
{"idempotency_label": "jira:PROJ-101", "role": "task", "title": "Upgrade database driver to v3", "state": "closed", "priority": "P1", "comment": "Migrated from Jira PROJ-101.\n\nOriginal discussion:\n- Alice: driver v3 fixes the connection pool leak\n- Bob: confirmed, tested in staging"}
{"idempotency_label": "jira:PROJ-102", "role": "task", "title": "Investigate flaky CI on ARM runners", "state": "deferred", "priority": "P3", "comment": "Migrated from Jira PROJ-102. Deferred pending ARM runner availability."}
{"idempotency_label": "jira:PROJ-103", "role": "epic", "title": "Q2 reliability improvements", "state": "open", "priority": "P2"}
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

**Practical transformation:** To clone an existing issue via import, pipe `show --json` through `jq` to add an `idempotency_label` and strip server-assigned fields:

```bash
np show FOO-abc12 --json \
  | jq '{idempotency_label: "clone:\(.id)", role, title, description, acceptance_criteria, priority, state, author, labels, parent: .parent_id}' \
  >> import.jsonl
```

The `parent` field rename (`parent_id` → `parent`) is deliberate: the import format uses `parent` to signal that the value may be either an idempotency label value or an issue ID, whereas `show --json` always emits `parent_id` with a resolved np issue ID. Extra fields from `show --json` (such as `id`, `revision`, `created_at`) are silently ignored by the importer.

**Relationships** from `show --json` use a different structure (an array of `{type, target_id}` objects) and are not directly portable. To re-create relationships, extract the relevant IDs and populate the `blocked_by`, `blocks`, or `refs` arrays in the import line.

## Design Decisions

**Why `idempotency_label` instead of `idempotency_key`?** The original v2 design stored deduplication keys in a dedicated `issues.idempotency_key` column, which required special-case code across validation, storage, and the label interface (via a virtual-label projection). The v3 design stores the deduplication marker as an ordinary label on the issue. This removes the dedicated column, eliminates virtual-label machinery, and lets callers choose both the label key and value freely — aligning deduplication with the label model the rest of np uses. A caller migrating from Jira can use `jira:PROJ-1234`; one with a generic tracker can use `tracker:some-id`. No label key is reserved for this purpose.

**Why is `idempotency_label` required?** Requiring the field forces every imported issue to carry an explicit deduplication marker, which makes import workflows resilient by default — re-running a file is safe, and partial-import recovery is straightforward. Callers who have no natural external key can synthesise one (e.g., a content hash or slug) rather than risking silent duplicate creation.

**Why a `key:value` string rather than separate fields?** Keeping `idempotency_label` as a single `key:value` string matches the shape of every other label reference in np (the `--label` filter flag, `np label add`, the `labels` array in `np json create`). Callers who already know their label key need only provide one field; callers who want to use the deduplication label directly in `labels` can do so without duplication.

**What about v2 databases with an `idempotency_key` column?** The v2→v3 schema migration (implemented as `MigrateV2ToV3` in the SQLite adapter, documented in `docs/developer/decisions/idempotency-key-migration.md`) carries non-NULL column values forward as ordinary labels with the key `idempotency`. For example, a v2 value `"my-import-slug"` becomes the label `idempotency:my-import-slug`. This key is reserved by convention for carry-forward data from v2 databases; new imports under v3 are free to use any caller-chosen key. Operators who want to find carry-forward rows can filter with `--label idempotency:*`.

**Why flat arrays for `blocked_by`, `blocks`, and `refs`?** Flat arrays of strings are jq-friendly and avoid nested objects. The resolution logic (idempotency label value vs. issue ID) is the same for every element, keeping parsing simple.

**Why `parent` instead of `parent_id`?** The field name `parent` signals that its value might be an intra-file idempotency label value — not necessarily a resolved issue ID. This distinguishes it from `show --json`'s `parent_id`, which is always a resolved ID.

**Why are extra fields silently ignored?** This enables piping `show --json` output with minimal transformation. Only recognized fields are processed; everything else is discarded without error.

**Why per-line `author` with `--force-author`?** Per-line authors support migration scenarios where different issues were created by different people. The `--force-author` boolean flag overrides all per-line authors with the `--author` value, which is useful when re-importing into a fresh workspace under a single identity or when the source system's author names are not meaningful in np.

**Why a single `comment` field instead of an array?** The import format is not designed to replicate another system's comment history. A single comment provides a place to consolidate context from an external system — the caller formats the content to meet their needs. This avoids the complexity of mapping between systems' author identities, timestamps, and threading models.

**Why does `closed` not require a reason?** Closed issues in np may optionally carry a closing reason, but it is not a data integrity requirement. Mandating a reason on import would make migration from systems that do not track close reasons unnecessarily difficult. A `comment` can serve as the closing rationale when one is available.

**How are issue IDs distinguished from idempotency label values?** A reference string is classified as an issue ID if it matches the database's prefix followed by a hyphen and exactly 5 lowercase Crockford Base32 characters (e.g., `"FOO-a3bxr"`). All other strings are treated as idempotency label values. This format-based classification eliminates ambiguity — there is never a question of whether a string is an issue ID or an idempotency label value.
