// Package match is the matchmaking aggregate. Ticket models a queued
// matchmaking request; Assignment captures the result handed back to a
// client once the matchmaker has paired them up. The matchmaking rules
// themselves (eligibility, tier relaxation) live here in the domain —
// see Ticket.CompatibleWith and Matcher — because they are intrinsic to
// matchmaking, not to any one application flow.
package match

import (
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
)

// Invariants a Ticket must satisfy to exist.
var (
	ErrTicketNoPlayer   = errors.New("match: ticket requires a player")
	ErrTicketNoGameMode = errors.New("match: ticket requires a game mode")
)

// Ticket is one player's request to be matched into a game.
type Ticket struct {
	ID       uuid.UUID
	PlayerID uuid.UUID
	GameMode string

	// Rating and Region drive tier-based matching (see CompatibleWith);
	// PartyMembers and Attributes are carried for future grouping and
	// plugin-specific hints. All are optional.
	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string

	CreatedAt time.Time
}

// NewTicket builds a Ticket, enforcing its invariants: it must name a
// player and a game mode. Optional hints (rating, region, party,
// attributes) are set on the returned value by the caller.
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

// CompatibleWith reports whether other may be matched with t under the
// given rating window and region rule. A ticket missing a rating or a
// region skips that dimension rather than blocking the match.
//
// This is an enterprise business rule — pairing eligibility between two
// tickets is intrinsic to matchmaking — so it lives on the entity, not
// in the usecase. The window and ignoreRegion inputs are decided by the
// tier policy (Matcher), which knows how long the tickets have waited.
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
