package player

import (
	"time"

	"github.com/google/uuid"
)

// Provider names the authentication provider that issued the provider_uid.
// Anonymous is the only one currently supported; OAuth providers would be added as further constants.
type Provider string

const (
	ProviderAnonymous Provider = "anonymous"
)

// Identity links a Player to one authentication credential.
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
