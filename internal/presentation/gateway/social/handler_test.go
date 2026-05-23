package social_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	socialv1 "github.com/averak/vfx/gen/go/vfx/v1/social"
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

func send(t *testing.T, srv *testconnect.Server, from member, toID string) *socialv1.SendFriendRequestResponse {
	t.Helper()
	resp, err := srv.Social.SendFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: toID}), from.token))
	if err != nil {
		t.Fatalf("SendFriendRequest: %v", err)
	}
	return resp.Msg
}

func friendIDs(t *testing.T, srv *testconnect.Server, m member) []string {
	t.Helper()
	resp, err := srv.Social.ListFriends(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.ListFriendsRequest{}), m.token))
	if err != nil {
		t.Fatalf("ListFriends: %v", err)
	}
	ids := make([]string, len(resp.Msg.GetFriends()))
	for i, f := range resp.Msg.GetFriends() {
		ids[i] = f.GetPlayerId()
	}
	return ids
}

func TestSendFriendRequest_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Social.SendFriendRequest(t.Context(), connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: join(t, srv, "x").id}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestSendFriendRequest_RejectsSelf(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "self")
	_, err := srv.Social.SendFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: a.id}), a.token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

// The full request/accept flow: pending shows on both sides, then accepting makes them friends.
func TestFriendRequest_AcceptFlow(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "a")
	b := join(t, srv, "b")

	if got := send(t, srv, a, b.id).GetStatus(); got != socialv1.RequestStatus_REQUEST_STATUS_PENDING {
		t.Fatalf("status = %v, want PENDING", got)
	}

	incoming, err := srv.Social.ListIncomingRequests(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.ListIncomingRequestsRequest{}), b.token))
	if err != nil {
		t.Fatalf("ListIncomingRequests: %v", err)
	}
	if len(incoming.Msg.GetRequests()) != 1 || incoming.Msg.GetRequests()[0].GetPlayerId() != a.id {
		t.Fatalf("b's incoming = %+v, want [a]", incoming.Msg.GetRequests())
	}

	if _, err := srv.Social.AcceptFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.AcceptFriendRequestRequest{RequesterPlayerId: a.id}), b.token)); err != nil {
		t.Fatalf("AcceptFriendRequest: %v", err)
	}

	if ids := friendIDs(t, srv, a); len(ids) != 1 || ids[0] != b.id {
		t.Errorf("a's friends = %v, want [b]", ids)
	}
	if ids := friendIDs(t, srv, b); len(ids) != 1 || ids[0] != a.id {
		t.Errorf("b's friends = %v, want [a]", ids)
	}
}

// A reciprocal request forms the friendship immediately.
func TestFriendRequest_MutualAutoAccepts(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ma")
	b := join(t, srv, "mb")

	send(t, srv, a, b.id)
	if got := send(t, srv, b, a.id).GetStatus(); got != socialv1.RequestStatus_REQUEST_STATUS_ACCEPTED {
		t.Fatalf("reciprocal status = %v, want ACCEPTED", got)
	}
	if ids := friendIDs(t, srv, a); len(ids) != 1 || ids[0] != b.id {
		t.Errorf("a's friends = %v, want [b]", ids)
	}
}

func TestSendFriendRequest_AlreadyFriends(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "fa")
	b := join(t, srv, "fb")
	send(t, srv, a, b.id)
	send(t, srv, b, a.id) // mutual -> friends

	_, err := srv.Social.SendFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: b.id}), a.token))
	requireCode(t, err, connect.CodeAlreadyExists)
}

func TestAcceptFriendRequest_NotFound(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "na")
	b := join(t, srv, "nb")
	_, err := srv.Social.AcceptFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.AcceptFriendRequestRequest{RequesterPlayerId: a.id}), b.token))
	requireCode(t, err, connect.CodeNotFound)
}

func TestRemoveFriend(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ra")
	b := join(t, srv, "rb")
	send(t, srv, a, b.id)
	send(t, srv, b, a.id) // friends

	if _, err := srv.Social.RemoveFriend(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.RemoveFriendRequest{FriendPlayerId: b.id}), a.token)); err != nil {
		t.Fatalf("RemoveFriend: %v", err)
	}
	if ids := friendIDs(t, srv, a); len(ids) != 0 {
		t.Errorf("a still has friends after removal: %v", ids)
	}

	// Removing a non-friend is NotFound.
	_, err := srv.Social.RemoveFriend(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.RemoveFriendRequest{FriendPlayerId: b.id}), a.token))
	requireCode(t, err, connect.CodeNotFound)
}

