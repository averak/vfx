package vfxclient

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/proto"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
)

// Session is a live WebTransport connection to a room.
// Inbound frames arrive on the channel returned by Frames, from both unreliable datagrams (small deltas) and reliable unidirectional streams (large snapshots); outbound input goes through SendInput.
type Session struct {
	wt     *webtransport.Session
	frames chan *realtimev1.Frame
	cancel context.CancelFunc
}

type SessionOption func(*sessionConfig)

type sessionConfig struct {
	insecureSkipVerify bool
}

// WithInsecureSkipVerify disables TLS verification.
// Use only against a self-signed development server.
func WithInsecureSkipVerify() SessionOption {
	return func(c *sessionConfig) { c.insecureSkipVerify = true }
}

// Connect opens a WebTransport session to the matched room.
// The caller owns the returned Session and must Close it.
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

	// The http.Response body wraps the upgrade stream; closing it would tear down the session, so it is deliberately left open.
	_, wt, err := dialer.Dial(ctx, u.String(), nil) //nolint:bodyclose // body wraps the WT session.
	if err != nil {
		return nil, fmt.Errorf("vfxclient: webtransport dial: %w", err)
	}

	// A browser cannot set the Authorization header on the CONNECT, so the token rides in a ClientHello sent before anything else.
	if err := sendHello(ctx, wt, m.SessionToken); err != nil {
		return nil, fmt.Errorf("vfxclient: %w", err)
	}

	loopCtx, cancel := context.WithCancel(ctx)
	s := &Session{
		wt:     wt,
		frames: make(chan *realtimev1.Frame, 16),
		cancel: cancel,
	}

	// Frames arrive on two transports; close the channel only once both readers have stopped so neither writes to a closed channel.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.datagramLoop(loopCtx) }()
	go func() { defer wg.Done(); s.streamLoop(loopCtx) }()
	go func() { wg.Wait(); close(s.frames) }()
	return s, nil
}

// sendHello writes the ClientHello over a fresh reliable stream and closes it, so the room reads exactly one frame and authenticates the session.
func sendHello(ctx context.Context, wt *webtransport.Session, sessionToken string) error {
	frame := &realtimev1.Frame{
		Body: &realtimev1.Frame_Hello{Hello: &realtimev1.ClientHello{SessionToken: sessionToken}},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal hello: %w", err)
	}
	stream, err := wt.OpenUniStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("open hello stream: %w", err)
	}
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("write hello: %w", err)
	}
	return stream.Close()
}

// Frames returns the channel of inbound frames.
// It is closed when the session ends.
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

// datagramLoop delivers unreliable datagram frames (small state deltas).
func (s *Session) datagramLoop(ctx context.Context) {
	for {
		raw, err := s.wt.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		s.deliver(ctx, raw)
	}
}

// streamLoop delivers frames sent over reliable unidirectional streams (large snapshots).
// Each stream carries exactly one frame.
func (s *Session) streamLoop(ctx context.Context) {
	for {
		stream, err := s.wt.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		raw, err := io.ReadAll(stream)
		if err != nil {
			continue
		}
		s.deliver(ctx, raw)
	}
}

func (s *Session) deliver(ctx context.Context, raw []byte) {
	var frame realtimev1.Frame
	if err := proto.Unmarshal(raw, &frame); err != nil {
		return
	}
	select {
	case s.frames <- &frame:
	case <-ctx.Done():
	}
}

// matchIDFromToken pulls the match id ("mid" claim) out of the session token's JWT payload.
// The client does not verify the signature (the room does that on accept); it only needs the id to build the URL.
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
