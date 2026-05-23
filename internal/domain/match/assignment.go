package match

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Assignment struct {
	MatchID      uuid.UUID
	Endpoint     string
	SessionToken string
	ExpiresAt    time.Time
}

// AssignmentStore outlives the Queue: a ticket's event stream closes once Matched is delivered, but a player may reconnect and call GetCurrentMatch afterwards.
type AssignmentStore interface {
	// ttl should track the session token lifetime; a stale assignment is useless once its token can no longer authenticate to the room.
	Put(ctx context.Context, playerID uuid.UUID, a *Assignment, ttl time.Duration) error
	Get(ctx context.Context, playerID uuid.UUID) (*Assignment, error)
}
