-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (id, player_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: FindRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > NOW();

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE id = $1;

-- name: RevokeAllRefreshTokensForPlayer :exec
UPDATE refresh_tokens
SET revoked_at = NOW()
WHERE player_id = $1
  AND revoked_at IS NULL;

-- name: DeleteExpiredRefreshTokens :exec
DELETE FROM refresh_tokens
WHERE expires_at < NOW() - INTERVAL '7 days';
