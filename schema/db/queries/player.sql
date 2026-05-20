-- name: GetPlayerByID :one
SELECT * FROM players
WHERE id = $1;

-- name: CreatePlayer :one
INSERT INTO players (id, nickname, created_at, updated_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdatePlayerNickname :one
UPDATE players
SET nickname = $2,
    updated_at = $3
WHERE id = $1
RETURNING *;

-- name: FindPlayerByIdentity :one
SELECT p.*
FROM players p
JOIN player_identities pi ON pi.player_id = p.id
WHERE pi.provider = $1
  AND pi.provider_uid = $2;

-- name: CreatePlayerIdentity :one
INSERT INTO player_identities (id, player_id, provider, provider_uid, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPlayerIdentities :many
SELECT * FROM player_identities
WHERE player_id = $1
ORDER BY created_at ASC;
