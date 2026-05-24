package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/domain/leaderboard"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// LeaderboardIndex accelerates ranking with a Valkey sorted set while PostgreSQL stays the durable source of truth.
//
// The expensive query is RankOf: in Postgres it counts every better score (O(rank)) and runs on every submit, so at scale it is the bottleneck.
// A ZSET answers it with ZREVRANK in O(log N). The ZSET is a rebuildable cache: a per-board TTL bounds how stale a missed write can leave it (the next access reloads from Postgres), and any Valkey error falls back to the Postgres path, so a Valkey outage only costs performance.
// Scores are int64 but a ZSET score is float64; game scores stay well within float64's exact-integer range.
type LeaderboardIndex struct {
	pg     *Leaderboard
	client valkeygo.Client
	ttl    time.Duration
}

var _ leaderboard.Repository = (*LeaderboardIndex)(nil)

func NewLeaderboardIndex(client valkeygo.Client, ttl time.Duration) *LeaderboardIndex {
	return &LeaderboardIndex{pg: NewLeaderboard(), client: client, ttl: ttl}
}

func leaderboardKey(id string) string { return "vfx:lb:" + id }

func (s *LeaderboardIndex) Submit(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID, score int64, now time.Time) (bool, error) {
	// Postgres is authoritative for keep-best, durability, and the returned improved flag.
	improved, err := s.pg.Submit(ctx, lb, playerID, score, now)
	if err != nil {
		return false, err
	}
	// Mirror into the ZSET best-effort. ensureBuilt first so a write to an expired key cannot create a partial index; a remaining failure self-heals on the next TTL rebuild.
	if s.ensureBuilt(ctx, lb) == nil {
		//nolint:errcheck // Best-effort index update; Postgres holds the truth and the TTL rebuild reconciles.
		_ = s.zadd(ctx, lb, playerID, score)
	}
	return improved, nil
}

func (s *LeaderboardIndex) RankOf(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID) (*leaderboard.RankedEntry, error) {
	entry, err := s.rankViaIndex(ctx, lb, playerID)
	switch {
	case err == nil:
		return entry, nil
	case errors.Is(err, leaderboard.ErrEntryNotFound):
		return nil, err
	default:
		// Valkey is unavailable or misbehaving: serve correctness from Postgres.
		return s.pg.RankOf(ctx, lb, playerID)
	}
}

// TopRanks stays on Postgres: an ORDER BY score LIMIT over the (leaderboard_id, score, achieved_at) index is already efficient for the shallow pages clients request.
func (s *LeaderboardIndex) TopRanks(ctx context.Context, lb leaderboard.Leaderboard, offset, limit int) ([]*leaderboard.RankedEntry, error) {
	return s.pg.TopRanks(ctx, lb, offset, limit)
}

func (s *LeaderboardIndex) rankViaIndex(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID) (*leaderboard.RankedEntry, error) {
	if err := s.ensureBuilt(ctx, lb); err != nil {
		return nil, err
	}

	rank, err := s.zeroBasedRank(ctx, lb, playerID)
	if err != nil {
		return nil, err
	}

	// Rank comes from the ZSET; score, name, and time come from a single indexed Postgres row (no count).
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).LeaderboardEntryByPlayer(ctx, dbgen.LeaderboardEntryByPlayerParams{LeaderboardID: lb.ID, PlayerID: playerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, leaderboard.ErrEntryNotFound
		}
		return nil, err
	}
	return &leaderboard.RankedEntry{
		Rank:       rank + 1,
		PlayerID:   row.PlayerID,
		Nickname:   row.Nickname,
		Score:      row.Score,
		AchievedAt: row.AchievedAt.Time,
	}, nil
}

// zeroBasedRank returns the player's 0-based rank from the ZSET, or ErrEntryNotFound when they are not ranked.
func (s *LeaderboardIndex) zeroBasedRank(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID) (int64, error) {
	member := playerID.String()
	key := leaderboardKey(lb.ID)
	var cmd valkeygo.Completed
	if lb.SortOrder == leaderboard.Ascending {
		cmd = s.client.B().Zrank().Key(key).Member(member).Build()
	} else {
		cmd = s.client.B().Zrevrank().Key(key).Member(member).Build()
	}
	rank, err := s.client.Do(ctx, cmd).ToInt64()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return 0, leaderboard.ErrEntryNotFound
		}
		return 0, err
	}
	return rank, nil
}

func (s *LeaderboardIndex) ensureBuilt(ctx context.Context, lb leaderboard.Leaderboard) error {
	exists, err := s.client.Do(ctx, s.client.B().Exists().Key(leaderboardKey(lb.ID)).Build()).ToInt64()
	if err != nil {
		return err
	}
	if exists == 1 {
		return nil
	}
	return s.rebuild(ctx, lb)
}

// rebuild loads every entry for the board from Postgres into the ZSET and sets the TTL.
// Concurrent rebuilds are harmless: ZADD is idempotent, so no lock is taken.
func (s *LeaderboardIndex) rebuild(ctx context.Context, lb leaderboard.Leaderboard) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	rows, err := dbgen.New(tx).AllLeaderboardEntries(ctx, lb.ID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		// An empty board indexes to nothing; the key stays absent and reads fall through cheaply until a score is submitted.
		return nil
	}

	add := s.client.B().Zadd().Key(leaderboardKey(lb.ID)).ScoreMember()
	for _, r := range rows {
		add = add.ScoreMember(float64(r.Score), r.PlayerID.String())
	}
	if err := s.client.Do(ctx, add.Build()).Error(); err != nil {
		return err
	}
	return s.client.Do(ctx, s.client.B().Expire().Key(leaderboardKey(lb.ID)).Seconds(int64(s.ttl.Seconds())).Build()).Error()
}

func (s *LeaderboardIndex) zadd(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID, score int64) error {
	key := leaderboardKey(lb.ID)
	member := playerID.String()
	f := float64(score)
	// GT/LT keep the better score atomically, mirroring the Postgres conditional upsert.
	if lb.SortOrder == leaderboard.Ascending {
		return s.client.Do(ctx, s.client.B().Zadd().Key(key).Lt().ScoreMember().ScoreMember(f, member).Build()).Error()
	}
	return s.client.Do(ctx, s.client.B().Zadd().Key(key).Gt().ScoreMember().ScoreMember(f, member).Build()).Error()
}
