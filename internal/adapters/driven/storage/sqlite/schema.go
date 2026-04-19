package sqlite

// schemaSQL defines the SQLite database schema for nitpicking.
const schemaSQL = `
-- Metadata table for database configuration.
CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

-- Issues table. WITHOUT ROWID makes the issue_id primary key the actual
-- B-tree key, avoiding sequential clustering from SQLite's implicit ROWID.
CREATE TABLE IF NOT EXISTS issues (
    issue_id           TEXT PRIMARY KEY,
    role                TEXT NOT NULL CHECK(role IN ('task', 'epic')),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    acceptance_criteria TEXT NOT NULL DEFAULT '',
    priority            TEXT NOT NULL DEFAULT 'P2',
    state               TEXT NOT NULL,
    parent_id           TEXT DEFAULT NULL REFERENCES issues(issue_id),
    created_at          TEXT NOT NULL,
    idempotency_key     TEXT DEFAULT NULL,
    deleted             INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_issues_parent ON issues(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_issues_state ON issues(state) WHERE deleted = 0;
CREATE INDEX IF NOT EXISTS idx_issues_priority_created ON issues(priority, created_at) WHERE deleted = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_issues_idempotency ON issues(idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Labels table.
CREATE TABLE IF NOT EXISTS labels (
    issue_id TEXT NOT NULL REFERENCES issues(issue_id),
    key       TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (issue_id, key)
) WITHOUT ROWID;

-- Comments table.
CREATE TABLE IF NOT EXISTS comments (
    comment_id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id   TEXT NOT NULL REFERENCES issues(issue_id),
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    body       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id);

-- Claims table.
CREATE TABLE IF NOT EXISTS claims (
    claim_sha512    TEXT PRIMARY KEY,
    issue_id       TEXT NOT NULL REFERENCES issues(issue_id),
    author          TEXT NOT NULL,
    stale_threshold INTEGER NOT NULL,
    last_activity   TEXT NOT NULL
) WITHOUT ROWID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_issue ON claims(issue_id);

-- Relationships table.
CREATE TABLE IF NOT EXISTS relationships (
    source_id TEXT NOT NULL REFERENCES issues(issue_id),
    target_id TEXT NOT NULL REFERENCES issues(issue_id),
    rel_type  TEXT NOT NULL CHECK(rel_type IN ('blocked_by', 'blocks', 'refs')),
    PRIMARY KEY (source_id, target_id, rel_type)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);

-- History entries table.
CREATE TABLE IF NOT EXISTS history (
    entry_id   INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id  TEXT NOT NULL REFERENCES issues(issue_id),
    revision   INTEGER NOT NULL,
    author     TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    event_type TEXT NOT NULL,
    changes    TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_history_issue ON history(issue_id, revision);

-- Full-text search for issues (standalone, not external content).
-- WITHOUT ROWID tables lack rowid, so we use a standalone FTS table
-- and manage sync manually in the repository layer.
CREATE VIRTUAL TABLE IF NOT EXISTS issues_fts USING fts5(
    issue_id,
    title,
    description,
    acceptance_criteria
);

-- Full-text search for comments (standalone).
CREATE VIRTUAL TABLE IF NOT EXISTS comments_fts USING fts5(
    comment_id,
    body
);
`
