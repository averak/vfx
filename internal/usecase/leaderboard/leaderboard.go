// Package leaderboard orchestrates the LeaderboardService.
//
// Leaderboards are defined by deployment config (id + sort order), so the usecase rejects unknown ids; scores are kept best-first per player.
package leaderboard

import (
	"context"

	"github.com/google/uuid"

	domainleaderboard "github.com/averak/vfx/internal/domain/leaderboard"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

type Config struct {
	// DefaultLimit applies when a ListRanks request omits the limit; MaxLimit caps it; MaxRadius caps the around-player window.
	DefaultLimit int
	MaxLimit     int
	MaxRadius    int
}

type Usecase struct {
	rw   tx.ReadWriter
	ro   tx.Reader
	repo domainleaderboard.Repository
	defs map[string]domainleaderboard.Leaderboard
	cfg  Config
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domainleaderboard.Repository, defs map[string]domainleaderboard.Leaderboard, cfg Config) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo, defs: defs, cfg: cfg}
}

func (u *Usecase) lookup(id string) (domainleaderboard.Leaderboard, error) {
	lb, ok := u.defs[id]
	if !ok {
		return domainleaderboard.Leaderboard{}, domainleaderboard.ErrLeaderboardNotFound
	}
	return lb, nil
}

// SubmitScore records score under the keep-best rule and returns the player's resulting ranked entry.
// improved is false when the score did not beat the player's current best (the existing entry is returned unchanged).
func (u *Usecase) SubmitScore(ctx context.Context, leaderboardID string, playerID uuid.UUID, score int64) (*domainleaderboard.RankedEntry, bool, error) {
	lb, err := u.lookup(leaderboardID)
	if err != nil {
		return nil, false, err
	}
	now := clock.Now(ctx)

	var improved bool
	err = u.rw.RW(ctx, func(ctx context.Context) error {
		var submitErr error
		improved, submitErr = u.repo.Submit(ctx, lb, playerID, score, now)
		return submitErr
	})
	if err != nil {
		return nil, false, err
	}

	// The rank is read after the write commits; the keep-best itself is applied atomically by Submit, so concurrent submits cannot lose a better score.
	ranked, err := u.rankOf(ctx, lb, playerID)
	if err != nil {
		return nil, false, err
	}
	return ranked, improved, nil
}

func (u *Usecase) ListRanks(ctx context.Context, leaderboardID string, offset, limit int) ([]*domainleaderboard.RankedEntry, error) {
	lb, err := u.lookup(leaderboardID)
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		offset = 0
	}
	limit = u.clampLimit(limit)

	var entries []*domainleaderboard.RankedEntry
	err = u.ro.RO(ctx, func(ctx context.Context) error {
		var listErr error
		entries, listErr = u.repo.TopRanks(ctx, lb, offset, limit)
		return listErr
	})
	return entries, err
}

func (u *Usecase) GetPlayerRank(ctx context.Context, leaderboardID string, playerID uuid.UUID) (*domainleaderboard.RankedEntry, error) {
	lb, err := u.lookup(leaderboardID)
	if err != nil {
		return nil, err
	}
	return u.rankOf(ctx, lb, playerID)
}

// ListRanksAroundPlayer returns the entries within radius positions of the player on either side.
func (u *Usecase) ListRanksAroundPlayer(ctx context.Context, leaderboardID string, playerID uuid.UUID, radius int) ([]*domainleaderboard.RankedEntry, error) {
	lb, err := u.lookup(leaderboardID)
	if err != nil {
		return nil, err
	}
	if radius < 0 {
		radius = 0
	}
	if radius > u.cfg.MaxRadius {
		radius = u.cfg.MaxRadius
	}

	var entries []*domainleaderboard.RankedEntry
	err = u.ro.RO(ctx, func(ctx context.Context) error {
		self, rankErr := u.repo.RankOf(ctx, lb, playerID)
		if rankErr != nil {
			return rankErr
		}
		offset := int(self.Rank) - 1 - radius
		if offset < 0 {
			offset = 0
		}
		var topErr error
		entries, topErr = u.repo.TopRanks(ctx, lb, offset, 2*radius+1)
		return topErr
	})
	return entries, err
}

func (u *Usecase) rankOf(ctx context.Context, lb domainleaderboard.Leaderboard, playerID uuid.UUID) (*domainleaderboard.RankedEntry, error) {
	var ranked *domainleaderboard.RankedEntry
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		ranked, err = u.repo.RankOf(ctx, lb, playerID)
		return err
	})
	return ranked, err
}

func (u *Usecase) clampLimit(limit int) int {
	if limit <= 0 {
		return u.cfg.DefaultLimit
	}
	if limit > u.cfg.MaxLimit {
		return u.cfg.MaxLimit
	}
	return limit
}
