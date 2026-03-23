package sqlite

// schemaSQL defines the SQLite database schema for nitpicking.
const schemaSQL = `
-- Metadata table for database configuration.
CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

-- Tickets table. WITHOUT ROWID makes the ticket_id primary key the actual
-- B-tree key, avoiding sequential clustering from SQLite's implicit ROWID.
CREATE TABLE IF NOT EXISTS tickets (
    ticket_id           TEXT PRIMARY KEY,
    role                TEXT NOT NULL CHECK(role IN ('task', 'epic')),
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    acceptance_criteria TEXT NOT NULL DEFAULT '',
    priority            TEXT NOT NULL DEFAULT 'P2',
    state               TEXT NOT NULL,
    parent_id           TEXT DEFAULT NULL REFERENCES tickets(ticket_id),
    created_at          TEXT NOT NULL,
    idempotency_key     TEXT DEFAULT NULL,
    deleted             INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_tickets_parent ON tickets(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tickets_state ON tickets(state) WHERE deleted = 0;
CREATE INDEX IF NOT EXISTS idx_tickets_priority_created ON tickets(priority, created_at) WHERE deleted = 0;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tickets_idempotency ON tickets(idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Facets table.
CREATE TABLE IF NOT EXISTS facets (
    ticket_id TEXT NOT NULL REFERENCES tickets(ticket_id),
    key       TEXT NOT NULL,
    value     TEXT NOT NULL,
    PRIMARY KEY (ticket_id, key)
) WITHOUT ROWID;

-- Notes table.
CREATE TABLE IF NOT EXISTS notes (
    note_id    INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  TEXT NOT NULL REFERENCES tickets(ticket_id),
    author     TEXT NOT NULL,
    created_at TEXT NOT NULL,
    body       TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notes_ticket ON notes(ticket_id);

-- Claims table.
CREATE TABLE IF NOT EXISTS claims (
    claim_id        TEXT PRIMARY KEY,
    ticket_id       TEXT NOT NULL REFERENCES tickets(ticket_id),
    author          TEXT NOT NULL,
    stale_threshold INTEGER NOT NULL,
    last_activity   TEXT NOT NULL
) WITHOUT ROWID;

CREATE UNIQUE INDEX IF NOT EXISTS idx_claims_ticket ON claims(ticket_id);

-- Relationships table.
CREATE TABLE IF NOT EXISTS relationships (
    source_id TEXT NOT NULL REFERENCES tickets(ticket_id),
    target_id TEXT NOT NULL REFERENCES tickets(ticket_id),
    rel_type  TEXT NOT NULL CHECK(rel_type IN ('blocked_by', 'blocks', 'cites', 'cited_by')),
    PRIMARY KEY (source_id, target_id, rel_type)
) WITHOUT ROWID;

CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);

-- History entries table.
CREATE TABLE IF NOT EXISTS history (
    entry_id   INTEGER PRIMARY KEY AUTOINCREMENT,
    ticket_id  TEXT NOT NULL REFERENCES tickets(ticket_id),
    revision   INTEGER NOT NULL,
    author     TEXT NOT NULL,
    timestamp  TEXT NOT NULL,
    event_type TEXT NOT NULL,
    changes    TEXT NOT NULL DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_history_ticket ON history(ticket_id, revision);

-- Full-text search for tickets (standalone, not external content).
-- WITHOUT ROWID tables lack rowid, so we use a standalone FTS table
-- and manage sync manually in the repository layer.
CREATE VIRTUAL TABLE IF NOT EXISTS tickets_fts USING fts5(
    ticket_id,
    title,
    description,
    acceptance_criteria
);

-- Full-text search for notes (standalone).
CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
    note_id,
    body
);
`
