-- Agent workspace chats this app opened on an inbox item's behalf: the chat
-- counterpart of item_session. A chat is a row in this database, so unlike a
-- hive session its link can cascade when the chat is deleted.
CREATE TABLE item_chat (
    chat_id      INTEGER NOT NULL REFERENCES agent_workspace_session(id) ON DELETE CASCADE,
    profile_id   TEXT NOT NULL,
    source_kind  TEXT NOT NULL,
    source_scope TEXT NOT NULL,
    external_id  TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    PRIMARY KEY (chat_id, profile_id, source_kind, source_scope, external_id)
) STRICT;

CREATE INDEX idx_item_chat_item
    ON item_chat(profile_id, source_kind, source_scope, external_id, created_at DESC);
