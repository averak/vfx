package player

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Repositories return these (or errors wrapping them) so callers act on the failure kind without parsing strings.
var (
	ErrPlayerNotFound   = errors.New("player: not found")
	ErrIdentityNotFound = errors.New("player: identity not found")

	// ErrIdentityAlreadyLinked is returned when linking a provider identity that already belongs to a different player.
	ErrIdentityAlreadyLinked = errors.New("player: identity already linked to another player")
)

// Repository takes only a context; the usecase owns the transaction boundary and the active transaction rides on the context, keeping the domain free of any persistence type.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Player, error)

	// Save persists the whole Player aggregate; there is no per-field update, so a caller mutates the Player via its methods and saves it back.
	Save(ctx context.Context, p *Player) error
}
