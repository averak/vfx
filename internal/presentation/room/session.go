package room

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

// playerSession bridges one WebTransport session and the Match it
// belongs to. It satisfies usecaseroom.PlayerIO so the orchestrator
// can call SendFrame without knowing anything about WebTransport.
type playerSession struct {
	playerID uuid.UUID
	matchID  uuid.UUID
	session  *webtransport.Session
	logger   *slog.Logger
	closed   atomic.Bool
}

func newPlayerSession(playerID, matchID uuid.UUID, session *webtransport.Session, logger *slog.Logger) *playerSession {
	return &playerSession{
		playerID: playerID,
		matchID:  matchID,
		session:  session,
		logger:   logger,
	}
}

// SendFrame marshals frame and writes it as a datagram. WebTransport
// datagrams are unreliable; that is what we want for high-frequency
// state. Larger Frames (snapshots) get a stream once the orchestrator
// learns to ask for one.
func (s *playerSession) SendFrame(frame *realtimev1.Frame) error {
	if s.closed.Load() {
		return errors.New("room: session closed")
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("room: marshal frame: %w", err)
	}
	if err := s.session.SendDatagram(data); err != nil {
		return fmt.Errorf("room: send datagram: %w", err)
	}
	return nil
}

// Close marks the session as closed; the WebTransport teardown happens
// in the handler once readLoop returns.
func (s *playerSession) Close() { s.closed.Store(true) }

// readLoop pumps datagrams from the WebTransport session into the
// match as PlayerInputs. It returns when the underlying transport
// closes or ctx is cancelled.
func (s *playerSession) readLoop(ctx context.Context, match *usecaseroom.Match) {
	for {
		raw, err := s.session.ReceiveDatagram(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				s.logger.Debug("room session read ended",
					"match_id", s.matchID, "player_id", s.playerID, "err", err)
			}
			return
		}

		var frame realtimev1.Frame
		if err := proto.Unmarshal(raw, &frame); err != nil {
			s.logger.Warn("room: malformed frame", "err", err)
			continue
		}
		input := frame.GetInput()
		if input == nil {
			// Phase 1 only consumes PlayerInput from clients; other
			// kinds (heartbeats) are accepted silently.
			continue
		}
		match.SubmitInput(s.playerID, input.GetTick(), input.GetPayload())
	}
}
