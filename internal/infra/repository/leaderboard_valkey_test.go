package repository_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/leaderboard"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/valkey"
	"github.com/averak/vfx/internal/testutils/testdb"
)

// These tests exercise the Valkey-accelerated leaderboard against a real Valkey (the compose stack provides it); they skip when VALKEY_URL is unset.
func newLeaderboardIndex(t *testing.T) (*repository.LeaderboardIndex, *db.Session) {
	t.Helper()
	url := os.Getenv("VALKEY_URL")
	if url == "" {
		t.Skip("VALKEY_URL not set; skipping Valkey leaderboard test")
	}
	client, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	t.Cleanup(client.Close)
	return repository.NewLeaderboardIndex(client, time.Hour), db.NewSession(testdb.Pool(t))
}

func seedScore(t *testing.T, s *db.Session, idx *repository.LeaderboardIndex, lb leaderboard.Leaderboard, score int64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustRW(t, s, func(ctx context.Context) error {
		p, err := player.New(id, nil, time.Now().UTC())
		if err != nil {
			return err
		}
		if saveErr := repository.NewPlayer().Save(ctx, p); saveErr != nil {
			return saveErr
		}
		_, err = idx.Submit(ctx, lb, id, score, time.Now().UTC())
		return err
	})
	return id
}

func rankOf(t *testing.T, s *db.Session, idx *repository.LeaderboardIndex, lb leaderboard.Leaderboard, playerID uuid.UUID) int64 {
	t.Helper()
	var rank int64
	mustRW(t, s, func(ctx context.Context) error {
		e, err := idx.RankOf(ctx, lb, playerID)
		if err != nil {
			return err
		}
		rank = e.Rank
		return nil
	})
	return rank
}

func TestLeaderboardIndex_RanksDescending(t *testing.T) {
	idx, s := newLeaderboardIndex(t)
	lb := leaderboard.Leaderboard{ID: "test-desc-" + uuid.NewString(), SortOrder: leaderboard.Descending}

	high := seedScore(t, s, idx, lb, 300)
	mid := seedScore(t, s, idx, lb, 200)
	low := seedScore(t, s, idx, lb, 100)

	if r := rankOf(t, s, idx, lb, high); r != 1 {
		t.Errorf("high rank = %d, want 1", r)
	}
	if r := rankOf(t, s, idx, lb, mid); r != 2 {
		t.Errorf("mid rank = %d, want 2", r)
	}
	if r := rankOf(t, s, idx, lb, low); r != 3 {
		t.Errorf("low rank = %d, want 3", r)
	}
}

func TestLeaderboardIndex_RanksAscending(t *testing.T) {
	idx, s := newLeaderboardIndex(t)
	lb := leaderboard.Leaderboard{ID: "test-asc-" + uuid.NewString(), SortOrder: leaderboard.Ascending}

	fast := seedScore(t, s, idx, lb, 45)
	slow := seedScore(t, s, idx, lb, 90)

	if r := rankOf(t, s, idx, lb, fast); r != 1 {
		t.Errorf("fast (lower) rank = %d, want 1", r)
	}
	if r := rankOf(t, s, idx, lb, slow); r != 2 {
		t.Errorf("slow rank = %d, want 2", r)
	}
}

// Dropping the ZSET key (as a Valkey eviction/restart would) must not break ranking: the next RankOf rebuilds the index from Postgres.
func TestLeaderboardIndex_RebuildsAfterKeyLoss(t *testing.T) {
	idx, s := newLeaderboardIndex(t)
	url := os.Getenv("VALKEY_URL")
	client, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	defer client.Close()

	lb := leaderboard.Leaderboard{ID: "test-rebuild-" + uuid.NewString(), SortOrder: leaderboard.Descending}
	top := seedScore(t, s, idx, lb, 500)
	seedScore(t, s, idx, lb, 250)

	// Evict the index.
	if err := client.Do(t.Context(), client.B().Del().Key("vfx:lb:"+lb.ID).Build()).Error(); err != nil {
		t.Fatalf("del key: %v", err)
	}

	if r := rankOf(t, s, idx, lb, top); r != 1 {
		t.Errorf("rank after index loss = %d, want 1 (rebuild failed)", r)
	}
}

func TestLeaderboardIndex_KeepsBestAndNotFound(t *testing.T) {
	idx, s := newLeaderboardIndex(t)
	lb := leaderboard.Leaderboard{ID: "test-best-" + uuid.NewString(), SortOrder: leaderboard.Descending}

	p := seedScore(t, s, idx, lb, 100)
	// A worse score must not replace the best, so the rank holds.
	mustRW(t, s, func(ctx context.Context) error {
		_, err := idx.Submit(ctx, lb, p, 50, time.Now().UTC())
		return err
	})
	mustRW(t, s, func(ctx context.Context) error {
		e, err := idx.RankOf(ctx, lb, p)
		if err != nil {
			return err
		}
		if e.Score != 100 {
			t.Errorf("score after worse submit = %d, want 100", e.Score)
		}
		return nil
	})

	// An unranked player is ErrEntryNotFound.
	if err := s.RW(context.Background(), func(ctx context.Context) error {
		_, err := idx.RankOf(ctx, lb, uuid.New())
		return err
	}); !errors.Is(err, leaderboard.ErrEntryNotFound) {
		t.Errorf("RankOf of unranked player: err = %v, want ErrEntryNotFound", err)
	}
}
