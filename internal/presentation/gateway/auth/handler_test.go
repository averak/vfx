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

func mustLogin(t *testing.T, srv *testconnect.Server, device string) *authv1.LoginResponse {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{
			Anonymous: &authv1.AnonymousCredential{DeviceId: ptr(device)},
		},
	}))
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return resp.Msg
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

// Refresh rotates the pair: the new tokens are issued and the presented refresh token is revoked, so reusing it fails.
func TestRefresh_RotatesAndRevokes(t *testing.T) {
	srv := testconnect.New(t)
	login := mustLogin(t, srv, "refresh-dev")

	rotated, err := srv.Auth.Refresh(t.Context(), connect.NewRequest(&authv1.RefreshRequest{
		RefreshToken: login.GetRefreshToken(),
	}))
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.Msg.GetAccessToken() == "" || rotated.Msg.GetRefreshToken() == "" {
		t.Error("Refresh returned an empty token")
	}

	_, err = srv.Auth.Refresh(t.Context(), connect.NewRequest(&authv1.RefreshRequest{
		RefreshToken: login.GetRefreshToken(),
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestRefresh_EmptyToken(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.Refresh(t.Context(), connect.NewRequest(&authv1.RefreshRequest{}))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestRefresh_UnknownToken(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.Refresh(t.Context(), connect.NewRequest(&authv1.RefreshRequest{
		RefreshToken: "not-a-real-token",
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestLogout_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.Logout(t.Context(), connect.NewRequest(&authv1.LogoutRequest{}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

// Logout revokes the player's refresh tokens, so a refresh with the pre-logout token then fails.
func TestLogout_RevokesRefreshTokens(t *testing.T) {
	srv := testconnect.New(t)
	login := mustLogin(t, srv, "logout-dev")

	if _, err := srv.Auth.Logout(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.LogoutRequest{}), login.GetAccessToken())); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err := srv.Auth.Refresh(t.Context(), connect.NewRequest(&authv1.RefreshRequest{
		RefreshToken: login.GetRefreshToken(),
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestUpdateProfile_InvalidNickname(t *testing.T) {
	srv := testconnect.New(t)
	login := mustLogin(t, srv, "invalid-nick")

	_, err := srv.Auth.UpdateProfile(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.UpdateProfileRequest{Nickname: ptr("   ")}), login.GetAccessToken()))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func oidcLogin(t *testing.T, srv *testconnect.Server, provider authv1.Provider, idToken string) *authv1.LoginResponse {
	t.Helper()
	resp, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Oidc{Oidc: &authv1.OidcCredential{Provider: provider, IdToken: idToken}},
	}))
	if err != nil {
		t.Fatalf("OIDC Login: %v", err)
	}
	return resp.Msg
}

// The same provider token always resolves to the same player.
func TestLogin_OIDCSameTokenSamePlayer(t *testing.T) {
	srv := testconnect.New(t)
	first := oidcLogin(t, srv, authv1.Provider_PROVIDER_GOOGLE, "alice")
	second := oidcLogin(t, srv, authv1.Provider_PROVIDER_GOOGLE, "alice")
	if first.GetPlayer().GetId() != second.GetPlayer().GetId() {
		t.Errorf("same token produced different players: %s vs %s", first.GetPlayer().GetId(), second.GetPlayer().GetId())
	}
	if first.GetAccessToken() == "" {
		t.Error("empty access token")
	}
}

func TestLogin_OIDCRejectsInvalidToken(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Oidc{Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_GOOGLE, IdToken: "invalid"}},
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

func TestLogin_OIDCRejectsUnspecifiedProvider(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.Login(t.Context(), connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Oidc{Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_UNSPECIFIED, IdToken: "alice"}},
	}))
	requireCode(t, err, connect.CodeInvalidArgument)
}

func TestLinkIdentity_RequiresAuth(t *testing.T) {
	srv := testconnect.New(t)
	_, err := srv.Auth.LinkIdentity(t.Context(), connect.NewRequest(&authv1.LinkIdentityRequest{
		Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_GOOGLE, IdToken: "alice"},
	}))
	requireCode(t, err, connect.CodeUnauthenticated)
}

// Linking upgrades an anonymous player: afterwards an OIDC login with that token returns the same player.
func TestLinkIdentity_UpgradesAnonymous(t *testing.T) {
	srv := testconnect.New(t)
	anon := mustLogin(t, srv, "anon-device")

	linked, err := srv.Auth.LinkIdentity(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.LinkIdentityRequest{
			Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_GOOGLE, IdToken: "linktoken"},
		}), anon.GetAccessToken()))
	if err != nil {
		t.Fatalf("LinkIdentity: %v", err)
	}
	if linked.Msg.GetPlayer().GetId() != anon.GetPlayer().GetId() {
		t.Errorf("link changed the player id")
	}

	viaOIDC := oidcLogin(t, srv, authv1.Provider_PROVIDER_GOOGLE, "linktoken")
	if viaOIDC.GetPlayer().GetId() != anon.GetPlayer().GetId() {
		t.Errorf("OIDC login after link returned a different player")
	}
}

func TestLinkIdentity_AlreadyLinkedToAnother(t *testing.T) {
	srv := testconnect.New(t)
	a := mustLogin(t, srv, "player-a")
	b := mustLogin(t, srv, "player-b")

	if _, err := srv.Auth.LinkIdentity(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.LinkIdentityRequest{
			Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_GOOGLE, IdToken: "shared"},
		}), a.GetAccessToken())); err != nil {
		t.Fatalf("first link: %v", err)
	}

	_, err := srv.Auth.LinkIdentity(t.Context(),
		testconnect.Authorize(connect.NewRequest(&authv1.LinkIdentityRequest{
			Oidc: &authv1.OidcCredential{Provider: authv1.Provider_PROVIDER_GOOGLE, IdToken: "shared"},
		}), b.GetAccessToken()))
	requireCode(t, err, connect.CodeAlreadyExists)
}

func ptr[T any](v T) *T { return &v }
