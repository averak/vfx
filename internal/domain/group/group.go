// Package group is the player-group aggregate (clans, guilds).
//
// The creator is the owner and first member; the owner disbands the group with DeleteGroup rather than leaving it.
package group

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxNameLength = 64

var (
	ErrInvalidName     = errors.New("group: name is blank or too long")
	ErrNotFound        = errors.New("group: not found")
	ErrNotOwner        = errors.New("group: only the owner may do that")
	ErrOwnerMustDelete = errors.New("group: the owner cannot leave; delete the group instead")
	ErrNotMember       = errors.New("group: not a member")
)

// Group is a named player group owned by its creator.
type Group struct {
	ID        uuid.UUID
	Name      string
	OwnerID   uuid.UUID
	CreatedAt time.Time
}

func New(id, ownerID uuid.UUID, name string, now time.Time) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxNameLength {
		return nil, ErrInvalidName
	}
	return &Group{ID: id, Name: name, OwnerID: ownerID, CreatedAt: now}, nil
}

// Member is a read model: a group member with their display name and join time.
type Member struct {
	PlayerID uuid.UUID
	Nickname *string
	JoinedAt time.Time
}
