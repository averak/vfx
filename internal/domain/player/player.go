// Package player is the player aggregate.
package player

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxNicknameLength bounds a nickname in runes.
// Validating it here, rather than at the handler, keeps a valid display name intrinsic to the Player.
const MaxNicknameLength = 32

// ErrInvalidNickname rejects a present nickname that is blank or longer than MaxNicknameLength.
var ErrInvalidNickname = errors.New("player: invalid nickname")

// Player can carry multiple Identity rows (anonymous device today, OAuth providers later).
type Player struct {
	ID        uuid.UUID
	Nickname  *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// New accepts a nil nickname (the player is unnamed); a non-nil one must satisfy the nickname invariant.
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

// SetNickname clears the nickname when nickname is nil; a non-nil one must satisfy the nickname invariant.
func (p *Player) SetNickname(nickname *string, now time.Time) error {
	if err := validateNickname(nickname); err != nil {
		return err
	}
	p.Nickname = nickname
	p.UpdatedAt = now
	return nil
}

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
