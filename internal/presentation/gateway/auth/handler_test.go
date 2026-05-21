package auth_test

import (
	"testing"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/internal/testutils/testconnect"
)

func TestLogin_AnonymousOverTheWire(t *testing.T) {
	srv := testconnect.New(t)

	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{
				DeviceId: ptr("wire-device"),
				Nickname: ptr("Wire"),
			},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.Msg.GetAccessToken() == "" {
		t.Error("empty access token")
	}
	if resp.Msg.GetPlayer().GetNickname() != "Wire" {
		t.Errorf("nickname = %q, want Wire", resp.Msg.GetPlayer().GetNickname())
	}
}

func TestLogin_RequiresCredential(t *testing.T) {
	srv := testconnect.New(t)

	_, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{}))
	if err == nil {
		t.Fatal("Login with no credential succeeded, want InvalidArgument")
	}
	if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", got)
	}
}

func TestUpdateProfile_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)

	_, err := srv.Auth.UpdateProfile(t.Context(), connect.NewRequest(&authv1.UpdateProfileRequest{
		Nickname: ptr("Nope"),
	}))
	if err == nil {
		t.Fatal("UpdateProfile without a token succeeded, want Unauthenticated")
	}
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("code = %v, want Unauthenticated", got)
	}
}

func TestUpdateProfile_WithToken(t *testing.T) {
	srv := testconnect.New(t)

	login, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{DeviceId: ptr("profile-device")},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	resp, err := srv.Auth.UpdateProfile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.UpdateProfileRequest{
			Nickname: ptr("Renamed"),
		}), login.Msg.GetAccessToken()))
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if resp.Msg.GetPlayer().GetNickname() != "Renamed" {
		t.Errorf("nickname = %q, want Renamed", resp.Msg.GetPlayer().GetNickname())
	}
}

func ptr[T any](v T) *T { return &v }
