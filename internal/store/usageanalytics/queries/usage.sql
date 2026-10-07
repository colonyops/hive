-- name: InitializeMetadata :exec
INSERT INTO usage_metadata (id, installation_id) VALUES (1, ?) ON CONFLICT DO NOTHING;

-- name: Metadata :one
SELECT installation_id, clear_cutoff_ns FROM usage_metadata WHERE id = 1;

-- name: InsertEvent :exec
INSERT INTO usage_event (event_id, occurred_at_ms, recorded_at_ms, name, schema_version, installation_id, run_id, surface, app_version, release_channel, properties_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING;

-- name: Counts :many
SELECT name, count(*) AS total FROM usage_event WHERE occurred_at_ms >= ? AND occurred_at_ms <= ? GROUP BY name;

-- name: ClearEvents :exec
DELETE FROM usage_event;

-- name: SetClearCutoff :exec
UPDATE usage_metadata SET clear_cutoff_ns = max(clear_cutoff_ns, ?), installation_id = ? WHERE id = 1;

-- name: Prune :execrows
DELETE FROM usage_event WHERE event_id IN (SELECT old.event_id FROM usage_event AS old WHERE old.occurred_at_ms < ? LIMIT 500);
