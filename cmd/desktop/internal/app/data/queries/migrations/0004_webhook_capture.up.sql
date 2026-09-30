-- The most recent request body received per webhook source topic
-- ("source:<flowId>/<nodeId>"). One row per topic, replaced on every
-- delivery — including deliveries whose payload was unchanged and produced
-- no event_log row. Read by the node editor's payload preview, its
-- feed-shape hint, and the LLM transform prompt.
CREATE TABLE webhook_capture (
    topic       TEXT PRIMARY KEY,
    received_at INTEGER NOT NULL,
    body        BLOB NOT NULL
) STRICT;
