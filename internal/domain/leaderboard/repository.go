package leaderboard

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository persists leaderboard scores and answers ranking queries.
// Ranking is order-aware, so methods take the Leaderboard (for its SortOrder) rather than just an id.
type Repository interface {
	// Submit applies the keep-best rule atomically (a single conditional upsert, no row lock or read-modify-write) and reports whether the player's best improved.
	// improved is false when an equal-or-worse score left the existing best unchanged, which makes a resubmit idempotent.
	Submit(ctx context.Context, lb Leaderboard, playerID uuid.UUID, score int64, now time.Time) (improved bool, err error)

	// RankOf returns the player's ranked entry, or ErrEntryNotFound.
	RankOf(ctx context.Context, lb Leaderboard, playerID uuid.UUID) (*RankedEntry, error)

	// TopRanks returns entries best-first, paginated; the rank field is offset + position.
	TopRanks(ctx context.Context, lb Leaderboard, offset, limit int) ([]*RankedEntry, error)
}
