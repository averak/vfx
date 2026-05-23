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
)

// Repository takes only a context: the usecase owns the transaction boundary (via a Transactor) and the active transaction rides on the context.
// This keeps the domain interface free of any persistence-technology type.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Player, error)
	Save(ctx context.Context, p *Player) error
	UpdateNickname(ctx context.Context, p *Player) error

	FindPlayerByIdentity(ctx context.Context, provider Provider, providerUID string) (*Player, error)
	SaveIdentity(ctx context.Context, i *Identity) error
}
