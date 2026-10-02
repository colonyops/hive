-- name: LinkItemChat :exec
INSERT INTO item_chat (chat_id, profile_id, source_kind, source_scope, external_id, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (chat_id, profile_id, source_kind, source_scope, external_id) DO NOTHING;

-- name: ListItemChats :many
-- Newest first, as ListItemSessions. The join reads the chat's current name,
-- so a chat renamed after launch reports what it is now.
SELECT item_chat.chat_id, agent_workspace_session.workspace, agent_workspace_session.name, item_chat.created_at
FROM item_chat
JOIN agent_workspace_session ON agent_workspace_session.id = item_chat.chat_id
WHERE item_chat.profile_id = ? AND item_chat.source_kind = ? AND item_chat.source_scope = ? AND item_chat.external_id = ?
ORDER BY item_chat.created_at DESC, item_chat.chat_id DESC;

-- name: DeleteItemChatsByProfile :exec
DELETE FROM item_chat WHERE profile_id = ?;

-- name: RescopeItemChats :exec
-- The chat counterpart of RescopeItemSessions.
UPDATE OR IGNORE item_chat SET source_scope = sqlc.arg(source_scope)
WHERE profile_id = sqlc.arg(profile_id)
  AND source_kind = sqlc.arg(source_kind)
  AND source_scope = ''
  AND external_id = sqlc.arg(external_id);

-- name: DeleteUnscopedItemChats :exec
DELETE FROM item_chat
WHERE profile_id = sqlc.arg(profile_id)
  AND source_kind = sqlc.arg(source_kind)
  AND source_scope = ''
  AND external_id = sqlc.arg(external_id);
