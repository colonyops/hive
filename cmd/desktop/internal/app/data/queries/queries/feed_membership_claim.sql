-- name: UpsertFeedMembershipClaim :exec
INSERT INTO feed_membership_claim (profile_id, feed_id, item_id, source_id)
VALUES (?, ?, ?, ?)
ON CONFLICT (profile_id, feed_id, item_id, source_id) DO NOTHING;

-- name: DeleteFeedMembershipClaimsNotInSnapshot :exec
DELETE FROM feed_membership_claim
WHERE feed_id = ? AND source_id = ? AND item_id NOT IN (sqlc.slice(item_ids));

-- name: DeleteFeedMembershipClaimsForSourceAll :exec
DELETE FROM feed_membership_claim WHERE feed_id = ? AND source_id = ?;

-- name: DeleteFeedMembershipClaimsForFeeds :exec
DELETE FROM feed_membership_claim
WHERE feed_membership_claim.profile_id = ? AND feed_id NOT IN (sqlc.slice(feed_ids));

-- name: DeleteFeedMembershipClaimsForFeedsAll :exec
DELETE FROM feed_membership_claim WHERE profile_id = ?;

-- name: DeleteFeedMembershipClaimsForRemovedSources :exec
DELETE FROM feed_membership_claim
WHERE feed_membership_claim.profile_id = ?
  AND source_id NOT IN (sqlc.slice(source_ids))
  AND item_id IN (SELECT id FROM inbox_item WHERE archived_at IS NULL);

-- name: DeleteFeedMembershipClaimsForRemovedSourcesAll :exec
DELETE FROM feed_membership_claim
WHERE feed_membership_claim.profile_id = ?
  AND item_id IN (SELECT id FROM inbox_item WHERE archived_at IS NULL);

-- name: DeleteUnarchivedFeedMembershipClaimsByProfile :exec
DELETE FROM feed_membership_claim
WHERE feed_membership_claim.profile_id = ? AND item_id IN (SELECT id FROM inbox_item WHERE archived_at IS NULL);

-- name: GetFeedIDForItem :one
-- The lowest feed id is the stable answer when several feeds claim the same
-- item: any of them reveals it, so this just has to agree with itself across
-- calls. Ordering (not the sidebar's own) is a frontend concern this leaves
-- alone.
SELECT feed_id FROM feed_membership_claim
WHERE profile_id = ? AND item_id = ?
ORDER BY feed_id
LIMIT 1;

-- name: ListFeedIDsForItems :many
-- One row per (item, its lowest feed id), for callers that list items flat
-- and need every item's feed without an N+1 of GetFeedIDForItem.
SELECT item_id, CAST(MIN(feed_id) AS TEXT) AS feed_id FROM feed_membership_claim
WHERE item_id IN (sqlc.slice(item_ids))
GROUP BY item_id;
