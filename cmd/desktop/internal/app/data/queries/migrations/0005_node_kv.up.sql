-- Durable per-node key-value state for function nodes: the dedup /
-- change-detection memory behind a script's `kv` object. Written only inside
-- CommitBatch's transaction, so a row is durable iff the tick that wrote it
-- committed. The scope column reserves room for a future flow/global scope
-- without a schema change; today every row is 'node'.
-- All timestamp columns store unix milliseconds (baseline convention);
-- expires_at NULL means no expiry.
CREATE TABLE node_kv (
    flow_id    TEXT NOT NULL,
    node_id    TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT 'node',
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    expires_at INTEGER,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (flow_id, node_id, scope, key)
) STRICT;

-- Partial index so the TTL sweep scans only expiring rows.
CREATE INDEX idx_node_kv_expires_at ON node_kv(expires_at) WHERE expires_at IS NOT NULL;
