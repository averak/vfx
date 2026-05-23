package leaderboard

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository persists leaderboard entries and answers ranking queries.
// Ranking is order-aware, so methods take the Leaderboard (for its SortOrder) rather than just an id.
type Repository interface {
	// GetEntry returns ErrEntryNotFound when the player has no score on the leaderboard.
	GetEntry(ctx context.Context, leaderboardID string, playerID uuid.UUID) (*Entry, error)

	// SaveEntry upserts the player's score unconditionally; the caller has already applied the keep-best rule via Leaderboard.Beats.
	SaveEntry(ctx context.Context, leaderboardID string, playerID uuid.UUID, score int64, now time.Time) error

	// RankOf returns the player's ranked entry, or ErrEntryNotFound.
	RankOf(ctx context.Context, lb Leaderboard, playerID uuid.UUID) (*RankedEntry, error)

	// TopRanks returns entries best-first, paginated; the rank field is offset + position.
	TopRanks(ctx context.Context, lb Leaderboard, offset, limit int) ([]*RankedEntry, error)
}
