-- SQLite does not support DROP COLUMN; recreate table without parent_id
DROP VIEW IF EXISTS topics;
CREATE TABLE messages_backup AS SELECT id, topic, payload, sender, session_id, created_at FROM messages;
DROP TABLE messages;
ALTER TABLE messages_backup RENAME TO messages;
CREATE INDEX IF NOT EXISTS idx_messages_topic_created ON messages(topic, created_at);
CREATE VIEW IF NOT EXISTS topics AS
SELECT
    topic AS name,
    MAX(created_at) AS updated_at
FROM messages
GROUP BY topic
ORDER BY updated_at DESC;
