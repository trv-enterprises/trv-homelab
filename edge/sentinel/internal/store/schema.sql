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
    raw          TEXT NOT NULL DEFAULT '',
    -- 0.4.0 (dashboard alerts); added by migration on older files
    link           TEXT NOT NULL DEFAULT '',
    dashboard_id   TEXT NOT NULL DEFAULT '',
    dashboard_vars TEXT NOT NULL DEFAULT '{}',
    store_name     TEXT NOT NULL DEFAULT '',
    subtitle       TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS alerts_review_id ON alerts(review_id) WHERE review_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS alerts_rule_status ON alerts(kind, rule, status);
CREATE INDEX IF NOT EXISTS alerts_updated_at ON alerts(updated_at);
CREATE INDEX IF NOT EXISTS alerts_rule_updated ON alerts(rule, updated_at);

CREATE TABLE IF NOT EXISTS devices (
    token       TEXT PRIMARY KEY,
    env         TEXT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    last_seen   TEXT NOT NULL,
    -- 0.7.0: whose phone this is; added by migration on older files
    person      TEXT NOT NULL DEFAULT ''
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
    always_notify INTEGER NOT NULL DEFAULT 0,
    passive       INTEGER NOT NULL DEFAULT 0
);

-- Small key/value settings (server-wide; the adoption marker).
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- 0.7.0: everything a person chooses is keyed by person. mutes, policies and
-- the quiet-hours row in settings above are the single-person tables from
-- before; Adopt copies them to the owner once and never touches them again,
-- so a rollback finds them as they were.
CREATE TABLE IF NOT EXISTS person_mutes (
    person TEXT NOT NULL,
    rule   TEXT NOT NULL,
    until  TEXT,
    PRIMARY KEY (person, rule)
);

CREATE TABLE IF NOT EXISTS person_policies (
    person        TEXT NOT NULL,
    source        TEXT NOT NULL,
    repeats       TEXT NOT NULL DEFAULT 'critical',
    objects       TEXT NOT NULL DEFAULT '[]',
    always_notify INTEGER NOT NULL DEFAULT 0,
    passive       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (person, source)
);

-- Per-person key/value: quiet hours, the Outpost user id.
CREATE TABLE IF NOT EXISTS person_settings (
    person TEXT NOT NULL,
    key    TEXT NOT NULL,
    value  TEXT NOT NULL,
    PRIMARY KEY (person, key)
);

-- An alert a person never sees in their own history: its source was muted
-- for them, or filtered out, when it fired. No row means visible, which is
-- why every alert from before 0.7.0 is visible to everyone.
CREATE TABLE IF NOT EXISTS hidden_alerts (
    alert_id TEXT NOT NULL,
    person   TEXT NOT NULL,
    reason   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (alert_id, person)
);
CREATE INDEX IF NOT EXISTS hidden_alerts_person ON hidden_alerts(person);
