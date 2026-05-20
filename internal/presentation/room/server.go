// Package room is the room daemon's HTTP entry point.
//
// The daemon serves a single path, /room/{match_id}, over WebTransport.
// A client connects with the session token issued by the gateway in
// the Authorization header. After the handshake, datagrams flow into
// the match orchestrator for that match id.
package room

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

// Server hosts the WebTransport endpoint for a single room daemon.
type Server struct {
	cfg     *config.Room
	signer  *token.Signer
	manager *usecaseroom.Manager
	logger  *slog.Logger
	wt      *webtransport.Server
}

// NewServer wires the WebTransport server.
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
	// Hands H3 the SETTINGS frame, datagram toggle, and ConnContext hook
	// that webtransport.Server.Upgrade needs to find the QUIC conn.
	webtransport.ConfigureHTTP3Server(h3)

	s.wt = &webtransport.Server{
		H3: h3,
		CheckOrigin: func(_ *http.Request) bool {
			// Phase 1: accept any origin. Production deployments add a
			// list once the client domain is known.
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

// handleRoom accepts the WebTransport handshake and bridges the player
// to the match orchestrator. The URL path encodes the match id; the
// Authorization header carries the session token.
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

	claims, err := s.authenticate(r)
	if err != nil {
		s.logger.Warn("room rejected unauthenticated connection", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if claims.MatchID != matchIDStr {
		s.logger.Warn("room rejected mismatched session token",
			"url_match", matchIDStr, "token_match", claims.MatchID)
		http.Error(w, "match id mismatch", http.StatusForbidden)
		return
	}

	session, err := s.wt.Upgrade(w, r)
	if err != nil {
		s.logger.Error("room upgrade failed", "err", err)
		return
	}
	//nolint:errcheck // Best-effort cleanup; client already disconnected.
	defer func() { _ = session.CloseWithError(0, "session ended") }()

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

	match, err := s.manager.FindOrCreate(r.Context(), matchID, roster)
	if err != nil {
		s.logger.Error("room: match unavailable", "err", err)
		return
	}

	playerIO := newPlayerSession(claims.PlayerID, matchID, session, s.logger)
	if err := match.Join(claims.PlayerID, playerIO); err != nil {
		s.logger.Warn("room: join failed", "err", err)
		return
	}
	defer match.Leave(claims.PlayerID, "disconnected")

	playerIO.readLoop(r.Context(), match)
}

func (s *Server) authenticate(r *http.Request) (*token.SessionClaims, error) {
	raw := r.Header.Get("Authorization")
	rawToken, ok := strings.CutPrefix(raw, "Bearer ")
	if !ok || rawToken == "" {
		return nil, errors.New("missing bearer token")
	}
	return s.signer.VerifySession(rawToken)
}
