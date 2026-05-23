package chat_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	chatv1 "github.com/averak/vfx/gen/go/vfx/v1/chat"
	"github.com/averak/vfx/internal/testutils/testconnect"
)

type member struct {
	token string
	id    string
}

func join(t *testing.T, srv *testconnect.Server, device string) member {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{Anonymous: &authv1.AnonymousCredential{DeviceId: &device}},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return member{token: resp.Msg.GetAccessToken(), id: resp.Msg.GetPlayer().GetId()}
}

func requireCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want %v", want)
	}
	if got := connect.CodeOf(err); got != want {
		t.Errorf("code = %v, want %v", got, want)
	}
}

func dm(t *testing.T, srv *testconnect.Server, from member, toID, body string) {
	t.Helper()
	if _, err := srv.Chat.SendDirectMessage(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SendDirectMessageRequest{RecipientId: toID, Body: body}), from.token)); err != nil {
		t.Fatalf("SendDirectMessage: %v", err)
	}
}

func TestSendDirectMessage_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Chat.SendDirectMessage(t.Context(), connect.NewRequest(&chatv1.SendDirectMessageRequest{RecipientId: join(t, srv, "x").id, Body: "hi"}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestSendDirectMessage_RejectsSelfAndBlank(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "a")
	b := join(t, srv, "b")

	_, selfErr := srv.Chat.SendDirectMessage(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SendDirectMessageRequest{RecipientId: a.id, Body: "hi"}), a.token))
	requireCode(t, selfErr, connect.CodeInvalidArgument)

	_, blankErr := srv.Chat.SendDirectMessage(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SendDirectMessageRequest{RecipientId: b.id, Body: "   "}), a.token))
	requireCode(t, blankErr, connect.CodeInvalidArgument)
}

// Both participants see the same conversation, newest-first, and the message carries the right sender/recipient.
func TestDirectMessages_ConversationHistory(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ha")
	b := join(t, srv, "hb")

	dm(t, srv, a, b.id, "first")
	dm(t, srv, b, a.id, "second")
	dm(t, srv, a, b.id, "third")

	resp, err := srv.Chat.ListDirectMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListDirectMessagesRequest{OtherPlayerId: b.id}), a.token))
	if err != nil {
		t.Fatalf("ListDirectMessages: %v", err)
	}
	msgs := resp.Msg.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	// Newest-first.
	if msgs[0].GetBody() != "third" || msgs[2].GetBody() != "first" {
		t.Errorf("order wrong: %q ... %q", msgs[0].GetBody(), msgs[2].GetBody())
	}
	// b's reply records b as sender, a as recipient.
	if msgs[1].GetBody() != "second" || msgs[1].GetSenderId() != b.id || msgs[1].GetRecipientId() != a.id {
		t.Errorf("reply sender/recipient wrong: %+v", msgs[1])
	}

	// b sees the same conversation.
	bResp, err := srv.Chat.ListDirectMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListDirectMessagesRequest{OtherPlayerId: a.id}), b.token))
	if err != nil {
		t.Fatalf("b ListDirectMessages: %v", err)
	}
	if len(bResp.Msg.GetMessages()) != 3 {
		t.Errorf("b sees %d messages, want 3", len(bResp.Msg.GetMessages()))
	}
}

// The limit caps the page, and the before cursor pages back to older messages.
func TestDirectMessages_Pagination(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "pa")
	b := join(t, srv, "pb")
	for _, body := range []string{"m1", "m2", "m3", "m4", "m5"} {
		dm(t, srv, a, b.id, body)
	}

	page1, err := srv.Chat.ListDirectMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListDirectMessagesRequest{OtherPlayerId: b.id, Limit: 2}), a.token))
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Msg.GetMessages()) != 2 || page1.Msg.GetMessages()[0].GetBody() != "m5" {
		t.Fatalf("page1 = %+v, want [m5, m4]", page1.Msg.GetMessages())
	}

	oldest := page1.Msg.GetMessages()[1].GetSentAt()
	page2, err := srv.Chat.ListDirectMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListDirectMessagesRequest{OtherPlayerId: b.id, Limit: 2, Before: oldest}), a.token))
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Msg.GetMessages()) != 2 || page2.Msg.GetMessages()[0].GetBody() != "m3" {
		t.Errorf("page2 = %+v, want [m3, m2]", page2.Msg.GetMessages())
	}
}
