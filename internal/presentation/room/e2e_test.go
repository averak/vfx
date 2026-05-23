package room_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/testutils/testwt"
	vfxclient "github.com/averak/vfx/sdk/client/go"
)

// endAfterTicksFactory hosts a plugin that ends the match after a few ticks on its own, so an E2E test drives a full connect → play → game_ended cycle without depending on unreliable datagram input reaching the server.
type endAfterTicksFactory struct{}

func (endAfterTicksFactory) Name() string { return "testwt" }

func (endAfterTicksFactory) Create(context.Context) (plugin.Plugin, error) {
	return &endAfterTicksPlugin{}, nil
}

type endAfterTicksPlugin struct{ ticks int }

func (p *endAfterTicksPlugin) Init(context.Context, *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	return &pluginv1.InitResponse{TickRateHz: 20}, nil
}

func (p *endAfterTicksPlugin) OnTick(context.Context, *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	p.ticks++
	return &pluginv1.OnTickResponse{StateDelta: []byte("tick"), GameEnded: p.ticks >= 3}, nil
}

func (p *endAfterTicksPlugin) OnGameEnd(context.Context, *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	return &pluginv1.OnGameEndResponse{}, nil
}

func (p *endAfterTicksPlugin) Close() error { return nil }

// dial connects to the room with a short retry while the UDP listener binds.
func dial(ctx context.Context, t *testing.T, endpoint, sessionToken string) *vfxclient.Session {
	t.Helper()
	m := &vfxclient.Match{Endpoint: endpoint, SessionToken: sessionToken}
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := m.Connect(ctx, vfxclient.WithInsecureSkipVerify())
		if err == nil {
			return session
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Exercises the real WebTransport handshake and downstream data plane: a valid token authenticates over the ClientHello, the match ticks, and the game_ended SystemEvent reaches the client over a reliable stream.
func TestRoomE2E_ConnectPlayEnd(t *testing.T) {
	rm := testwt.New(t, endAfterTicksFactory{})
	matchID := uuid.New()
	playerID := uuid.New()
	tok, err := rm.Signer.SignSession(playerID, matchID.String(), []string{playerID.String()}, time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("SignSession: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session := dial(ctx, t, rm.Endpoint, tok)
	defer func() { _ = session.Close() }()

	// The room keeps the session open after a match ends (the client drives the disconnect), so success is observing the game_ended event, not the channel closing.
	deadline := time.After(15 * time.Second)
	for {
		select {
		case frame, ok := <-session.Frames():
			if !ok {
				t.Fatal("frame channel closed before game_ended")
			}
			if ev := frame.GetEvent(); ev != nil && ev.GetType() == "game_ended" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for game_ended")
		}
	}
}

// A token signed with the wrong secret fails the ClientHello check, so the room closes the session without ever delivering a frame.
func TestRoomE2E_RejectsForeignToken(t *testing.T) {
	rm := testwt.New(t, endAfterTicksFactory{})
	matchID := uuid.New()
	playerID := uuid.New()
	foreign := token.NewSigner("a-different-secret")
	tok, err := foreign.SignSession(playerID, matchID.String(), []string{playerID.String()}, time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("SignSession: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	session := dial(ctx, t, rm.Endpoint, tok)
	defer func() { _ = session.Close() }()

	// The client observes the rejection as the frame channel closing.
	// A clean network delivers the CONNECTION_CLOSE at once; if it is dropped, the QUIC idle timeout (~30s) still tears the session down, so the deadline sits above it.
	deadline := time.After(40 * time.Second)
	for {
		select {
		case frame, ok := <-session.Frames():
			if !ok {
				return // session closed by the room, as expected
			}
			if ev := frame.GetEvent(); ev != nil && ev.GetType() == "game_ended" {
				t.Fatal("a foreign token reached gameplay")
			}
		case <-deadline:
			t.Fatal("room did not close the session for a foreign token")
		}
	}
}
