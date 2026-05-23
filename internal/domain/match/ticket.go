// Package match is the matchmaking domain.
// The matchmaking rules themselves — pairing eligibility and tier relaxation — live here (Ticket.CompatibleWith, Matcher), not in the usecase, because they are intrinsic to matchmaking rather than to any one application flow.
package match

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTicketNoPlayer   = errors.New("match: ticket requires a player")
	ErrTicketNoGameMode = errors.New("match: ticket requires a game mode")
)

type Ticket struct {
	ID       uuid.UUID
	PlayerID uuid.UUID
	GameMode string

	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string

	CreatedAt time.Time
}

func NewTicket(id, playerID uuid.UUID, gameMode string, now time.Time) (*Ticket, error) {
	if playerID == uuid.Nil {
		return nil, ErrTicketNoPlayer
	}
	if gameMode == "" {
		return nil, ErrTicketNoGameMode
	}
	return &Ticket{
		ID:        id,
		PlayerID:  playerID,
		GameMode:  gameMode,
		CreatedAt: now,
	}, nil
}

// CompatibleWith treats a missing rating or region as "skip that dimension", not as a mismatch.
func (t *Ticket) CompatibleWith(other *Ticket, ratingWindow float64, ignoreRegion bool) bool {
	if !ignoreRegion && t.Region != nil && other.Region != nil && *t.Region != *other.Region {
		return false
	}
	if t.Rating != nil && other.Rating != nil {
		if math.Abs(*t.Rating-*other.Rating) > ratingWindow {
			return false
		}
	}
	return true
}
