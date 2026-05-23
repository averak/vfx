package player

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Sentinel errors. Repositories must return these (or errors wrapping
// them) so callers can act on the failure kind without parsing strings.
var (
	ErrPlayerNotFound   = errors.New("player: not found")
	ErrIdentityNotFound = errors.New("player: identity not found")
)

// Repository persists Player and Identity rows. Methods take only a
// context; the transaction boundary is owned by the usecase (via a
// Transactor) and the active transaction is carried on the context, so
// this domain interface stays free of any persistence-technology type.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Player, error)
	Save(ctx context.Context, p *Player) error
	UpdateNickname(ctx context.Context, p *Player) error

	FindPlayerByIdentity(ctx context.Context, provider Provider, providerUID string) (*Player, error)
	SaveIdentity(ctx context.Context, i *Identity) error
}
