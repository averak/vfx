-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (id, player_id, token_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: FindRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > $2;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = $2
WHERE id = $1;

-- name: RevokeAllRefreshTokensForPlayer :exec
UPDATE refresh_tokens
SET revoked_at = $2
WHERE player_id = $1
  AND revoked_at IS NULL;

-- name: DeleteRefreshTokensExpiredBefore :exec
DELETE FROM refresh_tokens
WHERE expires_at < $1;
