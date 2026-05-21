package vfxclient

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
)

// Session is a live WebTransport connection to a room. Inbound frames
// arrive on the channel returned by Frames; outbound input goes through
// SendInput.
type Session struct {
	wt     *webtransport.Session
	frames chan *realtimev1.Frame
	cancel context.CancelFunc
}

// SessionOption customises how a Session dials.
type SessionOption func(*sessionConfig)

type sessionConfig struct {
	insecureSkipVerify bool
}

// WithInsecureSkipVerify disables TLS verification. Use only against a
// self-signed development server.
func WithInsecureSkipVerify() SessionOption {
	return func(c *sessionConfig) { c.insecureSkipVerify = true }
}

// Connect opens a WebTransport session to the matched room. The caller
// owns the returned Session and must Close it.
func (m *Match) Connect(ctx context.Context, opts ...SessionOption) (*Session, error) {
	cfg := &sessionConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	matchID, err := matchIDFromToken(m.SessionToken)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: %w", err)
	}

	dialer := &webtransport.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.insecureSkipVerify, //nolint:gosec // opt-in for dev self-signed certs.
			MinVersion:         tls.VersionTLS13,
		},
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
	}
	u := url.URL{Scheme: "https", Host: m.Endpoint, Path: "/room/" + matchID}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+m.SessionToken)

	// The http.Response body wraps the upgrade stream; closing it would
	// tear down the session, so it is deliberately left open.
	_, wt, err := dialer.Dial(ctx, u.String(), header) //nolint:bodyclose // body wraps the WT session.
	if err != nil {
		return nil, fmt.Errorf("vfxclient: webtransport dial: %w", err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s := &Session{
		wt:     wt,
		frames: make(chan *realtimev1.Frame, 16),
		cancel: cancel,
	}
	go s.readLoop(loopCtx)
	return s, nil
}

// Frames returns the channel of inbound frames. It is closed when the
// session ends.
func (s *Session) Frames() <-chan *realtimev1.Frame { return s.frames }

// SendInput sends a PlayerInput frame as an unreliable datagram.
func (s *Session) SendInput(tick uint32, payload []byte) error {
	frame := &realtimev1.Frame{
		Body: &realtimev1.Frame_Input{
			Input: &realtimev1.PlayerInput{Tick: tick, Payload: payload},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("vfxclient: marshal frame: %w", err)
	}
	if err := s.wt.SendDatagram(data); err != nil {
		return fmt.Errorf("vfxclient: send datagram: %w", err)
	}
	return nil
}

// Close terminates the session and stops the read loop.
func (s *Session) Close() error {
	s.cancel()
	//nolint:errcheck // best-effort cleanup
	_ = s.wt.CloseWithError(0, "client closed")
	return nil
}

func (s *Session) readLoop(ctx context.Context) {
	defer close(s.frames)
	for {
		raw, err := s.wt.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		var frame realtimev1.Frame
		if err := proto.Unmarshal(raw, &frame); err != nil {
			continue
		}
		select {
		case s.frames <- &frame:
		case <-ctx.Done():
			return
		}
	}
}

// matchIDFromToken pulls the match id ("mid" claim) out of the session
// token's JWT payload. The client does not verify the signature — the
// room does that on accept — it only needs the id to build the URL.
func matchIDFromToken(tokenStr string) (string, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return "", errors.New("malformed session token")
	}
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode token payload: %w", err)
	}
	var claims struct {
		MID string `json:"mid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse token claims: %w", err)
	}
	if claims.MID == "" {
		return "", errors.New("session token has no match id")
	}
	return claims.MID, nil
}

func decodeJWTSegment(seg string) ([]byte, error) {
	if pad := len(seg) % 4; pad != 0 {
		seg += strings.Repeat("=", 4-pad)
	}
	return base64.URLEncoding.DecodeString(seg)
}
