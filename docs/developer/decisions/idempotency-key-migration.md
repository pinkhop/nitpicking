# ADR: idempotency_key column → label carry-forward (v2 → v3 schema migration)

**Status:** Accepted  
**Date:** 2026-04-19  
**Issue:** NP-4k0xb (design task)  
**Implemented by:** NP-7047z (migration implementation)

---

## Context

Schema version 2 stored import-deduplication keys in a dedicated `idempotency_key TEXT DEFAULT NULL`
column on the `issues` table, backed by a partial unique index `idx_issues_idempotency`.
The v3 migration, introduced as part of the `refactor/IdempotencyLabelMigration` branch, drops
that column and carries any non-NULL values forward as ordinary label rows so that no data is lost.

Three design decisions had to be locked in before the migration function could be written: the
migration-key name, the collision-handling policy, and the label-value validation rule.

---

## Decision 1 — Migration-key naming

**Chosen key:** `idempotency`

Migrated rows land in the `labels` table as `(key = "idempotency", value = <original column value>)`,
i.e., the canonical string form is `idempotency:<value>`.

### Rationale

The v2 virtual-label machinery used the spelling `idempotency-key:<value>`. That spelling is retired
alongside the virtual-label machinery itself (NP-ssdcd). The new import subsystem uses the JSONL
field name `idempotency_label` and accepts any caller-chosen `key:value` string as the deduplication
marker; the key is not fixed — callers commonly use keys like `tracker` or `jira` (e.g.,
`tracker:idempotent-1`).

The migration, by contrast, must pick a **single** key under which all column values will land,
because the v2 column held bare values with no caller-supplied key. `idempotency` was chosen over
`idempotency-key` to match the new import field name `idempotency_label` and to drop the `-key`
suffix, which was an artefact of the v2 implementation detail (a column-name suffix) rather than a
user-facing concept. Changing the spelling at migration time is intentional: it signals that the
value now lives in the ordinary label store, not in a special column, and avoids any suggestion
that the retired virtual-label key (`idempotency-key`) is still honoured.

The key `idempotency` passes `domain.validateLabelKey`: it begins with an ASCII letter, is 11 bytes
(≤ 64), and contains only ASCII printable characters.

---

## Decision 2 — Collision policy

**Chosen policy:** skip-on-conflict

When the migration encounters an issue that already carries a label whose key is `idempotency`
(regardless of value), it does **not** write a new label row. The column value for that issue is
dropped silently from the perspective of the label store; the `MigrationResult` increments an
`IdempotencyKeysSkipped` counter so the operator can observe the anomaly.

### Rationale

Three options were considered:

| Option | Description | Verdict |
|--------|-------------|---------|
| (a) skip-on-conflict | Keep the existing label; drop the column value; increment skip counter | **Chosen** |
| (b) error | Abort the entire migration and surface an error to the operator | Rejected |
| (c) update/overwrite | Replace the existing label value with the column value | Rejected |

**Against option (b):** An abort policy means any operator with an edge-case database cannot
upgrade to v3 without manual intervention. Given that the `idempotency` key namespace was not
user-facing in v2, the probability of a meaningful collision is very low. Blocking all upgrades
for an unlikely edge case is disproportionate.

**Against option (c):** Overwriting a pre-existing label silently discards information. If an
operator manually added an `idempotency:<value>` label to an issue (e.g., to correct a stale
import), an overwrite would replace the corrected value with the stale column value. The label
must be treated as authoritative when it already exists.

**For option (a):** The existing label value already captures the intent; the column value adds
nothing. Counting skips in `MigrationResult.IdempotencyKeysSkipped` surfaces the event for any
operator who wants to audit.

---

## Decision 3 — Label-value validation rule

**Chosen rule:** skip invalid rows; increment `InvalidLabelValuesSkipped` counter; never fail.

For each non-NULL `idempotency_key` row, the migration calls `domain.NewLabel("idempotency", value)`.
If `NewLabel` returns an error, the row is skipped: no label is written for that issue's column value.
The `MigrationResult.InvalidLabelValuesSkipped` counter is incremented. The migration continues and
ultimately succeeds regardless of how many rows fail validation.

### Rationale

`idempotency_key` values in v2 databases are arbitrary user-supplied strings imported from JSONL
files that predated the `domain.NewLabel` validation layer. Some may contain whitespace, exceed
256 bytes, or lack an alphanumeric character — all of which are rejectable by `domain.NewLabel`.

Failing the migration over bad column data would block every v2 operator who imported such rows
from ever upgrading. Silently dropping the value without a counter would make the discrepancy
invisible. Exposing the count in `MigrationResult` provides an observable audit trail without
making the migration brittle.

#### Label-value constraints (summary, from `domain.NewLabel`)

| Rule | Limit |
|------|-------|
| Minimum length | 1 byte |
| Maximum length | 256 bytes |
| Character set | UTF-8 with no whitespace |
| Content | At least one alphanumeric character |

Any column value that violates these rules is skipped with the counter incremented.

---

## Implementation references

The migration function that implements these decisions is `MigrateV2ToV3` on `*sqlite.Store`,
introduced in NP-7047z. The function's godoc references this ADR by filename:

```
// See docs/developer/decisions/idempotency-key-migration.md for the migration-key
// naming rationale, collision-handling policy, and label-value validation rule.
```

The `MigrationResult` type (driven port: `internal/ports/driven/repository.go`; driving DTO:
`internal/ports/driving/dto.go`) carries:

- `IdempotencyKeysMigrated int` — column values successfully written as label rows.
- `IdempotencyKeysSkipped int` — column values not written because the label key already existed.
- `InvalidLabelValuesSkipped int` — column values not written because `domain.NewLabel` rejected the value.

---

## Consequences

- All v2 databases with well-formed `idempotency_key` values will have those values preserved as
  `idempotency:<value>` labels after the migration.
- v2 databases with invalid column values will lose those values silently, but the count is
  visible in `np admin upgrade` output so an operator can decide whether to recover them manually.
- The `idempotency` label key becomes a reserved-by-convention key for carry-forward data from v2
  databases. New imports under v3 are free to use any caller-chosen key in their `idempotency_label`
  field (e.g., `tracker:…`, `jira:…`), so `idempotency:<value>` labels in a v3 database will almost
  always be migrated values rather than new-import values. Operators who want to search for
  carry-forward rows specifically can filter with `--label idempotency:*`.
- The `idempotency-key:<value>` virtual-label spelling is fully retired and will not appear in
  any database schema v3 or later.
