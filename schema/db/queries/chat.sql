-- name: InsertDirectMessage :exec
INSERT INTO direct_messages (id, player_low, player_high, sender_id, body, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- before is exclusive, so paging passes the oldest seen created_at to fetch the next older page; the repository passes a far-future time for the first page.
-- name: ListConversation :many
SELECT id, player_low, player_high, sender_id, body, created_at
FROM direct_messages
WHERE player_low = $1 AND player_high = $2 AND created_at < $3
ORDER BY created_at DESC
LIMIT $4;
