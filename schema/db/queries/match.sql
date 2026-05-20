-- name: CreateMatch :one
INSERT INTO matches (id, game_mode, status, created_at, updated_at)
VALUES ($1, $2, 'pending', $3, $4)
RETURNING *;

-- name: GetMatchByID :one
SELECT * FROM matches
WHERE id = $1;

-- name: MarkMatchInProgress :one
UPDATE matches
SET status = 'in_progress',
    started_at = $2,
    updated_at = $3
WHERE id = $1
  AND status = 'pending'
RETURNING *;

-- name: FinishMatch :one
UPDATE matches
SET status = 'finished',
    ended_at = $2,
    final_state = $3,
    updated_at = $4
WHERE id = $1
RETURNING *;

-- name: CancelMatch :one
UPDATE matches
SET status = 'cancelled',
    ended_at = $2,
    updated_at = $3
WHERE id = $1
RETURNING *;

-- name: AddMatchPlayer :one
INSERT INTO match_players (id, match_id, player_id, created_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: SetMatchPlayerResult :one
UPDATE match_players
SET rank = $3,
    stats = $4
WHERE match_id = $1
  AND player_id = $2
RETURNING *;

-- name: ListMatchPlayers :many
SELECT * FROM match_players
WHERE match_id = $1
ORDER BY created_at ASC;

-- name: FindActiveMatchForPlayer :one
SELECT m.*
FROM matches m
JOIN match_players mp ON mp.match_id = m.id
WHERE mp.player_id = $1
  AND m.status IN ('pending', 'in_progress')
ORDER BY m.created_at DESC
LIMIT 1;
