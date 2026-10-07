CREATE TABLE usage_metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    installation_id TEXT NOT NULL,
    clear_cutoff_ns INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE usage_event (
    event_id TEXT PRIMARY KEY,
    occurred_at_ms INTEGER NOT NULL,
    recorded_at_ms INTEGER NOT NULL,
    name TEXT NOT NULL CHECK (name IN ('cli.command.completed', 'hive.session.created', 'terminal.session.started')),
    schema_version INTEGER NOT NULL,
    installation_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    surface TEXT NOT NULL CHECK (surface IN ('cli', 'desktop')),
    app_version TEXT NOT NULL,
    release_channel TEXT NOT NULL,
    properties_json TEXT NOT NULL CHECK (json_valid(properties_json))
);
CREATE INDEX usage_event_name_time ON usage_event(name, occurred_at_ms);
