// Package match is the matchmaking aggregate. Ticket models a queued
// matchmaking request; Assignment captures the result handed back to a
// client once the matchmaker has paired them up.
package match

import (
	"time"

	"github.com/google/uuid"
)

// Ticket is one player's request to be matched into a game.
type Ticket struct {
	ID       uuid.UUID
	PlayerID uuid.UUID
	GameMode string

	// Rating, Region, and PartyMembers are matchmaker hints. None of
	// them is currently used by the simple pairing strategy, but the
	// shape is fixed here so the matchmaker can grow without changing
	// the schema.
	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string

	CreatedAt time.Time
}

// NewTicket builds a Ticket ready to be enqueued.
func NewTicket(id, playerID uuid.UUID, gameMode string, now time.Time) *Ticket {
	return &Ticket{
		ID:        id,
		PlayerID:  playerID,
		GameMode:  gameMode,
		CreatedAt: now,
	}
}
