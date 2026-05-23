package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type Admin struct {
	ListenAddr string `env:"VFX_ADMIN_LISTEN_ADDR" envDefault:":8090"`

	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	ValkeyURL   string `env:"VALKEY_URL,notEmpty"     envDefault:"redis://localhost:6379"`

	// MatchQueue must match the gateway's setting so the admin reads the same queue the matchmaker writes; only "valkey" is meaningful across processes ("inmem" would show an empty, process-local queue).
	MatchQueue string `env:"VFX_MATCH_QUEUE" envDefault:"inmem"`

	// AuthToken, when set, is required as a bearer token on every non-probe request.
	// Empty leaves the API open (suitable only when a network boundary already restricts access); operators should set it on any shared cluster.
	AuthToken string `env:"VFX_ADMIN_AUTH_TOKEN"`

	// StorageBucket enables the title-file publish/unpublish endpoints; empty leaves them unmounted.
	StorageBucket       string `env:"VFX_STORAGE_BUCKET"`
	StorageEmulatorHost string `env:"STORAGE_EMULATOR_HOST"`
	StorageTitlePrefix  string `env:"VFX_STORAGE_TITLE_PREFIX" envDefault:"title"`
}

func LoadAdmin() (*Admin, error) {
	var cfg Admin
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}
