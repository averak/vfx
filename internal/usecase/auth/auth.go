// Package auth orchestrates the AuthService. It glues together the
// player repository, refresh token repository, token signer, and clock.
//
// Anonymous login is the only credential currently supported: if a
// device_id is provided, the same Player is returned across calls; if
// not, every call mints a fresh Player whose identity row carries a
// random server-side provider_uid and is therefore unrecoverable.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/stdx/db"
)

// Usecase exposes the auth operations to the handler layer.
type Usecase struct {
	session          *db.Session
	playerRepo       player.Repository
	refreshTokenRepo player.RefreshTokenRepository
	signer           *token.Signer
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

// New wires the usecase. The two TTLs come from config so they can be
// tuned per environment without code changes.
func New(
	session *db.Session,
	playerRepo player.Repository,
	refreshTokenRepo player.RefreshTokenRepository,
	signer *token.Signer,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *Usecase {
	return &Usecase{
		session:          session,
		playerRepo:       playerRepo,
		refreshTokenRepo: refreshTokenRepo,
		signer:           signer,
		accessTokenTTL:   accessTokenTTL,
		refreshTokenTTL:  refreshTokenTTL,
	}
}

// LoginResult is what LoginAnonymous returns: the freshly issued
// access/refresh pair plus the player they identify.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Player       *player.Player
}

// LoginAnonymous resolves an anonymous credential into a logged-in
// session. A non-nil deviceID makes the login idempotent across calls;
// a nil deviceID always mints a brand-new Player.
func (u *Usecase) LoginAnonymous(ctx context.Context, deviceID, nickname *string) (*LoginResult, error) {
	now := clock.Now(ctx)

	var result *LoginResult
	err := u.session.RW(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := u.findOrCreatePlayer(ctx, tx, deviceID, nickname, now)
		if err != nil {
			return err
		}
		accessToken, refreshTokenRaw, err := u.issueTokens(ctx, tx, p.ID, now)
		if err != nil {
			return err
		}
		result = &LoginResult{
			AccessToken:  accessToken,
			RefreshToken: refreshTokenRaw,
			Player:       p,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Refresh mints a new access token (and rotates the refresh token) for
// the holder of the given refresh token. The old refresh token is
// revoked atomically with the issue of the new pair.
func (u *Usecase) Refresh(ctx context.Context, refreshTokenRaw string) (*LoginResult, error) {
	now := clock.Now(ctx)

	var result *LoginResult
	err := u.session.RW(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rt, err := u.refreshTokenRepo.FindByHash(ctx, tx, token.HashRefresh(refreshTokenRaw), now)
		if err != nil {
			return err
		}
		if !rt.IsActive(now) {
			return player.ErrRefreshTokenInvalid
		}
		if revokeErr := u.refreshTokenRepo.Revoke(ctx, tx, rt.ID, now); revokeErr != nil {
			return revokeErr
		}
		p, err := u.playerRepo.GetByID(ctx, tx, rt.PlayerID)
		if err != nil {
			return err
		}
		accessToken, newRefreshRaw, err := u.issueTokens(ctx, tx, p.ID, now)
		if err != nil {
			return err
		}
		result = &LoginResult{
			AccessToken:  accessToken,
			RefreshToken: newRefreshRaw,
			Player:       p,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Logout revokes every refresh token belonging to the player. Access
// tokens issued before this call remain valid until they expire on
// their own — the server stays stateless about access tokens by design.
func (u *Usecase) Logout(ctx context.Context, playerID uuid.UUID) error {
	now := clock.Now(ctx)
	return u.session.RW(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return u.refreshTokenRepo.RevokeAllForPlayer(ctx, tx, playerID, now)
	})
}

// UpdateProfile lets the player rename themselves. Passing a nil
// nickname leaves the field unchanged so this RPC can grow to set other
// optional fields without becoming destructive.
func (u *Usecase) UpdateProfile(ctx context.Context, playerID uuid.UUID, nickname *string) (*player.Player, error) {
	now := clock.Now(ctx)

	var updated *player.Player
	err := u.session.RW(ctx, func(ctx context.Context, tx pgx.Tx) error {
		p, err := u.playerRepo.GetByID(ctx, tx, playerID)
		if err != nil {
			return err
		}
		if nickname != nil {
			p.SetNickname(nickname, now)
		}
		if err := u.playerRepo.UpdateNickname(ctx, tx, p); err != nil {
			return err
		}
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// findOrCreatePlayer returns an existing player for a known device_id
// or creates a new one when the device is fresh or unspecified.
func (u *Usecase) findOrCreatePlayer(ctx context.Context, tx pgx.Tx, deviceID, nickname *string, now time.Time) (*player.Player, error) {
	if deviceID != nil {
		existing, err := u.playerRepo.FindPlayerByIdentity(ctx, tx, player.ProviderAnonymous, *deviceID)
		switch {
		case err == nil:
			return existing, nil
		case errors.Is(err, player.ErrIdentityNotFound):
			// fall through to creation with the supplied device id
		default:
			return nil, err
		}
	}

	providerUID := uuid.NewString()
	if deviceID != nil {
		providerUID = *deviceID
	}

	p := player.New(uuid.New(), nickname, now)
	if err := u.playerRepo.Save(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("auth: save player: %w", err)
	}
	identity := player.NewIdentity(uuid.New(), p.ID, player.ProviderAnonymous, providerUID, now)
	if err := u.playerRepo.SaveIdentity(ctx, tx, identity); err != nil {
		return nil, fmt.Errorf("auth: save identity: %w", err)
	}
	return p, nil
}

// issueTokens mints an access token and persists a refresh token,
// returning both values for the caller to assemble into a response.
func (u *Usecase) issueTokens(ctx context.Context, tx pgx.Tx, playerID uuid.UUID, now time.Time) (accessToken, refreshTokenRaw string, err error) {
	accessToken, err = u.signer.SignAccess(playerID, now, u.accessTokenTTL)
	if err != nil {
		return "", "", err
	}
	refresh, err := token.NewRefresh()
	if err != nil {
		return "", "", err
	}
	rt := &player.RefreshToken{
		ID:        uuid.New(),
		PlayerID:  playerID,
		Hash:      refresh.Hash,
		ExpiresAt: now.Add(u.refreshTokenTTL),
		CreatedAt: now,
	}
	if err = u.refreshTokenRepo.Create(ctx, tx, rt); err != nil {
		return "", "", fmt.Errorf("auth: create refresh token: %w", err)
	}
	return accessToken, refresh.Raw, nil
}
