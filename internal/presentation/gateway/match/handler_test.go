package match_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	matchv1 "github.com/averak/vfx/gen/go/vfx/v1/match"
	"github.com/averak/vfx/internal/testutils/testconnect"
)

const gameModeRPS = "rps"

func login(t *testing.T, srv *testconnect.Server, device string) string {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{DeviceId: &device},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp.Msg.GetAccessToken()
}

func TestCreateTicket_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)

	_, err := srv.Match.CreateTicket(t.Context(), connect.NewRequest(&matchv1.CreateTicketRequest{
		GameMode: gameModeRPS,
	}))
	if err == nil {
		t.Fatal("CreateTicket without a token succeeded, want Unauthenticated")
	}
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

func TestCreateTicket_RequiresGameMode(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "mode-device")

	_, err := srv.Match.CreateTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CreateTicketRequest{}), token))
	if err == nil {
		t.Fatal("CreateTicket with no game_mode succeeded, want InvalidArgument")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestCreateTicket_ReturnsTicketID(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "ticket-device")

	resp, err := srv.Match.CreateTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CreateTicketRequest{GameMode: gameModeRPS}), token))
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if resp.Msg.GetTicketId() == "" {
		t.Error("empty ticket id")
	}
}

func TestWatchTicket_StreamsQueued(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "watch-device")

	created, err := srv.Match.CreateTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CreateTicketRequest{GameMode: gameModeRPS}), token))
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}

	stream, err := srv.Match.WatchTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.WatchTicketRequest{
			TicketId: created.Msg.GetTicketId(),
		}), token))
	if err != nil {
		t.Fatalf("WatchTicket: %v", err)
	}
	defer func() { _ = stream.Close() }()

	if !stream.Receive() {
		t.Fatalf("stream produced no events: %v", stream.Err())
	}
	if _, ok := stream.Msg().GetEvent().(*matchv1.WatchTicketResponse_Queued); !ok {
		t.Errorf("first event = %T, want Queued", stream.Msg().GetEvent())
	}
}
