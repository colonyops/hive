-- source_head persists the last payload emitted for each source item. Producer
-- uses it to skip unchanged source values across process restarts.
CREATE TABLE source_head (
    topic   TEXT NOT NULL,
    key     TEXT NOT NULL,
    payload BLOB NOT NULL,
    PRIMARY KEY (topic, key)
) STRICT;
