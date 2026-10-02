-- sentinel schema. Applied in full on every open; every statement is
-- idempotent so a restart against an existing file is a no-op.

CREATE TABLE IF NOT EXISTS alerts (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    status       TEXT NOT NULL,
    severity     TEXT NOT NULL,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    rule         TEXT NOT NULL,
    device       TEXT NOT NULL DEFAULT '',
    camera       TEXT NOT NULL DEFAULT '',
    review_id    TEXT,
    event_ids    TEXT NOT NULL DEFAULT '[]',
    objects      TEXT NOT NULL DEFAULT '[]',
    thumb_path   TEXT NOT NULL DEFAULT '',
    start_ts     REAL NOT NULL DEFAULT 0,
    end_ts       REAL NOT NULL DEFAULT 0,
    started_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    resolved_at  TEXT,
    repeat_count INTEGER NOT NULL DEFAULT 0,
    raw          TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS alerts_review_id ON alerts(review_id) WHERE review_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS alerts_rule_status ON alerts(kind, rule, status);
CREATE INDEX IF NOT EXISTS alerts_updated_at ON alerts(updated_at);

CREATE TABLE IF NOT EXISTS devices (
    token       TEXT PRIMARY KEY,
    env         TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    last_seen   TEXT NOT NULL
);

-- Phase 3: server-side mutes. Created now so the schema never needs a
-- migration step for it.
CREATE TABLE IF NOT EXISTS mutes (
    rule  TEXT PRIMARY KEY,
    until TEXT
);

-- Per-source notification policy (0.3.0). Absent row = defaults.
CREATE TABLE IF NOT EXISTS policies (
    source        TEXT PRIMARY KEY,
    repeats       TEXT NOT NULL DEFAULT 'critical',
    objects       TEXT NOT NULL DEFAULT '[]',
    always_notify INTEGER NOT NULL DEFAULT 0
);

-- Small key/value settings (quiet hours).
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
