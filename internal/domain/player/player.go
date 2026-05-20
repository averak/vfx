// Package player is the player aggregate. It owns the Player entity,
// its identity links to authentication providers, and the repository
// contract used to persist them.
package player

import (
	"time"

	"github.com/google/uuid"
)

// Player is the core game profile. A single Player can carry multiple
// Identity rows (anonymous device today, OAuth providers later).
type Player struct {
	ID        uuid.UUID
	Nickname  *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New constructs a Player ready for first insertion.
func New(id uuid.UUID, nickname *string, now time.Time) *Player {
	return &Player{
		ID:        id,
		Nickname:  nickname,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// SetNickname updates the nickname and refreshes UpdatedAt. The argument
// is taken by pointer so a caller can explicitly clear the field by
// passing nil.
func (p *Player) SetNickname(nickname *string, now time.Time) {
	p.Nickname = nickname
	p.UpdatedAt = now
}
