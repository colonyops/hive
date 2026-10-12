-- name: InsertDelivery :exec
INSERT INTO deliveries (id, source_id, received_at, enc, size, record)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListDeliveriesAfter :many
SELECT id, source_id, received_at, enc, size, record FROM deliveries
WHERE source_id = sqlc.arg(source_id) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(max_rows);

-- name: DeleteDeliveriesBefore :many
DELETE FROM deliveries
WHERE source_id = sqlc.arg(source_id) AND received_at < sqlc.arg(before)
RETURNING size;

-- name: DeleteDeliveriesBeyondNewest :many
DELETE FROM deliveries
WHERE id IN (
  SELECT d.id FROM deliveries d
  WHERE d.source_id = sqlc.arg(source_id)
  ORDER BY d.id DESC
  LIMIT -1 OFFSET sqlc.arg(keep)
)
RETURNING size;

-- name: SourceDeliveryStats :one
SELECT COUNT(*) AS count, CAST(COALESCE(SUM(size), 0) AS INTEGER) AS bytes
FROM deliveries WHERE source_id = ?;
