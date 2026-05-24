package player

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Provider string

const (
	ProviderAnonymous Provider = "anonymous"
	ProviderGoogle    Provider = "google"
	ProviderApple     Provider = "apple"
)

type Identity struct {
	ID          uuid.UUID
	PlayerID    uuid.UUID
	Provider    Provider
	ProviderUID string
	CreatedAt   time.Time
}

func NewIdentity(id, playerID uuid.UUID, provider Provider, providerUID string, now time.Time) *Identity {
	return &Identity{
		ID:          id,
		PlayerID:    playerID,
		Provider:    provider,
		ProviderUID: providerUID,
		CreatedAt:   now,
	}
}

// Identity is its own aggregate root: a provider credential that authenticates a Player, globally unique on (Provider, ProviderUID) and referencing its owner by PlayerID.
// It is persisted apart from the Player, so resolving or linking a credential never loads the whole player and its global-uniqueness invariant lives with it.
type IdentityRepository interface {
	// Find returns ErrIdentityNotFound when no identity matches (provider, providerUID).
	Find(ctx context.Context, provider Provider, providerUID string) (*Identity, error)

	// Save links the identity, returning ErrIdentityAlreadyLinked when (provider, providerUID) already belongs to another player.
	Save(ctx context.Context, i *Identity) error
}
