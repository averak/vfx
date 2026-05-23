package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/leaderboard"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// Leaderboard is the storage implementation of [leaderboard.Repository].
type Leaderboard struct{}

var _ leaderboard.Repository = (*Leaderboard)(nil)

func NewLeaderboard() *Leaderboard {
	return &Leaderboard{}
}

func (Leaderboard) Submit(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID, score int64, now time.Time) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	q := dbgen.New(tx)
	// A fresh id is used only when the row is new; ON CONFLICT keeps the existing row's id and created_at.
	// affected is 1 on insert or improvement, 0 when the conditional WHERE left an equal-or-worse score in place.
	if lb.SortOrder == leaderboard.Ascending {
		affected, ascErr := q.UpsertLeaderboardEntryAsc(ctx, dbgen.UpsertLeaderboardEntryAscParams{
			ID:            uuid.New(),
			LeaderboardID: lb.ID,
			PlayerID:      playerID,
			Score:         score,
			CreatedAt:     toTimestamptz(now),
		})
		return affected > 0, ascErr
	}
	affected, err := q.UpsertLeaderboardEntryDesc(ctx, dbgen.UpsertLeaderboardEntryDescParams{
		ID:            uuid.New(),
		LeaderboardID: lb.ID,
		PlayerID:      playerID,
		Score:         score,
		CreatedAt:     toTimestamptz(now),
	})
	return affected > 0, err
}

func (Leaderboard) RankOf(ctx context.Context, lb leaderboard.Leaderboard, playerID uuid.UUID) (*leaderboard.RankedEntry, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	q := dbgen.New(tx)
	params := dbgen.RankOfDescParams{LeaderboardID: lb.ID, PlayerID: playerID}
	if lb.SortOrder == leaderboard.Ascending {
		row, ascErr := q.RankOfAsc(ctx, dbgen.RankOfAscParams(params))
		if ascErr != nil {
			return nil, rankErr(ascErr)
		}
		return &leaderboard.RankedEntry{Rank: int64(row.Rank), PlayerID: row.PlayerID, Nickname: row.Nickname, Score: row.Score, UpdatedAt: row.UpdatedAt.Time}, nil
	}
	row, err := q.RankOfDesc(ctx, params)
	if err != nil {
		return nil, rankErr(err)
	}
	return &leaderboard.RankedEntry{Rank: int64(row.Rank), PlayerID: row.PlayerID, Nickname: row.Nickname, Score: row.Score, UpdatedAt: row.UpdatedAt.Time}, nil
}

func (Leaderboard) TopRanks(ctx context.Context, lb leaderboard.Leaderboard, offset, limit int) ([]*leaderboard.RankedEntry, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	q := dbgen.New(tx)
	//nolint:gosec // offset/limit are small, server-clamped pagination bounds.
	params := dbgen.TopRanksDescParams{LeaderboardID: lb.ID, Limit: int32(limit), Offset: int32(offset)}

	out := make([]*leaderboard.RankedEntry, 0, limit)
	if lb.SortOrder == leaderboard.Ascending {
		rows, ascErr := q.TopRanksAsc(ctx, dbgen.TopRanksAscParams(params))
		if ascErr != nil {
			return nil, ascErr
		}
		for i, row := range rows {
			out = append(out, &leaderboard.RankedEntry{Rank: int64(offset + i + 1), PlayerID: row.PlayerID, Nickname: row.Nickname, Score: row.Score, UpdatedAt: row.UpdatedAt.Time})
		}
		return out, nil
	}
	rows, err := q.TopRanksDesc(ctx, params)
	if err != nil {
		return nil, err
	}
	for i, row := range rows {
		out = append(out, &leaderboard.RankedEntry{Rank: int64(offset + i + 1), PlayerID: row.PlayerID, Nickname: row.Nickname, Score: row.Score, UpdatedAt: row.UpdatedAt.Time})
	}
	return out, nil
}

func rankErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return leaderboard.ErrEntryNotFound
	}
	return err
}
