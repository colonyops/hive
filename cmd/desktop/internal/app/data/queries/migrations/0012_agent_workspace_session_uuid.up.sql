-- Chat row ids are durable identities. SQLite INTEGER PRIMARY KEY values can be
-- reused after the newest row is deleted, which can make a schedule run point
-- at an unrelated live chat. Move the identity and every stored reference to
-- time-ordered UUIDv7 text. legacy_id lets a chat that survived the app
-- restart keep using the numeric HIVE_AGENT_SESSION already in its environment.
CREATE TABLE agent_workspace_session_id_map (
    old_id INTEGER PRIMARY KEY,
    new_id TEXT NOT NULL UNIQUE
) STRICT;

INSERT INTO agent_workspace_session_id_map (old_id, new_id)
SELECT id,
       lower(substr(printf('%012x', created_at), 1, 8)) || '-' ||
       lower(substr(printf('%012x', created_at), 9, 4)) || '-7' ||
       lower(substr(hex(randomblob(2)), 2, 3)) || '-' ||
       substr('89ab', 1 + (random() & 3), 1) ||
       lower(substr(hex(randomblob(2)), 2, 3)) || '-' ||
       lower(hex(randomblob(6)))
FROM agent_workspace_session;

CREATE TABLE agent_workspace_session_v12 (
    id               TEXT PRIMARY KEY,
    legacy_id        INTEGER UNIQUE,
    workspace        TEXT NOT NULL,
    name             TEXT NOT NULL,
    agent            TEXT NOT NULL,
    agent_session_id TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    last_opened_at   INTEGER NOT NULL,
    schedule_id      TEXT NOT NULL DEFAULT '',
    end_token        TEXT NOT NULL DEFAULT '',
    terminal_id      TEXT NOT NULL DEFAULT ''
) STRICT;

INSERT INTO agent_workspace_session_v12 (
    id, legacy_id, workspace, name, agent, agent_session_id, created_at,
    last_opened_at, schedule_id, end_token, terminal_id
)
SELECT id_map.new_id, session.id, session.workspace, session.name, session.agent,
       session.agent_session_id, session.created_at, session.last_opened_at,
       session.schedule_id, session.end_token, session.terminal_id
FROM agent_workspace_session AS session
JOIN agent_workspace_session_id_map AS id_map ON id_map.old_id = session.id
ORDER BY session.id;

CREATE TABLE schedule_run_v12 (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace     TEXT NOT NULL,
    schedule_id   TEXT NOT NULL,
    schedule_name TEXT NOT NULL,
    scheduled_for INTEGER NOT NULL,
    started_at    INTEGER NOT NULL,
    reason        TEXT NOT NULL,
    missed        INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    session_id    TEXT,
    prompt        TEXT NOT NULL,
    error         TEXT NOT NULL DEFAULT ''
) STRICT;

INSERT INTO schedule_run_v12 (
    id, workspace, schedule_id, schedule_name, scheduled_for, started_at,
    reason, missed, status, session_id, prompt, error
)
SELECT run.id, run.workspace, run.schedule_id, run.schedule_name,
       run.scheduled_for, run.started_at, run.reason, run.missed, run.status,
       id_map.new_id, run.prompt, run.error
FROM schedule_run AS run
LEFT JOIN agent_workspace_session_id_map AS id_map ON id_map.old_id = run.session_id
ORDER BY run.id;

CREATE TABLE item_chat_v12 (
    chat_id      TEXT NOT NULL REFERENCES agent_workspace_session_v12(id) ON DELETE CASCADE,
    profile_id   TEXT NOT NULL,
    source_kind  TEXT NOT NULL,
    source_scope TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    PRIMARY KEY (chat_id, profile_id, source_kind, source_scope, external_id)
) STRICT;

INSERT INTO item_chat_v12 (
    chat_id, profile_id, source_kind, source_scope, external_id, created_at
)
SELECT id_map.new_id, item.profile_id, item.source_kind, item.source_scope,
       item.external_id, item.created_at
FROM item_chat AS item
JOIN agent_workspace_session_id_map AS id_map ON id_map.old_id = item.chat_id;

DROP TABLE item_chat;
DROP TABLE schedule_run;
DROP TABLE agent_workspace_session;

ALTER TABLE agent_workspace_session_v12 RENAME TO agent_workspace_session;
ALTER TABLE schedule_run_v12 RENAME TO schedule_run;
ALTER TABLE item_chat_v12 RENAME TO item_chat;

DELETE FROM sqlite_sequence WHERE name = 'schedule_run' AND seq = 0;

CREATE INDEX idx_agent_workspace_session_workspace
    ON agent_workspace_session(workspace, last_opened_at DESC);
CREATE UNIQUE INDEX idx_agent_workspace_session_terminal_id
    ON agent_workspace_session(terminal_id);
CREATE INDEX schedule_run_by_schedule
    ON schedule_run (workspace, schedule_id, started_at DESC);
CREATE INDEX idx_item_chat_item
    ON item_chat(profile_id, source_kind, source_scope, external_id, created_at DESC);

DROP TABLE agent_workspace_session_id_map;
