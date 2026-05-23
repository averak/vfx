package match

import (
	"context"

	"github.com/google/uuid"
)

// RoomAllocation is the matchmaker's view of a freshly reserved room.
// The usecase layers the session token on top, since only it knows the player and TTL; the Allocator only knows where the room runs.
type RoomAllocation struct {
	MatchID  uuid.UUID
	Endpoint string
}

// Allocator reserves a room for an upcoming match.
// The stub points every match at one fixed endpoint (local/compose); the Agones implementation reserves a GameServer per match in a cluster.
type Allocator interface {
	Allocate(ctx context.Context, gameMode string, playerCount int) (*RoomAllocation, error)
}
