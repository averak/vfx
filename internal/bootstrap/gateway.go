// Package bootstrap wires every dependency a subcommand needs.
//
// Each subcommand has its own container so unused dependencies do not
// participate in startup. The container is intentionally a plain struct
// constructed by a top-down function — manual wiring keeps the
// dependency graph visible without pulling in a DI library.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/postgres"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/infra/valkey"
	gatewayauthhandler "github.com/averak/vfx/internal/presentation/gateway/auth"
	"github.com/averak/vfx/internal/stdx/db"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
)

// Gateway bundles everything the gateway process needs at runtime.
type Gateway struct {
	Config *config.Gateway

	Pool   *pgxpool.Pool
	Valkey valkeygo.Client

	Session *db.Session
	Signer  *token.Signer

	PlayerRepo       player.Repository
	RefreshTokenRepo player.RefreshTokenRepository

	AuthUsecase *usecaseauth.Usecase
	AuthHandler *gatewayauthhandler.Handler
}

// NewGateway constructs and validates the gateway container. The
// returned cleanup function closes long-lived resources and must be
// called before the process exits.
func NewGateway(ctx context.Context) (*Gateway, func(), error) {
	cfg, err := config.LoadGateway()
	if err != nil {
		return nil, nil, err
	}

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}

	valkeyClient, err := valkey.NewClient(cfg.ValkeyURL)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("bootstrap: %w", err)
	}

	session := db.NewSession(pool)
	signer := token.NewSigner(cfg.JWTSecret)
	playerRepo := repository.NewPlayer()
	refreshRepo := repository.NewRefreshToken()
	authUC := usecaseauth.New(
		session,
		playerRepo,
		refreshRepo,
		signer,
		cfg.AccessTokenTTL,
		cfg.RefreshTokenTTL,
	)
	authHandler := gatewayauthhandler.New(authUC)

	cleanup := func() {
		valkeyClient.Close()
		pool.Close()
	}

	return &Gateway{
		Config:           cfg,
		Pool:             pool,
		Valkey:           valkeyClient,
		Session:          session,
		Signer:           signer,
		PlayerRepo:       playerRepo,
		RefreshTokenRepo: refreshRepo,
		AuthUsecase:      authUC,
		AuthHandler:      authHandler,
	}, cleanup, nil
}
