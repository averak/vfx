// Package player is the player aggregate.
// It owns the Player entity, its identity links to authentication providers, and the repository contract used to persist them.
package player

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxNicknameLength bounds a nickname in runes.
// The limit is an enterprise rule: a valid Player's display name is intrinsic to the Player, independent of any transport or storage that happens to carry it.
const MaxNicknameLength = 32

// ErrInvalidNickname rejects a nickname that is present but blank or longer than MaxNicknameLength.
var ErrInvalidNickname = errors.New("player: invalid nickname")

// Player is the core game profile.
// A single Player can carry multiple Identity rows (anonymous device today, OAuth providers later).
type Player struct {
	ID        uuid.UUID
	Nickname  *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New constructs a Player ready for first insertion.
// A nil nickname leaves the player unnamed; a non-nil one must satisfy the nickname invariant.
func New(id uuid.UUID, nickname *string, now time.Time) (*Player, error) {
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}
	return &Player{
		ID:        id,
		Nickname:  nickname,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// SetNickname updates the nickname and refreshes UpdatedAt.
// A nil nickname clears the field; a non-nil one must satisfy the nickname invariant.
func (p *Player) SetNickname(nickname *string, now time.Time) error {
	if err := validateNickname(nickname); err != nil {
		return err
	}
	p.Nickname = nickname
	p.UpdatedAt = now
	return nil
}

// validateNickname enforces the nickname invariant: nil is allowed, but a present nickname must be non-blank and at most MaxNicknameLength runes.
func validateNickname(nickname *string) error {
	if nickname == nil {
		return nil
	}
	if strings.TrimSpace(*nickname) == "" {
		return ErrInvalidNickname
	}
	if utf8.RuneCountInString(*nickname) > MaxNicknameLength {
		return ErrInvalidNickname
	}
	return nil
}
