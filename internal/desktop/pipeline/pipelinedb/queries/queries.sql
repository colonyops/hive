-- name: AppendEvent :one
INSERT INTO event_log (topic, key, payload, created_at)
VALUES (?, ?, ?, ?)
RETURNING "offset";

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

-- name: CompactEventLogByKey :exec
-- Log-compaction pass: for every non-empty key, keep only the row at its
-- highest offset (the current value). Rows with an empty key (system events
-- with no stable identity) are exempt from key-compaction.
DELETE FROM event_log
WHERE key != ''
  AND "offset" NOT IN (
    SELECT MAX("offset") FROM event_log WHERE key != '' GROUP BY key
  );

-- name: DeleteEventLogOlderThan :exec
DELETE FROM event_log WHERE created_at < ?;

-- name: CountEventLog :one
SELECT COUNT(*) FROM event_log;

-- name: DeleteOldestEventLog :exec
-- Deletes the oldest ? rows by offset. Used for count-based retention once
-- age-based retention still leaves the table over its row cap.
DELETE FROM event_log
WHERE "offset" IN (
    SELECT "offset" FROM event_log ORDER BY "offset" ASC LIMIT ?
);
