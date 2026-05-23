-- name: GetLeaderboardEntry :one
SELECT * FROM leaderboard_entries
WHERE leaderboard_id = $1 AND player_id = $2;

-- name: UpsertLeaderboardEntry :exec
INSERT INTO leaderboard_entries (id, leaderboard_id, player_id, score, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (leaderboard_id, player_id) DO UPDATE
SET score = EXCLUDED.score,
    updated_at = EXCLUDED.updated_at;

-- name: TopRanksDesc :many
SELECT e.player_id, p.nickname, e.score, e.updated_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1
ORDER BY e.score DESC, e.updated_at ASC
LIMIT $2 OFFSET $3;

-- name: TopRanksAsc :many
SELECT e.player_id, p.nickname, e.score, e.updated_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1
ORDER BY e.score ASC, e.updated_at ASC
LIMIT $2 OFFSET $3;

-- The rank counts entries strictly ahead in the total (score, then earlier updated_at) order, so it matches the paginated rank.
-- name: RankOfDesc :one
SELECT
  1 + (
    SELECT count(*) FROM leaderboard_entries o
    WHERE o.leaderboard_id = e.leaderboard_id
      AND (o.score > e.score OR (o.score = e.score AND o.updated_at < e.updated_at))
  ) AS rank,
  e.player_id, p.nickname, e.score, e.updated_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1 AND e.player_id = $2;

-- name: RankOfAsc :one
SELECT
  1 + (
    SELECT count(*) FROM leaderboard_entries o
    WHERE o.leaderboard_id = e.leaderboard_id
      AND (o.score < e.score OR (o.score = e.score AND o.updated_at < e.updated_at))
  ) AS rank,
  e.player_id, p.nickname, e.score, e.updated_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1 AND e.player_id = $2;
