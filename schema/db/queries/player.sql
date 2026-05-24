-- name: GetPlayerByID :one
SELECT * FROM players
WHERE id = $1;

-- Player is the aggregate root, so it is persisted whole: one upsert serves both creation and profile updates, rather than per-field update queries.
-- name: UpsertPlayer :one
INSERT INTO players (id, nickname, created_at, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (id) DO UPDATE
SET nickname = EXCLUDED.nickname,
    updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: FindIdentity :one
SELECT * FROM player_identities
WHERE provider = $1
  AND provider_uid = $2;

-- name: CreatePlayerIdentity :one
INSERT INTO player_identities (id, player_id, provider, provider_uid, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListPlayerIdentities :many
SELECT * FROM player_identities
WHERE player_id = $1
ORDER BY created_at ASC;
