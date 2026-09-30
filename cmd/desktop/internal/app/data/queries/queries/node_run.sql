-- name: InsertNodeRun :exec
INSERT INTO node_run (flow_id, node_id, ok, in_count, out_count, drop_count, err, ended_at, dur_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListNodeRunsByFlow :many
-- Recent runs, newest first: the canvas derives latest-per-node status and a
-- RECENT list from this page rather than querying per-node.
SELECT * FROM node_run
WHERE flow_id = ?
ORDER BY ended_at DESC
LIMIT ?;

-- name: PruneNodeRuns :exec
-- Retain the newest rows globally. rowid breaks same-nanosecond ties, so the
-- limit is exact even when a fast batch stamps equal ended_at values.
DELETE FROM node_run
WHERE rowid IN (
    SELECT rowid FROM node_run
    ORDER BY ended_at DESC, rowid DESC
    LIMIT -1 OFFSET ?
);
