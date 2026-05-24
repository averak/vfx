-- name: GetPlayerByID :one
SELECT * FROM players
WHERE id = $1;

-- Player is the aggregate root, so it is persisted whole: one upsert serves both creation and profile updates, rather than per-field update queries.
-- registered_at is set once at creation (domain data); updated_at is row audit, refreshed by the database on every write.
-- name: UpsertPlayer :one
INSERT INTO players (id, nickname, registered_at)
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE
SET nickname = EXCLUDED.nickname,
    updated_at = NOW()
RETURNING *;

-- name: FindIdentity :one
SELECT * FROM player_identities
WHERE provider = $1
  AND provider_uid = $2;

-- name: CreatePlayerIdentity :one
INSERT INTO player_identities (id, player_id, provider, provider_uid)
VALUES ($1, $2, $3, $4)
RETURNING *;
