// Package leaderboard is the leaderboard aggregate.
//
// A leaderboard is an operator-defined ranking with a fixed sort order; scores are kept best-first per player.
package leaderboard

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrLeaderboardNotFound is returned for a leaderboard id the deployment did not define.
	ErrLeaderboardNotFound = errors.New("leaderboard: not defined")

	// ErrEntryNotFound is returned when a player has submitted no score to the leaderboard.
	ErrEntryNotFound = errors.New("leaderboard: player has no entry")
)

// SortOrder fixes whether a higher or lower score ranks better; it is set when the leaderboard is defined, not per submission.
type SortOrder int

const (
	Descending SortOrder = iota // higher score ranks better (high-score boards)
	Ascending                   // lower score ranks better (race times)
)

// Leaderboard is an operator-defined ranking identified by ID.
type Leaderboard struct {
	ID        string
	SortOrder SortOrder
}

// Beats reports whether score a ranks strictly better than b under this leaderboard's order.
// It states the keep-best rule; the repository enforces it atomically via a conditional upsert whose WHERE clause mirrors this method, so concurrent submits never lose a better score without taking a lock.
func (l Leaderboard) Beats(a, b int64) bool {
	if l.SortOrder == Ascending {
		return a < b
	}
	return a > b
}

// RankedEntry is a read model: an entry with its computed 1-based rank and the player's display name.
type RankedEntry struct {
	Rank       int64
	PlayerID   uuid.UUID
	Nickname   *string
	Score      int64
	AchievedAt time.Time
}
