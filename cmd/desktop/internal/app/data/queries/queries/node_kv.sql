-- name: GetNodeKV :one
-- Expiry-aware read: an expired row reads as absent (sql.ErrNoRows). The
-- final arg is an explicit `now` cutoff -- never a fresh time.Now() inside
-- the query -- so the expiry boundary is deterministically testable.
SELECT value FROM node_kv
WHERE flow_id = ? AND node_id = ? AND scope = ? AND key = ?
  AND (expires_at IS NULL OR expires_at > ?);

-- name: UpsertNodeKV :exec
INSERT INTO node_kv (flow_id, node_id, scope, key, value, expires_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (flow_id, node_id, scope, key) DO UPDATE SET
    value = excluded.value, expires_at = excluded.expires_at, updated_at = excluded.updated_at;

-- name: DeleteNodeKV :exec
DELETE FROM node_kv WHERE flow_id = ? AND node_id = ? AND scope = ? AND key = ?;

-- name: ListNodeKVKeysByPrefix :many
-- Unexpired keys under one node/scope whose key has the given prefix. Uses
-- GLOB, which is binary/case-SENSITIVE, so it matches Go strings.HasPrefix
-- exactly; default LIKE is ASCII case-insensitive and would silently
-- disagree on a mixed-case prefix. The wrapper escapes GLOB metacharacters
-- in the prefix and appends `*`; a literal leading prefix lets SQLite
-- range-scan the composite PK index. The final arg is an explicit `now`
-- cutoff.
SELECT key FROM node_kv
WHERE flow_id = ? AND node_id = ? AND scope = ?
  AND key GLOB sqlc.arg(pattern)
  AND (expires_at IS NULL OR expires_at > sqlc.arg(now))
ORDER BY key;

-- name: DeleteExpiredNodeKV :exec
DELETE FROM node_kv WHERE expires_at IS NOT NULL AND expires_at <= ?;

-- name: DeleteNodeKVByFlow :exec
DELETE FROM node_kv WHERE flow_id = ?;

-- name: DeleteNodeKVForFlowExceptNodes :exec
-- The empty-slice case is invalid SQL (NOT IN ()); callers with no node ids
-- to retain use DeleteNodeKVByFlow instead, mirroring the
-- DeleteFeedMembershipClaimsForFeeds / *All pair.
DELETE FROM node_kv WHERE flow_id = ? AND node_id NOT IN (sqlc.slice(node_ids));
