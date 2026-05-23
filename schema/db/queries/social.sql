-- name: CreateFriendRequest :exec
INSERT INTO friend_requests (id, requester_id, addressee_id, created_at)
VALUES ($1, $2, $3, $4);

-- name: FriendRequestExists :one
SELECT EXISTS (
  SELECT 1 FROM friend_requests
  WHERE requester_id = $1 AND addressee_id = $2
);

-- name: DeleteFriendRequest :execrows
DELETE FROM friend_requests
WHERE requester_id = $1 AND addressee_id = $2;

-- name: ListIncomingRequests :many
SELECT p.id, p.nickname, fr.created_at
FROM friend_requests fr
JOIN players p ON p.id = fr.requester_id
WHERE fr.addressee_id = $1
ORDER BY fr.created_at DESC;

-- name: ListOutgoingRequests :many
SELECT p.id, p.nickname, fr.created_at
FROM friend_requests fr
JOIN players p ON p.id = fr.addressee_id
WHERE fr.requester_id = $1
ORDER BY fr.created_at DESC;

-- name: CreateFriendship :exec
INSERT INTO friendships (id, player_low, player_high, created_at)
VALUES ($1, $2, $3, $4);

-- name: FriendshipExists :one
SELECT EXISTS (
  SELECT 1 FROM friendships
  WHERE player_low = $1 AND player_high = $2
);

-- name: DeleteFriendship :execrows
DELETE FROM friendships
WHERE player_low = $1 AND player_high = $2;

-- The friend is whichever side of the pair is not the caller.
-- name: ListFriends :many
SELECT p.id, p.nickname, f.created_at
FROM friendships f
JOIN players p ON p.id = CASE WHEN f.player_low = $1 THEN f.player_high ELSE f.player_low END
WHERE f.player_low = $1 OR f.player_high = $1
ORDER BY f.created_at DESC;
