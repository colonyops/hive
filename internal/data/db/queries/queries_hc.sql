-- HC Items

-- name: CreateHCItem :exec
INSERT INTO hc_items (id, repo_key, epic_id, parent_id, session_id, title, desc, type, status, depth, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetHCItem :one
SELECT * FROM hc_items WHERE id = ?;

-- name: UpdateHCItem :exec
UPDATE hc_items SET status = ?, session_id = ?, updated_at = ? WHERE id = ?;

-- name: ListHCItemsBySession :many
SELECT * FROM hc_items WHERE session_id = ? ORDER BY created_at ASC;

-- name: ListHCItemsByEpic :many
SELECT * FROM hc_items WHERE epic_id = ? ORDER BY depth ASC, created_at ASC;

-- name: ListHCItemsByEpicAndStatus :many
SELECT * FROM hc_items WHERE epic_id = ? AND status = ? ORDER BY depth ASC, created_at ASC;

-- name: ListHCItemsByRepo :many
SELECT * FROM hc_items WHERE repo_key = ? ORDER BY created_at ASC;

-- name: ListHCEpics :many
SELECT * FROM hc_items WHERE type = 'epic' ORDER BY created_at DESC;

-- name: ListHCEpicsByRepo :many
SELECT * FROM hc_items WHERE type = 'epic' AND repo_key = ? ORDER BY created_at DESC;

-- name: ListAllHCItems :many
SELECT * FROM hc_items ORDER BY created_at DESC;

-- name: ListAllHCItemsByStatus :many
SELECT * FROM hc_items WHERE status = ? ORDER BY created_at DESC;

-- name: NextHCItemForSession :one
SELECT outer_item.* FROM hc_items AS outer_item
WHERE outer_item.session_id = ?
  AND outer_item.status = 'open'
  AND outer_item.id NOT IN (
    SELECT DISTINCT inner_item.parent_id FROM hc_items AS inner_item
    WHERE inner_item.parent_id != '' AND inner_item.status IN ('open', 'in_progress')
  )
ORDER BY outer_item.depth DESC, outer_item.created_at ASC
LIMIT 1;

-- name: NextHCItemForSessionInEpic :one
SELECT outer_item.* FROM hc_items AS outer_item
WHERE outer_item.session_id = ?
  AND outer_item.epic_id = ?
  AND outer_item.status = 'open'
  AND outer_item.id NOT IN (
    SELECT DISTINCT inner_item.parent_id FROM hc_items AS inner_item
    WHERE inner_item.parent_id != '' AND inner_item.status IN ('open', 'in_progress')
  )
ORDER BY outer_item.depth DESC, outer_item.created_at ASC
LIMIT 1;

-- name: CountHCOpenChildren :one
SELECT COUNT(*) FROM hc_items
WHERE parent_id = ? AND status IN ('open', 'in_progress');

-- name: ListHCBlockedParentIDs :many
SELECT DISTINCT parent_id FROM hc_items
WHERE parent_id != '' AND status IN ('open', 'in_progress');

-- name: DeleteHCItem :exec
DELETE FROM hc_items WHERE id = ?;

-- name: CountHCItemsByStatusOlderThan :one
SELECT COUNT(*) FROM hc_items WHERE status = ? AND updated_at < ?;

-- name: DeleteHCItemsByStatusOlderThan :exec
DELETE FROM hc_items WHERE status = ? AND updated_at < ?;

-- HC Comments

-- name: InsertHCComment :one
INSERT INTO hc_comments (id, item_id, message, created_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: ListHCComments :many
SELECT * FROM hc_comments WHERE item_id = ? ORDER BY created_at ASC;

-- name: PruneHCCommentsByStatus :exec
DELETE FROM hc_comments
WHERE hc_comments.item_id IN (
    SELECT hc_items.id
    FROM hc_items
    WHERE hc_items.status = ? AND hc_items.updated_at < ?
);
