package match

import (
	"context"
)

// RoomAllocation is the matchmaker's view of a freshly reserved room.
// The session token shown to clients is layered on top of this by the
// usecase: it knows the player id and TTL, the Allocator only knows
// where the room runs.
type RoomAllocation struct {
	MatchID  string // UUID string; stored as match_id in DB and tokens.
	Endpoint string
}

// Allocator reserves a room for an upcoming match.
//
// A deterministic stub implementation always points clients at the same
// local endpoint, for local and compose runs; the Agones-backed
// implementation reserves a GameServer per match in a cluster.
type Allocator interface {
	Allocate(ctx context.Context, gameMode string, playerCount int) (*RoomAllocation, error)
}
