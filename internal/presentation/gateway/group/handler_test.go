package group_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
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

func createGroup(t *testing.T, srv *testconnect.Server, owner member) string {
	t.Helper()
	resp, err := srv.Group.CreateGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.CreateGroupRequest{Name: "clan"}), owner.token))
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return resp.Msg.GetGroup().GetId()
}

func memberIDs(t *testing.T, srv *testconnect.Server, m member, groupID string) []string {
	t.Helper()
	resp, err := srv.Group.ListMembers(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.ListMembersRequest{GroupId: groupID}), m.token))
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	ids := make([]string, len(resp.Msg.GetMembers()))
	for i, x := range resp.Msg.GetMembers() {
		ids[i] = x.GetPlayerId()
	}
	return ids
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestCreateGroup_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Group.CreateGroup(t.Context(), connect.NewRequest(&groupv1.CreateGroupRequest{Name: "clan"}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestCreateGroup_RejectsBlankName(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "owner")
	_, err := srv.Group.CreateGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.CreateGroupRequest{Name: "   "}), owner.token))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestGroup_CreateJoinLeave(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "g-owner")
	other := join(t, srv, "g-other")

	gid := createGroup(t, srv, owner)

	// Owner is the sole initial member and sees the group.
	if ids := memberIDs(t, srv, owner, gid); len(ids) != 1 || ids[0] != owner.id {
		t.Fatalf("initial members = %v, want [owner]", ids)
	}
	mine, err := srv.Group.ListMyGroups(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.ListMyGroupsRequest{}), owner.token))
	if err != nil {
		t.Fatalf("ListMyGroups: %v", err)
	}
	if len(mine.Msg.GetGroups()) != 1 || mine.Msg.GetGroups()[0].GetId() != gid {
		t.Fatalf("owner's groups = %+v", mine.Msg.GetGroups())
	}

	// Another player joins (idempotently).
	for range 2 {
		if _, err := srv.Group.JoinGroup(t.Context(),
			testconnect.Authorize(connect.NewRequest(&groupv1.JoinGroupRequest{GroupId: gid}), other.token)); err != nil {
			t.Fatalf("JoinGroup: %v", err)
		}
	}
	if ids := memberIDs(t, srv, owner, gid); len(ids) != 2 || !contains(ids, other.id) {
		t.Errorf("members after join = %v, want owner+other", ids)
	}

	// Non-owner leaves.
	if _, err := srv.Group.LeaveGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.LeaveGroupRequest{GroupId: gid}), other.token)); err != nil {
		t.Fatalf("LeaveGroup: %v", err)
	}
	if ids := memberIDs(t, srv, owner, gid); contains(ids, other.id) {
		t.Errorf("member still present after leaving: %v", ids)
	}
}

func TestJoinGroup_UnknownIsNotFound(t *testing.T) {
	srv := testconnect.New(t)
	m := join(t, srv, "joiner")
	_, err := srv.Group.JoinGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.JoinGroupRequest{GroupId: "00000000-0000-0000-0000-000000000000"}), m.token))
	requireCode(t, err, connect.CodeNotFound)
}

func TestLeaveGroup_OwnerCannotLeave(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "lo")
	gid := createGroup(t, srv, owner)
	_, err := srv.Group.LeaveGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.LeaveGroupRequest{GroupId: gid}), owner.token))
	requireCode(t, err, connect.CodeFailedPrecondition)
}

func TestDeleteGroup_OnlyOwner(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "do")
	other := join(t, srv, "dx")
	gid := createGroup(t, srv, owner)
	if _, err := srv.Group.JoinGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.JoinGroupRequest{GroupId: gid}), other.token)); err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}

	// A non-owner cannot delete.
	_, err := srv.Group.DeleteGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.DeleteGroupRequest{GroupId: gid}), other.token))
	requireCode(t, err, connect.CodePermissionDenied)

	// The owner disbands it.
	if _, delErr := srv.Group.DeleteGroup(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.DeleteGroupRequest{GroupId: gid}), owner.token)); delErr != nil {
		t.Fatalf("DeleteGroup: %v", delErr)
	}
	mine, err := srv.Group.ListMyGroups(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.ListMyGroupsRequest{}), owner.token))
	if err != nil {
		t.Fatalf("ListMyGroups: %v", err)
	}
	if len(mine.Msg.GetGroups()) != 0 {
		t.Errorf("group survived deletion: %+v", mine.Msg.GetGroups())
	}
}

func TestListMembers_RequiresMembership(t *testing.T) {
	srv := testconnect.New(t)
	owner := join(t, srv, "mo")
	outsider := join(t, srv, "mx")
	gid := createGroup(t, srv, owner)

	_, err := srv.Group.ListMembers(t.Context(),
		testconnect.Authorize(connect.NewRequest(&groupv1.ListMembersRequest{GroupId: gid}), outsider.token))
	requireCode(t, err, connect.CodeFailedPrecondition)
}
