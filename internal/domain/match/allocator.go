package match

import (
	"context"

	"github.com/google/uuid"
)

// RoomAllocation deliberately carries no session token: only the usecase knows the player and TTL, so it adds the token on top of what the Allocator returns.
type RoomAllocation struct {
	MatchID  uuid.UUID
	Endpoint string
}

type Allocator interface {
	Allocate(ctx context.Context, gameMode string, playerCount int) (*RoomAllocation, error)
}
