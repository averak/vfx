// Package testconnect spins up an in-process Connect-RPC server wired with the real gateway handlers and interceptors, for component tests that exercise the HTTP/RPC boundary.
//
// It deliberately wires the handlers by hand rather than calling bootstrap.NewGateway, because bootstrap also dials Valkey and reads process config, which a handler test should not depend on.
// The connect protocol streams over HTTP/1.1, so a plain httptest server is enough for both unary and server-streaming RPCs.
package testconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/chat/chatconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/group/groupconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/leaderboard/leaderboardconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/social/socialconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/storage/storageconnect"
	domainleaderboard "github.com/averak/vfx/internal/domain/leaderboard"
	domainstorage "github.com/averak/vfx/internal/domain/storage"
	"github.com/averak/vfx/internal/infra/assignmentstore"
	"github.com/averak/vfx/internal/infra/connectrpc/interceptor"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	gatewayauthhandler "github.com/averak/vfx/internal/presentation/gateway/auth"
	gatewaychathandler "github.com/averak/vfx/internal/presentation/gateway/chat"
	gatewaygrouphandler "github.com/averak/vfx/internal/presentation/gateway/group"
	gatewayleaderboardhandler "github.com/averak/vfx/internal/presentation/gateway/leaderboard"
	gatewaymatchhandler "github.com/averak/vfx/internal/presentation/gateway/match"
	gatewaysocialhandler "github.com/averak/vfx/internal/presentation/gateway/social"
	gatewaystoragehandler "github.com/averak/vfx/internal/presentation/gateway/storage"
	"github.com/averak/vfx/internal/testutils/fakeblob"
	"github.com/averak/vfx/internal/testutils/fakeoidc"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
	usecasechat "github.com/averak/vfx/internal/usecase/chat"
	usecasegroup "github.com/averak/vfx/internal/usecase/group"
	usecaseleaderboard "github.com/averak/vfx/internal/usecase/leaderboard"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
	usecasesocial "github.com/averak/vfx/internal/usecase/social"
	usecasestorage "github.com/averak/vfx/internal/usecase/storage"
)

// Leaderboard ids the harness defines: a descending (high-score) and an ascending (best-time) board.
const (
	LeaderboardDesc = "global"
	LeaderboardAsc  = "besttime"
)

const jwtSecret = "test-secret"

// Storage limits are kept tiny so quota and size rejections are easy to trigger from a handler test.
const (
	StorageMaxBytesPerPlayer = 1024
	StorageMaxFilesPerPlayer = 2
)

type Server struct {
	Auth        authconnect.AuthServiceClient
	Match       matchconnect.MatchServiceClient
	PlayerData  storageconnect.PlayerDataStorageServiceClient
	Title       storageconnect.TitleStorageServiceClient
	Leaderboard leaderboardconnect.LeaderboardServiceClient
	Social      socialconnect.SocialServiceClient
	Chat        chatconnect.ChatServiceClient
	Group       groupconnect.GroupServiceClient

	// Blob is the in-memory object store behind the storage services; tests Put an object to simulate a finished upload before CommitFile.
	Blob *fakeblob.Store

	session    *db.Session
	httpServer *httptest.Server
}

