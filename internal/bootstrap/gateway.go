// Package bootstrap wires every dependency a subcommand needs.
//
// Each subcommand has its own container so unused dependencies do not participate in startup.
// The container is intentionally a plain struct constructed by a top-down function: manual wiring keeps the dependency graph visible without pulling in a DI library.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	valkeygo "github.com/valkey-io/valkey-go"

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/allocator"
	"github.com/averak/vfx/internal/infra/assignmentstore"
	"github.com/averak/vfx/internal/infra/blobstore"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/metrics"
	"github.com/averak/vfx/internal/infra/postgres"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/infra/valkey"
	gatewayauthhandler "github.com/averak/vfx/internal/presentation/gateway/auth"
	gatewaymatchhandler "github.com/averak/vfx/internal/presentation/gateway/match"
	gatewaystoragehandler "github.com/averak/vfx/internal/presentation/gateway/storage"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

type Gateway struct {
	Config *config.Gateway

	Pool    *pgxpool.Pool
	Valkey  valkeygo.Client
	Metrics *metrics.Registry

	Session *db.Session
	Signer  *token.Signer

	PlayerRepo       player.Repository
	RefreshTokenRepo player.RefreshTokenRepository

	MatchQueue       domainmatch.Queue
	MatchAllocator   domainmatch.Allocator
	MatchAssignments domainmatch.AssignmentStore
	Matchmaker       *usecasematch.Matchmaker

	AuthUsecase *usecaseauth.Usecase
	AuthHandler *gatewayauthhandler.Handler

	MatchUsecase *usecasematch.Usecase
	MatchHandler *gatewaymatchhandler.Handler

	// Storage fields are nil when VFX_STORAGE_BUCKET is unset: the storage services are then disabled and the gateway needs no object store.
	StorageUsecase           *usecasestorage.Usecase
	PlayerDataStorageHandler *gatewaystoragehandler.PlayerDataHandler
	TitleStorageHandler      *gatewaystoragehandler.TitleHandler
}

// matchmakerMetrics adapts the Prometheus registry to the usecasematch.Metrics interface, keeping the usecase layer free of a concrete metrics dependency.
type matchmakerMetrics struct {
	reg *metrics.Registry
}

func (m matchmakerMetrics) MatchAllocated() {
	m.reg.MatchesAllocated.Inc()
}

func (m matchmakerMetrics) SetQueueDepth(gameMode string, depth int) {
	m.reg.QueueDepth.WithLabelValues(gameMode).Set(float64(depth))
}

// newMatchQueue picks the matchmaking queue by mode: in-memory (single process) or Valkey-backed (shared across gateway replicas, and the one the admin API reads to report queue depth).
func newMatchQueue(mode string, valkeyClient valkeygo.Client) (domainmatch.Queue, error) {
	switch mode {
	case "valkey":
		return matchqueue.NewValkey(valkeyClient), nil
	case "", "inmem":
		return matchqueue.NewInMem(), nil
	default:
		return nil, fmt.Errorf("bootstrap: unknown match queue %q (want \"inmem\" or \"valkey\")", mode)
	}
}

// newAllocator picks the room allocator from config: the stub (single fixed endpoint, for compose/local) or Agones (a GameServerAllocation per match against the in-cluster API).
func newAllocator(cfg *config.Gateway) (domainmatch.Allocator, error) {
	switch cfg.Allocator {
	case "agones":
		return allocator.NewAgones(cfg.AgonesNamespace)
	case "", "stub":
		return allocator.NewStub(cfg.RoomEndpoint), nil
	default:
		return nil, fmt.Errorf("bootstrap: unknown allocator %q (want \"stub\" or \"agones\")", cfg.Allocator)
	}
}

// NewGateway's returned cleanup closes long-lived resources and must be called before the process exits.
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

	metricsReg := metrics.NewRegistry()

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

	matchQueue, err := newMatchQueue(cfg.MatchQueue, valkeyClient)
	if err != nil {
		valkeyClient.Close()
		pool.Close()
		return nil, nil, err
	}
	matchAllocator, err := newAllocator(cfg)
	if err != nil {
		valkeyClient.Close()
		pool.Close()
		return nil, nil, err
	}
	matchAssignments := assignmentstore.NewValkey(valkeyClient)
	matchUC := usecasematch.New(matchQueue, matchAssignments)
	matchHandler := gatewaymatchhandler.New(matchUC)
	matchmaker := usecasematch.NewMatchmaker(matchQueue, matchAllocator, signer, usecasematch.Config{
		Interval:                 cfg.MatchmakerInterval,
		SessionTokenTTL:          cfg.SessionTokenTTL,
		PlayersPerMatch:          cfg.PlayersPerMatch,
		GameModes:                cfg.GameModes,
		BaseRatingWindow:         cfg.MatchBaseRatingWindow,
		RatingWindowGrowthPerSec: cfg.MatchRatingWindowGrowthPerSec,
		RegionRelaxAfter:         cfg.MatchRegionRelaxAfter,
		Assignments:              matchAssignments,
		Metrics:                  matchmakerMetrics{reg: metricsReg},
	})

	var (
		storageUC         *usecasestorage.Usecase
		playerDataHandler *gatewaystoragehandler.PlayerDataHandler
		titleHandler      *gatewaystoragehandler.TitleHandler
		blobCleanup       = func() {}
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
		storageUC = usecasestorage.New(
			session,
			session,
			repository.NewPlayerFile(),
			repository.NewTitleFile(),
			blobs,
			usecasestorage.Config{
				PlayerDataPrefix:  cfg.StoragePlayerDataPrefix,
				TitlePrefix:       cfg.StorageTitlePrefix,
				URLTTL:            cfg.StorageURLTTL,
				MaxBytesPerPlayer: cfg.StorageMaxBytesPerPlayer,
				MaxFilesPerPlayer: cfg.StorageMaxFilesPerPlayer,
			},
		)
		playerDataHandler = gatewaystoragehandler.NewPlayerDataHandler(storageUC)
		titleHandler = gatewaystoragehandler.NewTitleHandler(storageUC)
	}

	cleanup := func() {
		blobCleanup()
		valkeyClient.Close()
		pool.Close()
	}

	return &Gateway{
		Config:           cfg,
		Pool:             pool,
		Valkey:           valkeyClient,
		Metrics:          metricsReg,
		Session:          session,
		Signer:           signer,
		PlayerRepo:       playerRepo,
		RefreshTokenRepo: refreshRepo,
		MatchQueue:       matchQueue,
		MatchAllocator:   matchAllocator,
		MatchAssignments: matchAssignments,
		Matchmaker:       matchmaker,
		AuthUsecase:      authUC,
		AuthHandler:      authHandler,
		MatchUsecase:     matchUC,
		MatchHandler:     matchHandler,

		StorageUsecase:           storageUC,
		PlayerDataStorageHandler: playerDataHandler,
		TitleStorageHandler:      titleHandler,
	}, cleanup, nil
}
