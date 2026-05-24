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

type Player struct {
	ID           uuid.UUID
	Nickname     *string
	RegisteredAt time.Time
}

// A nil nickname leaves the player unnamed.
func New(id uuid.UUID, nickname *string, now time.Time) (*Player, error) {
	if err := validateNickname(nickname); err != nil {
		return nil, err
	}
	return &Player{
		ID:           id,
		Nickname:     nickname,
		RegisteredAt: now,
	}, nil
}

// A nil nickname clears it.
func (p *Player) SetNickname(nickname *string) error {
	if err := validateNickname(nickname); err != nil {
		return err
	}
	p.Nickname = nickname
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
