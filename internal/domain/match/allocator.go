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
// Phase 1 ships a deterministic stub that always points clients at the
// same local endpoint. Production deployments swap in an Agones-backed
// implementation that calls the Allocator API.
type Allocator interface {
	Allocate(ctx context.Context, gameMode string, playerCount int) (*RoomAllocation, error)
}
