-- +goose Up
-- Схема по ТЗ §6 (nodes, runtimes, bots, bot_events). Таблицы clients нет.
-- NOTIFY: триггеры на bots/runtimes → каналы bot_changes / runtime_changes.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE nodes (
    id              TEXT PRIMARY KEY,
    hostname        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'online'
                    CHECK (status IN ('online', 'offline', 'draining')),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_version   TEXT,
    meta            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE runtime_kind AS ENUM ('bot_runner', 'custom_bot');
CREATE TYPE desired_state AS ENUM ('running', 'stopped');
CREATE TYPE actual_state AS ENUM (
    'unknown', 'starting', 'running', 'stopping', 'stopped', 'failed', 'migrating'
);

CREATE TABLE runtimes (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind              runtime_kind NOT NULL,
    name              TEXT NOT NULL UNIQUE,

    start_command     TEXT NOT NULL,
    workdir           TEXT,
    env               JSONB NOT NULL DEFAULT '{}'::jsonb,

    desired_state     desired_state NOT NULL DEFAULT 'stopped',
    actual_state      actual_state  NOT NULL DEFAULT 'unknown',

    assigned_node_id  TEXT REFERENCES nodes(id),
    lease_owner       TEXT REFERENCES nodes(id),
    lease_until       TIMESTAMPTZ,

    pid               INT,
    exit_code         INT,
    last_error        TEXT,
    config_version    BIGINT NOT NULL DEFAULT 1,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE bot_type AS ENUM (
    'custom',
    'default',
    'default_extended'
);

CREATE TYPE bot_channel AS ENUM ('telegram', 'max');
CREATE TYPE bot_run_mode AS ENUM ('webhook', 'polling');

CREATE TABLE bots (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id         UUID,

    name              TEXT NOT NULL,
    bot_type          bot_type NOT NULL,
    custom_name       TEXT,
    channel           bot_channel NOT NULL,
    run_mode          bot_run_mode NOT NULL DEFAULT 'webhook',

    port              INT NOT NULL,
    token_ref         TEXT NOT NULL,

    runtime_id        UUID REFERENCES runtimes(id),

    artifact_path     TEXT,
    repo_url          TEXT,
    start_command     TEXT,

    desired_state     desired_state NOT NULL DEFAULT 'stopped',
    actual_state      actual_state  NOT NULL DEFAULT 'unknown',
    assigned_node_id  TEXT REFERENCES nodes(id),

    last_error        TEXT,
    config_version    BIGINT NOT NULL DEFAULT 1,
    scenario_config   JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT bots_port_unique UNIQUE (port),
    CONSTRAINT bots_custom_name_required CHECK (
        (bot_type = 'custom' AND custom_name IS NOT NULL AND length(custom_name) > 0)
        OR (bot_type <> 'custom' AND custom_name IS NULL)
    )
);

CREATE INDEX bots_runtime_idx ON bots (runtime_id);
CREATE INDEX bots_node_desired_idx ON bots (assigned_node_id, desired_state);
CREATE INDEX bots_type_idx ON bots (bot_type);

CREATE TABLE bot_events (
    id           BIGSERIAL PRIMARY KEY,
    bot_id       UUID REFERENCES bots(id) ON DELETE CASCADE,
    runtime_id   UUID REFERENCES runtimes(id) ON DELETE SET NULL,
    node_id      TEXT,
    event_type   TEXT NOT NULL,
    message      TEXT,
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- LISTEN/NOTIFY: ускорение reconcile (poll остаётся safety net).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_bot_changes() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('bot_changes', COALESCE(NEW.id::text, OLD.id::text));
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION notify_runtime_changes() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('runtime_changes', COALESCE(NEW.id::text, OLD.id::text));
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER bots_notify
    AFTER INSERT OR UPDATE OR DELETE ON bots
    FOR EACH ROW EXECUTE FUNCTION notify_bot_changes();

CREATE TRIGGER runtimes_notify
    AFTER INSERT OR UPDATE OR DELETE ON runtimes
    FOR EACH ROW EXECUTE FUNCTION notify_runtime_changes();

-- +goose Down
DROP TRIGGER IF EXISTS runtimes_notify ON runtimes;
DROP TRIGGER IF EXISTS bots_notify ON bots;
DROP FUNCTION IF EXISTS notify_runtime_changes();
DROP FUNCTION IF EXISTS notify_bot_changes();
DROP TABLE IF EXISTS bot_events;
DROP TABLE IF EXISTS bots;
DROP TYPE IF EXISTS bot_run_mode;
DROP TYPE IF EXISTS bot_channel;
DROP TYPE IF EXISTS bot_type;
DROP TABLE IF EXISTS runtimes;
DROP TYPE IF EXISTS actual_state;
DROP TYPE IF EXISTS desired_state;
DROP TYPE IF EXISTS runtime_kind;
DROP TABLE IF EXISTS nodes;
