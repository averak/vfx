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

// Repository takes only a context: the usecase owns the transaction boundary (via a Transactor) and the active transaction rides on the context.
// This keeps the domain interface free of any persistence-technology type.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Player, error)

	// Save persists the whole Player aggregate (create or update); there is no per-field update, so a caller mutates the Player via its methods and saves it back.
	Save(ctx context.Context, p *Player) error

	FindPlayerByIdentity(ctx context.Context, provider Provider, providerUID string) (*Player, error)
	SaveIdentity(ctx context.Context, i *Identity) error
}
