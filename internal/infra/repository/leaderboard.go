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

func (Leaderboard) GetEntry(ctx context.Context, leaderboardID string, playerID uuid.UUID) (*leaderboard.Entry, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).GetLeaderboardEntry(ctx, dbgen.GetLeaderboardEntryParams{LeaderboardID: leaderboardID, PlayerID: playerID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, leaderboard.ErrEntryNotFound
		}
		return nil, err
	}
	return &leaderboard.Entry{PlayerID: row.PlayerID, Score: row.Score, UpdatedAt: row.UpdatedAt.Time}, nil
}

func (Leaderboard) SaveEntry(ctx context.Context, leaderboardID string, playerID uuid.UUID, score int64, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).UpsertLeaderboardEntry(ctx, dbgen.UpsertLeaderboardEntryParams{
		ID:            uuid.New(),
		LeaderboardID: leaderboardID,
		PlayerID:      playerID,
		Score:         score,
		CreatedAt:     toTimestamptz(now),
	})
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
