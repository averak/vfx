package match_test

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	matchv1 "github.com/averak/vfx/gen/go/vfx/v1/match"
	"github.com/averak/vfx/internal/testutils/testconnect"
)

const gameModeRPS = "rps"

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want %v", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v", got, want)
	}
}

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

func TestWatchTicket_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)

	stream, err := srv.Match.WatchTicket(t.Context(), connect.NewRequest(&matchv1.WatchTicketRequest{
		TicketId: uuid.NewString(),
	}))
	if err != nil {
		// Some transports surface the auth error only on the first Receive; tolerate either.
		requireCode(t, err, connect.CodeUnauthenticated)
		return
	}
	defer func() { _ = stream.Close() }()
	if stream.Receive() {
		t.Fatal("WatchTicket without a token produced an event, want Unauthenticated")
	}
	requireCode(t, stream.Err(), connect.CodeUnauthenticated)
}

func TestCancelTicket_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Match.CancelTicket(t.Context(), connect.NewRequest(&matchv1.CancelTicketRequest{
		TicketId: uuid.NewString(),
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestCancelTicket_InvalidID(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "cancel-bad-id")

	_, err := srv.Match.CancelTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CancelTicketRequest{TicketId: "not-a-uuid"}), token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestCancelTicket_UnknownTicket(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "cancel-unknown")

	_, err := srv.Match.CancelTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CancelTicketRequest{TicketId: uuid.NewString()}), token))
	requireCode(t, err, connect.CodeNotFound)
}

func TestCancelTicket_CancelsQueuedTicket(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "cancel-ok")

	created, err := srv.Match.CreateTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CreateTicketRequest{GameMode: gameModeRPS}), token))
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if _, err := srv.Match.CancelTicket(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.CancelTicketRequest{TicketId: created.Msg.GetTicketId()}), token)); err != nil {
		t.Fatalf("CancelTicket: %v", err)
	}
}

func TestGetCurrentMatch_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Match.GetCurrentMatch(t.Context(), connect.NewRequest(&matchv1.GetCurrentMatchRequest{}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

// A player not in a match gets an empty response, not an error: it is the normal "nothing to reconnect to".
func TestGetCurrentMatch_NoneReturnsEmpty(t *testing.T) {
	srv := testconnect.New(t)
	token := login(t, srv, "no-match")

	resp, err := srv.Match.GetCurrentMatch(t.Context(),
		testconnect.Authorize(connect.NewRequest(&matchv1.GetCurrentMatchRequest{}), token))
	if err != nil {
		t.Fatalf("GetCurrentMatch: %v", err)
	}
	if resp.Msg.GetMatch() != nil {
		t.Errorf("GetCurrentMatch returned a match for a player not in one: %+v", resp.Msg.GetMatch())
	}
}
