package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/infra/blobstore"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/postgres"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/valkey"
	adminhandler "github.com/averak/vfx/internal/presentation/admin"
	usecaseadmin "github.com/averak/vfx/internal/usecase/admin"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

type Admin struct {
	Config  *config.Admin
	Pool    *pgxpool.Pool
	Valkey  valkeygo.Client
	Handler http.Handler
}

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

	var (
		storageUC   *usecasestorage.Usecase
		blobCleanup = func() {}
	)
	if cfg.StorageBucket != "" {
		blobs, cleanup, blobErr := blobstore.NewGCS(ctx, blobstore.Config{
			Bucket:   cfg.StorageBucket,
			Emulated: cfg.StorageEmulatorHost != "",
		})
		if blobErr != nil {
			valkeyClient.Close()
			pool.Close()
			return nil, nil, blobErr
		}
		blobCleanup = cleanup
		session := db.NewSession(pool)
		// Only the title-file methods are exercised here, so player-data prefix/quota knobs are left at their zero values.
		storageUC = usecasestorage.New(session, session, repository.NewPlayerFile(), repository.NewTitleFile(), blobs, usecasestorage.Config{
			TitlePrefix: cfg.StorageTitlePrefix,
		})
	}

	handler := adminhandler.NewHandler(uc, storageUC, pool, cfg.AuthToken)

	cleanup := func() {
		blobCleanup()
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
