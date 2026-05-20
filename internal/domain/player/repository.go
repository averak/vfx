package player

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Sentinel errors. Repositories must return these (or errors wrapping
// them) so callers can act on the failure kind without parsing strings.
var (
	ErrPlayerNotFound   = errors.New("player: not found")
	ErrIdentityNotFound = errors.New("player: identity not found")
)

// Repository persists Player and Identity rows. All methods take a
// [pgx.Tx] so the caller decides the transaction boundary; an
// implementation never opens its own.
type Repository interface {
	GetByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Player, error)
	Save(ctx context.Context, tx pgx.Tx, p *Player) error
	UpdateNickname(ctx context.Context, tx pgx.Tx, p *Player) error

	FindPlayerByIdentity(ctx context.Context, tx pgx.Tx, provider Provider, providerUID string) (*Player, error)
	SaveIdentity(ctx context.Context, tx pgx.Tx, i *Identity) error
}
