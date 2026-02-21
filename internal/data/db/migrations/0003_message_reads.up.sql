-- Message acknowledgment tracking
CREATE TABLE IF NOT EXISTS message_reads (
    message_id TEXT NOT NULL,
    consumer_id TEXT NOT NULL,  -- Session ID directly (no prefix)
    read_at INTEGER NOT NULL,   -- Unix nanoseconds
    PRIMARY KEY (message_id, consumer_id)
);

CREATE INDEX IF NOT EXISTS idx_message_reads_consumer ON message_reads(consumer_id, read_at);
CREATE INDEX IF NOT EXISTS idx_message_reads_message ON message_reads(message_id);