// Blocking severs an existing friendship and bars new requests; both are visible/enforced afterwards.
func TestBlock_SeversFriendshipAndBarsRequests(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ba")
	b := join(t, srv, "bb")
	send(t, srv, a, b.id)
	send(t, srv, b, a.id) // mutual -> friends

	if _, err := srv.Social.BlockPlayer(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.BlockPlayerRequest{PlayerId: b.id}), a.token)); err != nil {
		t.Fatalf("BlockPlayer: %v", err)
	}

	// Friendship is gone for both.
	if ids := friendIDs(t, srv, a); len(ids) != 0 {
		t.Errorf("blocker still has the friend: %v", ids)
	}
	if ids := friendIDs(t, srv, b); len(ids) != 0 {
		t.Errorf("blocked player still has the friend: %v", ids)
	}

	// b is listed as blocked by a.
	listed, err := srv.Social.ListBlocked(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.ListBlockedRequest{}), a.token))
	if err != nil {
		t.Fatalf("ListBlocked: %v", err)
	}
	if len(listed.Msg.GetBlocked()) != 1 || listed.Msg.GetBlocked()[0].GetPlayerId() != b.id {
		t.Fatalf("ListBlocked = %+v, want [b]", listed.Msg.GetBlocked())
	}

	// A new request either way is refused while the block stands.
	_, aToB := srv.Social.SendFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: b.id}), a.token))
	requireCode(t, aToB, connect.CodeFailedPrecondition)
	_, bToA := srv.Social.SendFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: a.id}), b.token))
	requireCode(t, bToA, connect.CodeFailedPrecondition)
}

// Block is idempotent, and unblocking restores the ability to send requests.
func TestBlock_IdempotentAndUnblock(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ia")
	b := join(t, srv, "ib")

	block := func() error {
		_, err := srv.Social.BlockPlayer(t.Context(),
			testconnect.Authorize(connect.NewRequest(&socialv1.BlockPlayerRequest{PlayerId: b.id}), a.token))
		return err
	}
	if err := block(); err != nil {
		t.Fatalf("first block: %v", err)
	}
	if err := block(); err != nil {
		t.Fatalf("second block (must be idempotent): %v", err)
	}

	if _, err := srv.Social.UnblockPlayer(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.UnblockPlayerRequest{PlayerId: b.id}), a.token)); err != nil {
		t.Fatalf("UnblockPlayer: %v", err)
	}
	// Unblock is idempotent too.
	if _, err := srv.Social.UnblockPlayer(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.UnblockPlayerRequest{PlayerId: b.id}), a.token)); err != nil {
		t.Fatalf("second unblock (must be idempotent): %v", err)
	}

	// After unblocking, a request goes through again.
	if status := send(t, srv, a, b.id).GetStatus(); status != socialv1.RequestStatus_REQUEST_STATUS_PENDING {
		t.Errorf("after unblock, send status = %v, want PENDING", status)
	}
}

func TestBlockPlayer_RejectsSelf(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "selfblock")
	_, err := srv.Social.BlockPlayer(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.BlockPlayerRequest{PlayerId: a.id}), a.token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestCancelFriendRequest(t *testing.T) {
	srv := testconnect.New(t)
	a := join(t, srv, "ca")
	b := join(t, srv, "cb")
	send(t, srv, a, b.id)

	if _, err := srv.Social.CancelFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.CancelFriendRequestRequest{AddresseePlayerId: b.id}), a.token)); err != nil {
		t.Fatalf("CancelFriendRequest: %v", err)
	}
	// Cancelling again is NotFound.
	_, err := srv.Social.CancelFriendRequest(t.Context(),
		testconnect.Authorize(connect.NewRequest(&socialv1.CancelFriendRequestRequest{AddresseePlayerId: b.id}), a.token))
	requireCode(t, err, connect.CodeNotFound)
}
