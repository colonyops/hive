-- name: AppendEvent :one
INSERT INTO event_log (topic, key, payload, created_at, snapshot, source_kind, source_scope, occurrence_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING "offset";

-- name: UpdateEventOccurrenceKey :exec
UPDATE event_log SET occurrence_key = ? WHERE "offset" = ?;

-- name: ReadEventsFrom :many
SELECT * FROM event_log
WHERE "offset" > ?
ORDER BY "offset" ASC
LIMIT ?;

-- name: GetConsumerOffset :one
SELECT * FROM consumer_offset
WHERE consumer = ?;

-- name: CommitConsumerOffset :exec
-- Monotonic upsert: on conflict, only advance the stored offset when the
-- incoming one is greater. If it isn't, the WHERE clause makes the DO UPDATE
-- a no-op (SQLite upsert semantics), so a stale/out-of-order commit never
-- regresses a consumer's checkpoint.
INSERT INTO consumer_offset (consumer, "offset")
VALUES (?, ?)
ON CONFLICT(consumer) DO UPDATE SET "offset" = excluded."offset"
WHERE excluded."offset" > consumer_offset."offset";

-- name: DeleteEventsOlderThan :exec
DELETE FROM event_log AS target
WHERE target.created_at < ?
  AND NOT (
      target.snapshot = 1
      AND target."offset" = (
          SELECT MAX(snapshot_row."offset")
          FROM event_log AS snapshot_row
          WHERE snapshot_row.topic = target.topic AND snapshot_row.snapshot = 1
      )
  );

-- name: DeleteEventsOverLimitPerTopic :exec
DELETE FROM event_log AS target
WHERE (
    SELECT COUNT(*) FROM event_log AS newer
    WHERE newer.topic = target.topic AND newer."offset" > target."offset"
) >= CAST(sqlc.arg(limit) AS INTEGER)
  AND NOT (
      target.snapshot = 1
      AND target."offset" = (
          SELECT MAX(snapshot_row."offset")
          FROM event_log AS snapshot_row
          WHERE snapshot_row.topic = target.topic AND snapshot_row.snapshot = 1
      )
  );

-- name: DeleteSnapshotsOverLimitPerTopic :exec
-- Superseded source snapshots are dead weight: replay and every snapshot read
-- take only the newest snapshot per topic, so older ones reconcile nothing.
-- Keep the newest N snapshots per topic and delete the rest. This is the bound
-- that actually caps event_log size, because a full-source snapshot dwarfs an
-- ordinary item event; the per-topic row bound above counts item events too
-- and so cannot target the snapshots that dominate. N is >= 1, so the single
-- latest snapshot the age/count bounds preserve is preserved here as well.
DELETE FROM event_log AS target
WHERE target.snapshot = 1
  AND (
      SELECT COUNT(*) FROM event_log AS newer
      WHERE newer.topic = target.topic
        AND newer.snapshot = 1
        AND newer."offset" > target."offset"
  ) >= CAST(sqlc.arg(limit) AS INTEGER);

-- name: ListLatestSourceSnapshotsByTopicPrefix :many
SELECT event_log.*
FROM event_log
JOIN (
    SELECT topic, MAX("offset") AS "offset"
    FROM event_log
    WHERE snapshot = 1
      AND "offset" <= CAST(sqlc.arg(through_offset) AS INTEGER)
      AND substr(topic, 1, length(CAST(sqlc.arg(topic_prefix) AS TEXT))) = CAST(sqlc.arg(topic_prefix) AS TEXT)
    GROUP BY topic
) AS latest ON latest.topic = event_log.topic AND latest."offset" = event_log."offset"
ORDER BY event_log.topic;

-- name: DeleteEventLogByTopicPrefix :exec
DELETE FROM event_log WHERE topic LIKE ? ESCAPE '\';

-- name: DeleteConsumerOffsetByConsumer :exec
DELETE FROM consumer_offset WHERE consumer = ?;