// New wires the gateway handlers against a clean test database and starts an httptest server, torn down via t.Cleanup.
// Tests that do not set DATABASE_URL are skipped (see testdb.Pool).
func New(t *testing.T) *Server {
	t.Helper()

	pool := testdb.Pool(t)
	session := db.NewSession(pool)
	signer := token.NewSigner(jwtSecret)

	authUC := usecaseauth.New(
		session,
		repository.NewPlayer(),
		repository.NewRefreshToken(),
		signer,
		fakeoidc.New(),
		15*time.Minute,
		720*time.Hour,
	)
	matchUC := usecasematch.New(matchqueue.NewInMem(), assignmentstore.NewInMem())

	blob := fakeblob.New()
	storageUC := usecasestorage.New(
		session,
		session,
		repository.NewPlayerFile(),
		repository.NewTitleFile(),
		blob,
		usecasestorage.Config{
			PlayerDataPrefix:  "player-data",
			TitlePrefix:       "title",
			URLTTL:            5 * time.Minute,
			MaxBytesPerPlayer: StorageMaxBytesPerPlayer,
			MaxFilesPerPlayer: StorageMaxFilesPerPlayer,
		},
	)

	interceptors := connect.WithInterceptors(
		interceptor.Clock(),
		interceptor.Auth(signer),
	)

	mux := http.NewServeMux()
	authPath, authHandler := authconnect.NewAuthServiceHandler(gatewayauthhandler.New(authUC), interceptors)
	mux.Handle(authPath, authHandler)
	matchPath, matchHandler := matchconnect.NewMatchServiceHandler(gatewaymatchhandler.New(matchUC), interceptors)
	mux.Handle(matchPath, matchHandler)
	playerDataPath, playerDataHandler := storageconnect.NewPlayerDataStorageServiceHandler(gatewaystoragehandler.NewPlayerDataHandler(storageUC), interceptors)
	mux.Handle(playerDataPath, playerDataHandler)
	titlePath, titleHandler := storageconnect.NewTitleStorageServiceHandler(gatewaystoragehandler.NewTitleHandler(storageUC), interceptors)
	mux.Handle(titlePath, titleHandler)

	leaderboardUC := usecaseleaderboard.New(session, session, repository.NewLeaderboard(), map[string]domainleaderboard.Leaderboard{
		LeaderboardDesc: {ID: LeaderboardDesc, SortOrder: domainleaderboard.Descending},
		LeaderboardAsc:  {ID: LeaderboardAsc, SortOrder: domainleaderboard.Ascending},
	}, usecaseleaderboard.Config{DefaultLimit: 20, MaxLimit: 100, MaxRadius: 50})
	leaderboardPath, leaderboardHandler := leaderboardconnect.NewLeaderboardServiceHandler(gatewayleaderboardhandler.New(leaderboardUC), interceptors)
	mux.Handle(leaderboardPath, leaderboardHandler)

	socialUC := usecasesocial.New(session, session, repository.NewSocial())
	socialPath, socialHandler := socialconnect.NewSocialServiceHandler(gatewaysocialhandler.New(socialUC), interceptors)
	mux.Handle(socialPath, socialHandler)

	chatUC := usecasechat.New(session, session, repository.NewChat(), repository.NewGroup(), usecasechat.Config{DefaultLimit: 50, MaxLimit: 200})
	chatPath, chatHandler := chatconnect.NewChatServiceHandler(gatewaychathandler.New(chatUC), interceptors)
	mux.Handle(chatPath, chatHandler)

	groupUC := usecasegroup.New(session, session, repository.NewGroup())
	groupPath, groupHandler := groupconnect.NewGroupServiceHandler(gatewaygrouphandler.New(groupUC), interceptors)
	mux.Handle(groupPath, groupHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &Server{
		Auth:        authconnect.NewAuthServiceClient(srv.Client(), srv.URL),
		Match:       matchconnect.NewMatchServiceClient(srv.Client(), srv.URL),
		PlayerData:  storageconnect.NewPlayerDataStorageServiceClient(srv.Client(), srv.URL),
		Title:       storageconnect.NewTitleStorageServiceClient(srv.Client(), srv.URL),
		Leaderboard: leaderboardconnect.NewLeaderboardServiceClient(srv.Client(), srv.URL),
		Social:      socialconnect.NewSocialServiceClient(srv.Client(), srv.URL),
		Chat:        chatconnect.NewChatServiceClient(srv.Client(), srv.URL),
		Group:       groupconnect.NewGroupServiceClient(srv.Client(), srv.URL),
		Blob:        blob,
		session:     session,
		httpServer:  srv,
	}
}

// SeedTitleFile inserts title-file metadata directly, standing in for the operator-side publish path that clients cannot perform.
func (s *Server) SeedTitleFile(t *testing.T, f *domainstorage.File, tags []string) {
	t.Helper()
	if err := s.session.RW(t.Context(), func(ctx context.Context) error {
		return repository.NewTitleFile().SaveFile(ctx, f, tags)
	}); err != nil {
		t.Fatalf("testconnect: seed title file: %v", err)
	}
}

func Authorize[T any](req *connect.Request[T], accessToken string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+accessToken)
	return req
}
