-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (id, player_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: FindRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND expires_at > $2;

-- Conditional on revoked_at IS NULL so concurrent refreshes of the same token serialize on the row: the second UPDATE re-checks the predicate after the first commits, matches no row, and the caller treats that as invalid.
-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at = $2
WHERE id = $1
  AND revoked_at IS NULL;

-- name: RevokeAllRefreshTokensForPlayer :exec
UPDATE refresh_tokens
SET revoked_at = $2
WHERE player_id = $1
  AND revoked_at IS NULL;

-- name: DeleteRefreshTokensExpiredBefore :exec
DELETE FROM refresh_tokens
WHERE expires_at < $1;
