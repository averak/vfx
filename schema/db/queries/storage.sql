-- name: GetPlayerFile :one
SELECT * FROM player_files
WHERE owner_id = $1 AND filename = $2;

-- starts_with(filename, '') is true for every row, so an empty prefix lists all of the owner's files.
-- name: ListPlayerFiles :many
SELECT * FROM player_files
WHERE owner_id = sqlc.arg(owner_id) AND starts_with(filename, sqlc.arg(prefix))
ORDER BY filename ASC;

-- name: UpsertPlayerFile :one
INSERT INTO player_files (id, owner_id, filename, size, hash, modified_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (owner_id, filename) DO UPDATE
SET size = EXCLUDED.size,
    hash = EXCLUDED.hash,
    modified_at = EXCLUDED.modified_at
RETURNING *;

-- name: DeletePlayerFile :exec
DELETE FROM player_files
WHERE owner_id = $1 AND filename = $2;

-- name: PlayerStorageUsage :one
SELECT COALESCE(SUM(size), 0)::BIGINT AS total_size, COUNT(*) AS file_count
FROM player_files
WHERE owner_id = $1;

-- tags @> '{}' holds for every row, so an empty tag set lists all title files.
-- name: ListTitleFiles :many
SELECT * FROM title_files
WHERE tags @> $1
ORDER BY filename ASC;

-- name: GetTitleFile :one
SELECT * FROM title_files
WHERE filename = $1;

-- name: UpsertTitleFile :one
INSERT INTO title_files (id, filename, size, hash, tags, modified_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (filename) DO UPDATE
SET size = EXCLUDED.size,
    hash = EXCLUDED.hash,
    tags = EXCLUDED.tags,
    modified_at = EXCLUDED.modified_at
RETURNING *;

-- name: DeleteTitleFile :exec
DELETE FROM title_files
WHERE filename = $1;
