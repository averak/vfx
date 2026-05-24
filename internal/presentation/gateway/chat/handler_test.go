package chat_test

import (
	"testing"
	"time"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	chatv1 "github.com/averak/vfx/gen/go/vfx/v1/chat"
	groupv1 "github.com/averak/vfx/gen/go/vfx/v1/group"
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

func TestSubscribeChannel_RejectsNonMember(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "sub-nm-owner")
	outsider := join(t, srv, "sub-nm-outsider")
	channelID := createGroup(t, srv, owner)

	stream, err := srv.Chat.SubscribeChannel(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SubscribeChannelRequest{ChannelId: channelID}), outsider.token))
	if err != nil {
		requireCode(t, err, connect.CodeFailedPrecondition)
		return
	}
	defer func() { _ = stream.Close() }()
	if stream.Receive() {
		t.Fatal("non-member received a message, want FailedPrecondition")
	}
	requireCode(t, stream.Err(), connect.CodeFailedPrecondition)
}

// A member's subscription delivers messages posted after it attaches.
//
// The client-side SubscribeChannel call only returns once the server flushes its first message, and the server attaches the subscriber asynchronously while handling the request.
// So the sender must run concurrently: it resends until one lands after the subscription attaches (earlier sends are simply missed, which is the intended "new only" contract).
func TestSubscribeChannel_StreamsLiveMessage(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "sub-live-owner")
	channelID := createGroup(t, srv, owner)

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = srv.Chat.SendChannelMessage(t.Context(),
					testconnect.Authorize(connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: "live"}), owner.token))
			}
		}
	}()
	defer close(stop)

	stream, err := srv.Chat.SubscribeChannel(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SubscribeChannelRequest{ChannelId: channelID}), owner.token))
	if err != nil {
		t.Fatalf("SubscribeChannel: %v", err)
	}
	defer func() { _ = stream.Close() }()

	if !stream.Receive() {
		t.Fatalf("stream produced no message: %v", stream.Err())
	}
	got := stream.Msg().GetMessage()
	if got.GetBody() != "live" || got.GetChannelId() != channelID {
		t.Errorf("message = %+v, want body=live channel=%s", got, channelID)
	}
}

func createGroup(t *testing.T, srv *testconnect.Server, owner member) string {
	t.Helper()
	resp, err := srv.Group.CreateGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.CreateGroupRequest{Name: "clan"}), owner.token))
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return resp.Msg.GetGroup().GetId()
}

func joinGroup(t *testing.T, srv *testconnect.Server, m member, groupID string) {
	t.Helper()
	if _, err := srv.Group.JoinGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.JoinGroupRequest{GroupId: groupID}), m.token)); err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}
}

func TestSendChannelMessage_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "ca-owner")
	channelID := createGroup(t, srv, owner)
	_, err := srv.Chat.SendChannelMessage(t.Context(), connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: "hi"}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

// A non-member cannot post to or read a channel.
func TestChannelMessage_RejectsNonMember(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "cm-owner")
	outsider := join(t, srv, "cm-outsider")
	channelID := createGroup(t, srv, owner)

	_, sendErr := srv.Chat.SendChannelMessage(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: "intrude"}), outsider.token))
	requireCode(t, sendErr, connect.CodeFailedPrecondition)

	_, listErr := srv.Chat.ListChannelMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListChannelMessagesRequest{ChannelId: channelID}), outsider.token))
	requireCode(t, listErr, connect.CodeFailedPrecondition)
}

func TestSendChannelMessage_RejectsBlank(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "cb-owner")
	channelID := createGroup(t, srv, owner)
	_, err := srv.Chat.SendChannelMessage(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: "   "}), owner.token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

// Every member sees the same channel history, newest-first, with the right sender.
func TestChannelMessages_History(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "ch-owner")
	other := join(t, srv, "ch-other")
	channelID := createGroup(t, srv, owner)
	joinGroup(t, srv, other, channelID)

	send := func(from member, body string) {
		t.Helper()
		if _, err := srv.Chat.SendChannelMessage(t.Context(),
			testconnect.Authorize(connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: body}), from.token)); err != nil {
			t.Fatalf("SendChannelMessage: %v", err)
		}
	}
	send(owner, "first")
	send(other, "second")
	send(owner, "third")

	resp, err := srv.Chat.ListChannelMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListChannelMessagesRequest{ChannelId: channelID}), other.token))
	if err != nil {
		t.Fatalf("ListChannelMessages: %v", err)
	}
	msgs := resp.Msg.GetMessages()
	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3", len(msgs))
	}
	if msgs[0].GetBody() != "third" || msgs[2].GetBody() != "first" {
		t.Errorf("order wrong: %q ... %q", msgs[0].GetBody(), msgs[2].GetBody())
	}
	if msgs[1].GetBody() != "second" || msgs[1].GetSenderId() != other.id || msgs[1].GetChannelId() != channelID {
		t.Errorf("message fields wrong: %+v", msgs[1])
	}
}

func TestChannelMessages_Pagination(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "cp-owner")
	channelID := createGroup(t, srv, owner)
	for _, body := range []string{"m1", "m2", "m3", "m4", "m5"} {
		if _, err := srv.Chat.SendChannelMessage(t.Context(),
			testconnect.Authorize(connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: body}), owner.token)); err != nil {
			t.Fatalf("SendChannelMessage: %v", err)
		}
	}

	page1, err := srv.Chat.ListChannelMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListChannelMessagesRequest{ChannelId: channelID, Limit: 2}), owner.token))
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Msg.GetMessages()) != 2 || page1.Msg.GetMessages()[0].GetBody() != "m5" {
		t.Fatalf("page1 = %+v, want [m5, m4]", page1.Msg.GetMessages())
	}

	oldest := page1.Msg.GetMessages()[1].GetSentAt()
	page2, err := srv.Chat.ListChannelMessages(t.Context(),
		testconnect.Authorize(connect.NewRequest(&chatv1.ListChannelMessagesRequest{ChannelId: channelID, Limit: 2, Before: oldest}), owner.token))
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Msg.GetMessages()) != 2 || page2.Msg.GetMessages()[0].GetBody() != "m3" {
		t.Errorf("page2 = %+v, want [m3, m2]", page2.Msg.GetMessages())
	}
}
