-- Diff reviews

-- name: DeleteDiffReviewByAuthor :exec
DELETE FROM diff_reviews WHERE session_id = ? AND author = ?;

-- name: InsertDiffReview :exec
INSERT INTO diff_reviews (id, session_id, author, agent, head, summary, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: InsertDiffReviewComment :exec
INSERT INTO diff_review_comments (
    id, review_id, position, kind, path, start_side, start_line, end_side, end_line, quote, text, severity
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListDiffReviews :many
SELECT * FROM diff_reviews WHERE session_id = ? ORDER BY created_at ASC, author ASC;

-- name: ListDiffReviewComments :many
SELECT c.* FROM diff_review_comments c
JOIN diff_reviews r ON r.id = c.review_id
WHERE r.session_id = ?
ORDER BY c.review_id, c.position;

-- name: GetDiffReviewComment :one
SELECT * FROM diff_review_comments WHERE id = ?;

-- name: DeleteDiffReviewComment :execrows
DELETE FROM diff_review_comments WHERE id = ?;

-- name: CountDiffReviewComments :one
SELECT COUNT(*) FROM diff_review_comments WHERE review_id = ?;

-- name: DeleteDiffReview :execrows
DELETE FROM diff_reviews WHERE id = ?;

-- name: DeleteDiffReviewsBySession :execrows
DELETE FROM diff_reviews WHERE session_id = ?;

-- name: DiffReviewFingerprint :one
SELECT
    (SELECT COUNT(*) FROM diff_reviews) AS reviews,
    (SELECT COUNT(*) FROM diff_review_comments) AS comments,
    CAST((SELECT COALESCE(MAX(created_at), 0) FROM diff_reviews) AS INTEGER) AS latest;

-- name: GetDiffReview :one
SELECT * FROM diff_reviews WHERE id = ?;
