package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/stdx/db"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
)

func newUsecase(t *testing.T) *usecaseauth.Usecase {
	t.Helper()
	pool := testdb.Pool(t)
	return usecaseauth.New(
		db.NewSession(pool),
		repository.NewPlayer(),
		repository.NewRefreshToken(),
		token.NewSigner("test-secret"),
		15*time.Minute,
		720*time.Hour,
	)
}

func ctxWithClock(t *testing.T) context.Context {
	t.Helper()
	return clock.With(t.Context(), time.Now().UTC())
}

func ptr[T any](v T) *T { return &v }

func TestLoginAnonymous_CreatesPlayerAndTokens(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	res, err := uc.LoginAnonymous(ctx, ptr("device-1"), ptr("Alice"))
	if err != nil {
		t.Fatalf("LoginAnonymous: %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Error("login returned an empty token")
	}
	if res.Player == nil || res.Player.Nickname == nil || *res.Player.Nickname != "Alice" {
		t.Errorf("player nickname not persisted: %+v", res.Player)
	}
}

func TestLoginAnonymous_SameDeviceReturnsSamePlayer(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	first, err := uc.LoginAnonymous(ctx, ptr("device-stable"), ptr("Bob"))
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	second, err := uc.LoginAnonymous(ctx, ptr("device-stable"), ptr("Ignored"))
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if first.Player.ID != second.Player.ID {
		t.Errorf("same device produced different players: %s vs %s", first.Player.ID, second.Player.ID)
	}
}

func TestLoginAnonymous_NoDeviceMintsFreshPlayer(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	a, err := uc.LoginAnonymous(ctx, nil, nil)
	if err != nil {
		t.Fatalf("login a: %v", err)
	}
	b, err := uc.LoginAnonymous(ctx, nil, nil)
	if err != nil {
		t.Fatalf("login b: %v", err)
	}
	if a.Player.ID == b.Player.ID {
		t.Error("two device-less logins shared a player")
	}
}

func TestRefresh_RotatesAndInvalidatesOldToken(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	login, err := uc.LoginAnonymous(ctx, ptr("device-refresh"), nil)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	refreshed, err := uc.Refresh(ctx, login.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.RefreshToken == login.RefreshToken {
		t.Error("Refresh did not rotate the refresh token")
	}

	// The old refresh token must no longer work.
	if _, err := uc.Refresh(ctx, login.RefreshToken); err == nil {
		t.Error("the old refresh token still works after rotation")
	}
}

func TestLogout_RevokesRefreshTokens(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	login, err := uc.LoginAnonymous(ctx, ptr("device-logout"), nil)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := uc.Logout(ctx, login.Player.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := uc.Refresh(ctx, login.RefreshToken); err == nil {
		t.Error("refresh token still works after logout")
	}
}

func TestUpdateProfile_ChangesNickname(t *testing.T) {
	uc := newUsecase(t)
	ctx := ctxWithClock(t)

	login, err := uc.LoginAnonymous(ctx, ptr("device-profile"), ptr("Before"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	updated, err := uc.UpdateProfile(ctx, login.Player.ID, ptr("After"))
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.Nickname == nil || *updated.Nickname != "After" {
		t.Errorf("nickname = %v, want After", updated.Nickname)
	}
}
