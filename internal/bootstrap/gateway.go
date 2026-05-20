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

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/allocator"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/metrics"
	"github.com/averak/vfx/internal/infra/postgres"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/infra/valkey"
	gatewayauthhandler "github.com/averak/vfx/internal/presentation/gateway/auth"
	gatewaymatchhandler "github.com/averak/vfx/internal/presentation/gateway/match"
	"github.com/averak/vfx/internal/stdx/db"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
)

// Gateway bundles everything the gateway process needs at runtime.
type Gateway struct {
	Config *config.Gateway

	Pool    *pgxpool.Pool
	Valkey  valkeygo.Client
	Metrics *metrics.Registry

	Session *db.Session
	Signer  *token.Signer

	PlayerRepo       player.Repository
	RefreshTokenRepo player.RefreshTokenRepository

	MatchQueue     domainmatch.Queue
	MatchAllocator domainmatch.Allocator
	Matchmaker     *usecasematch.Matchmaker

	AuthUsecase *usecaseauth.Usecase
	AuthHandler *gatewayauthhandler.Handler

	MatchUsecase *usecasematch.Usecase
	MatchHandler *gatewaymatchhandler.Handler
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

	matchQueue := matchqueue.NewInMem()
	matchAllocator := allocator.NewStub(cfg.RoomEndpoint)
	matchUC := usecasematch.New(matchQueue)
	matchHandler := gatewaymatchhandler.New(matchUC)
	matchmaker := usecasematch.NewMatchmaker(matchQueue, matchAllocator, signer, usecasematch.Config{
		Interval:        cfg.MatchmakerInterval,
		SessionTokenTTL: cfg.SessionTokenTTL,
		PlayersPerMatch: 2,
		GameModes:       []string{"rps"},
	})

	cleanup := func() {
		valkeyClient.Close()
		pool.Close()
	}

	return &Gateway{
		Config:           cfg,
		Pool:             pool,
		Valkey:           valkeyClient,
		Metrics:          metrics.NewRegistry(),
		Session:          session,
		Signer:           signer,
		PlayerRepo:       playerRepo,
		RefreshTokenRepo: refreshRepo,
		MatchQueue:       matchQueue,
		MatchAllocator:   matchAllocator,
		Matchmaker:       matchmaker,
		AuthUsecase:      authUC,
		AuthHandler:      authHandler,
		MatchUsecase:     matchUC,
		MatchHandler:     matchHandler,
	}, cleanup, nil
}
