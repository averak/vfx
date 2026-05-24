package leaderboard

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Methods take the whole Leaderboard, not just an id, because ranking depends on its SortOrder.
type Repository interface {
	// Submit applies the keep-best rule and reports whether the player's best improved; an equal-or-worse score reports false, so a resubmit is idempotent.
	Submit(ctx context.Context, lb Leaderboard, playerID uuid.UUID, score int64, now time.Time) (improved bool, err error)

	// RankOf returns ErrEntryNotFound when the player has no entry.
	RankOf(ctx context.Context, lb Leaderboard, playerID uuid.UUID) (*RankedEntry, error)

	// TopRanks returns entries best-first; each entry's rank is offset + its position.
	TopRanks(ctx context.Context, lb Leaderboard, offset, limit int) ([]*RankedEntry, error)
}
