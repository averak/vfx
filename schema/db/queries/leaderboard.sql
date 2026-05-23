-- Keep-best, applied atomically by the database so concurrent submits cannot lose a better score (no row lock, no read-modify-write).
-- The WHERE mirrors leaderboard.Leaderboard.Beats for the descending (higher-is-better) order.
-- :execrows returns 1 when the score was inserted or improved, 0 when an equal-or-worse score left the row unchanged (so a resubmit is idempotent).
-- name: UpsertLeaderboardEntryDesc :execrows
INSERT INTO leaderboard_entries (id, leaderboard_id, player_id, score, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (leaderboard_id, player_id) DO UPDATE
SET score = EXCLUDED.score,
    updated_at = EXCLUDED.updated_at
WHERE EXCLUDED.score > leaderboard_entries.score;

-- Ascending (lower-is-better) keep-best; mirror of the descending variant.
-- name: UpsertLeaderboardEntryAsc :execrows
INSERT INTO leaderboard_entries (id, leaderboard_id, player_id, score, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $5)
ON CONFLICT (leaderboard_id, player_id) DO UPDATE
SET score = EXCLUDED.score,
    updated_at = EXCLUDED.updated_at
WHERE EXCLUDED.score < leaderboard_entries.score;

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
