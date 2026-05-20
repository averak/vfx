// Package room is the room daemon's HTTP entry point.
//
// The daemon serves a single path, /room/{match_id}, over WebTransport.
// A client connects with the session token issued by the gateway in
// the Authorization header. After the handshake, datagrams and bidi
// streams flow into the match goroutine for that match id.
//
// Phase 1 verifies tokens and accepts connections but does not yet run
// a plugin tick loop; that lands in a subsequent commit together with
// the wazero integration.
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

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"

	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
)

// Server hosts the WebTransport endpoint for a single room daemon.
type Server struct {
	cfg    *config.Room
	signer *token.Signer
	logger *slog.Logger
	wt     *webtransport.Server
}

// NewServer wires the WebTransport server.
func NewServer(cfg *config.Room, signer *token.Signer, logger *slog.Logger) (*Server, error) {
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("room: load tls: %w", err)
	}

	s := &Server{
		cfg:    cfg,
		signer: signer,
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/room/", s.handleRoom)

	s.wt = &webtransport.Server{
		H3: &http3.Server{
			Addr:    cfg.ListenAddr,
			Handler: mux,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				NextProtos:   []string{"h3"},
				MinVersion:   tls.VersionTLS13,
			},
			QUICConfig: &quic.Config{
				HandshakeIdleTimeout: cfg.HandshakeTimeout,
			},
		},
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
		closeCtx, cancel := context.WithTimeout(context.Background(), s.cfg.HandshakeTimeout)
		defer cancel()
		//nolint:errcheck // Close errors at shutdown are not actionable.
		_ = s.wt.Close()
		_ = closeCtx // reserved for richer shutdown later
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("room: serve: %w", err)
		}
		return nil
	}
}

// handleRoom accepts the WebTransport handshake and dispatches the
// session. The URL path encodes the match id; the Authorization header
// carries the session token.
func (s *Server) handleRoom(w http.ResponseWriter, r *http.Request) {
	matchID := strings.TrimPrefix(r.URL.Path, "/room/")
	if matchID == "" {
		http.Error(w, "match id required", http.StatusBadRequest)
		return
	}

	claims, err := s.authenticate(r)
	if err != nil {
		s.logger.Warn("room rejected unauthenticated connection", "err", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if claims.MatchID != matchID {
		s.logger.Warn("room rejected mismatched session token", "url_match", matchID, "token_match", claims.MatchID)
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

	// Phase 1: echo datagrams back so we can verify the transport
	// end-to-end. The real handler attaches to the match goroutine and
	// forwards through to the plugin.
	s.echoLoop(r.Context(), session, matchID, claims.PlayerID.String())
}

func (s *Server) authenticate(r *http.Request) (*token.SessionClaims, error) {
	raw := r.Header.Get("Authorization")
	rawToken, ok := strings.CutPrefix(raw, "Bearer ")
	if !ok || rawToken == "" {
		return nil, errors.New("missing bearer token")
	}
	return s.signer.VerifySession(rawToken)
}

// echoLoop is a placeholder that reads datagrams and writes them back.
// Replaced by the plugin-driven handler in the next iteration.
func (s *Server) echoLoop(ctx context.Context, session *webtransport.Session, matchID, playerID string) {
	for {
		payload, err := session.ReceiveDatagram(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				s.logger.Info("room datagram receive ended",
					"match_id", matchID, "player_id", playerID, "err", err)
			}
			return
		}
		if err := session.SendDatagram(payload); err != nil {
			s.logger.Warn("room datagram send failed",
				"match_id", matchID, "player_id", playerID, "err", err)
			return
		}
	}
}
