-- Keep-best, applied atomically by the database so concurrent submits cannot lose a better score (no row lock, no read-modify-write).
-- The WHERE mirrors leaderboard.Leaderboard.Beats for the descending (higher-is-better) order.
-- :execrows returns 1 when the score was inserted or improved, 0 when an equal-or-worse score left the row unchanged (so a resubmit is idempotent).
-- name: UpsertLeaderboardEntryDesc :execrows
INSERT INTO leaderboard_entries (id, leaderboard_id, player_id, score, achieved_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (leaderboard_id, player_id) DO UPDATE
SET score = EXCLUDED.score,
    achieved_at = EXCLUDED.achieved_at
WHERE EXCLUDED.score > leaderboard_entries.score;

-- Ascending (lower-is-better) keep-best; mirror of the descending variant.
-- name: UpsertLeaderboardEntryAsc :execrows
INSERT INTO leaderboard_entries (id, leaderboard_id, player_id, score, achieved_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (leaderboard_id, player_id) DO UPDATE
SET score = EXCLUDED.score,
    achieved_at = EXCLUDED.achieved_at
WHERE EXCLUDED.score < leaderboard_entries.score;

-- name: TopRanksDesc :many
SELECT e.player_id, p.nickname, e.score, e.achieved_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1
ORDER BY e.score DESC, e.achieved_at ASC
LIMIT $2 OFFSET $3;

-- name: TopRanksAsc :many
SELECT e.player_id, p.nickname, e.score, e.achieved_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1
ORDER BY e.score ASC, e.achieved_at ASC
LIMIT $2 OFFSET $3;

-- AllLeaderboardEntries feeds the Valkey ZSET rebuild (player_id + score for every entry on a board).
-- name: AllLeaderboardEntries :many
SELECT player_id, score
FROM leaderboard_entries
WHERE leaderboard_id = $1;

-- LeaderboardEntryByPlayer fetches one entry's score, name, and time without the rank count, for the Valkey-accelerated RankOf (rank comes from the ZSET).
-- name: LeaderboardEntryByPlayer :one
SELECT e.player_id, p.nickname, e.score, e.achieved_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1 AND e.player_id = $2;

-- The rank counts entries strictly ahead in the total (score, then earlier achieved_at) order, so it matches the paginated rank.
-- name: RankOfDesc :one
SELECT
  1 + (
    SELECT count(*) FROM leaderboard_entries o
    WHERE o.leaderboard_id = e.leaderboard_id
      AND (o.score > e.score OR (o.score = e.score AND o.achieved_at < e.achieved_at))
  ) AS rank,
  e.player_id, p.nickname, e.score, e.achieved_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1 AND e.player_id = $2;

-- name: RankOfAsc :one
SELECT
  1 + (
    SELECT count(*) FROM leaderboard_entries o
    WHERE o.leaderboard_id = e.leaderboard_id
      AND (o.score < e.score OR (o.score = e.score AND o.achieved_at < e.achieved_at))
  ) AS rank,
  e.player_id, p.nickname, e.score, e.achieved_at
FROM leaderboard_entries e
JOIN players p ON p.id = e.player_id
WHERE e.leaderboard_id = $1 AND e.player_id = $2;
