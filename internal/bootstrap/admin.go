package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/postgres"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/valkey"
	adminhandler "github.com/averak/vfx/internal/presentation/admin"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
)

// Admin bundles everything the admin process needs at runtime.
type Admin struct {
	Config  *config.Admin
	Pool    *pgxpool.Pool
	Valkey  valkeygo.Client
	Handler http.Handler
}

// NewAdmin constructs and validates the admin container.
func NewAdmin(ctx context.Context) (*Admin, func(), error) {
	cfg, err := config.LoadAdmin()
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

	matchQueue, err := newMatchQueue(cfg.MatchQueue, valkeyClient)
	if err != nil {
		valkeyClient.Close()
		pool.Close()
		return nil, nil, err
	}

	uc := usecaseadmin.New(db.NewSession(pool), repository.NewPlayer(), matchQueue)
	handler := adminhandler.NewHandler(uc, pool, cfg.AuthToken)

	cleanup := func() {
		valkeyClient.Close()
		pool.Close()
	}

	return &Admin{
		Config:  cfg,
		Pool:    pool,
		Valkey:  valkeyClient,
		Handler: handler,
	}, cleanup, nil
}
