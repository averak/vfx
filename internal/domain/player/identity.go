package player

import (
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
