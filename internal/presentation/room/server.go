// Package room is the room daemon's HTTP entry point.
//
// The daemon serves a single path, /room/{match_id}, over WebTransport.
// After the upgrade the client proves itself with a ClientHello frame carrying the gateway-issued session token; datagrams then flow into the match orchestrator for that match id.
package room

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

// tracer is the room presentation layer's instrumentation scope.
// It resolves through the global tracer provider, so spans are no-ops until tracing.Setup installs an exporter.
var tracer = otel.Tracer("github.com/averak/vfx/internal/presentation/room")

type Server struct {
	cfg     *config.Room
	signer  *token.Signer
	manager *usecaseroom.Manager
	logger  *slog.Logger
	wt      *webtransport.Server
}

func NewServer(cfg *config.Room, signer *token.Signer, manager *usecaseroom.Manager, logger *slog.Logger) (*Server, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("room: load tls: %w", err)
	}

	s := &Server{
		cfg:     cfg,
		signer:  signer,
		manager: manager,
		logger:  logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/room/", s.handleRoom)

	h3 := &http3.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h3"},
			MinVersion:   tls.VersionTLS13,
		},
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout:             cfg.HandshakeTimeout,
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
	}
	// Hands H3 the SETTINGS frame, datagram toggle, and ConnContext hook that webtransport.Server.Upgrade needs to find the QUIC conn.
	webtransport.ConfigureHTTP3Server(h3)

	s.wt = &webtransport.Server{
		H3: h3,
		CheckOrigin: func(_ *http.Request) bool {
			// Accept any origin; a deployment restricts this to the known client domains once they are configured.
			return true
		},
	}

	return s, nil
}

// ListenAndServe runs the WebTransport server until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("room listening", "addr", s.cfg.ListenAddr)
		errCh <- s.wt.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		//nolint:errcheck // Close errors at shutdown are not actionable.
		_ = s.wt.Close()
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("room: serve: %w", err)
		}
		return nil
	}
}

// handleRoom accepts the WebTransport handshake and bridges the player to the match orchestrator.
// The URL path encodes the match id; the Authorization header carries the session token.
func (s *Server) handleRoom(w http.ResponseWriter, r *http.Request) {
	matchIDStr := strings.TrimPrefix(r.URL.Path, "/room/")
	if matchIDStr == "" {
		http.Error(w, "match id required", http.StatusBadRequest)
		return
	}
	matchID, err := uuid.Parse(matchIDStr)
	if err != nil {
		http.Error(w, "invalid match id", http.StatusBadRequest)
		return
	}

	session, err := s.wt.Upgrade(w, r)
	if err != nil {
		s.logger.Error("room upgrade failed", "err", err)
		return
	}
	//nolint:errcheck // Best-effort cleanup; client already disconnected.
	defer func() { _ = session.CloseWithError(0, "session ended") }()

	// A browser cannot set the Authorization header on a WebTransport CONNECT, so the client authenticates after the upgrade with a ClientHello frame instead.
	claims, err := s.authenticate(session)
	if err != nil {
		s.logger.Warn("room rejected unauthenticated connection", "err", err)
		return
	}
	if claims.MatchID != matchIDStr {
		s.logger.Warn("room rejected mismatched session token",
			"url_match", matchIDStr, "token_match", claims.MatchID)
		return
	}

	s.logger.Info("room session opened",
		"match_id", matchID,
		"player_id", claims.PlayerID,
	)

	roster := make([]uuid.UUID, 0, len(claims.MatchPlayers))
	for _, raw := range claims.MatchPlayers {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			s.logger.Warn("room: invalid player id in token", "value", raw)
			continue
		}
		roster = append(roster, id)
	}
	if len(roster) == 0 {
		roster = []uuid.UUID{claims.PlayerID}
	}

	// The HTTP request context fires Done as soon as net/http considers the response sent, which for a WebTransport Upgrade happens immediately.
	// The session keeps its own context that mirrors the actual transport lifetime; use that everywhere downstream.
	sessionCtx := session.Context()

	// One span per connected session, covering its whole lifetime.
	// Started on sessionCtx so the match-creation span nests under it.
	sessionCtx, span := tracer.Start(sessionCtx, "room.session", trace.WithAttributes(
		attribute.String("vfx.match_id", matchID.String()),
		attribute.String("vfx.player_id", claims.PlayerID.String()),
	))
	defer span.End()

	match, err := s.manager.FindOrCreate(sessionCtx, matchID, roster)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "match unavailable")
		s.logger.Error("room: match unavailable", "err", err)
		return
	}

	playerIO := newPlayerSession(claims.PlayerID, matchID, session, s.logger, s.cfg.DatagramMaxBytes)
	if err := match.Join(claims.PlayerID, playerIO); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "join failed")
		s.logger.Warn("room: join failed", "err", err)
		return
	}
	defer match.Leave(claims.PlayerID, "disconnected")

	playerIO.readLoop(sessionCtx, match)
}

// authenticate reads the client's ClientHello from the first reliable stream and verifies the session token it carries.
// HandshakeTimeout bounds the wait so an idle or hostile connection cannot hold a slot open.
func (s *Server) authenticate(session *webtransport.Session) (*token.SessionClaims, error) {
	ctx, cancel := context.WithTimeout(session.Context(), s.cfg.HandshakeTimeout)
	defer cancel()

	stream, err := session.AcceptUniStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("accept hello stream: %w", err)
	}
	raw, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read hello: %w", err)
	}
	var frame realtimev1.Frame
	if err := proto.Unmarshal(raw, &frame); err != nil {
		return nil, fmt.Errorf("unmarshal hello: %w", err)
	}
	hello := frame.GetHello()
	if hello == nil {
		return nil, errors.New("first frame was not a ClientHello")
	}
	return s.signer.VerifySession(hello.GetSessionToken())
}
