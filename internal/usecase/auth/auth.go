// Package auth orchestrates the AuthService and owns the transaction boundary via a Transactor.
//
// Anonymous login is the only credential currently supported.
// A non-nil device_id returns the same Player across calls; a nil one mints a fresh Player whose identity carries a random server-side provider_uid and is therefore unrecoverable.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

// TokenIssuer mints the credentials a login produces.
// It is a port, so the usecase depends on the capability rather than the crypto in infra.
type TokenIssuer interface {
	SignAccess(playerID uuid.UUID, now time.Time, ttl time.Duration) (string, error)
	NewRefresh() (raw string, hash []byte, err error)
	HashRefresh(raw string) []byte
}

type Usecase struct {
	tx               tx.ReadWriter
	playerRepo       player.Repository
	refreshTokenRepo player.RefreshTokenRepository
	tokens           TokenIssuer
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

func New(
	transactor tx.ReadWriter,
	playerRepo player.Repository,
	refreshTokenRepo player.RefreshTokenRepository,
	tokens TokenIssuer,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *Usecase {
	return &Usecase{
		tx:               transactor,
		playerRepo:       playerRepo,
		refreshTokenRepo: refreshTokenRepo,
		tokens:           tokens,
		accessTokenTTL:   accessTokenTTL,
		refreshTokenTTL:  refreshTokenTTL,
	}
}

type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Player       *player.Player
}

// LoginAnonymous resolves an anonymous credential into a logged-in session.
// A non-nil deviceID makes the login idempotent across calls; a nil one always mints a new Player.
func (u *Usecase) LoginAnonymous(ctx context.Context, deviceID, nickname *string) (*LoginResult, error) {
	now := clock.Now(ctx)

	var result *LoginResult
	err := u.tx.RW(ctx, func(ctx context.Context) error {
		p, err := u.findOrCreatePlayer(ctx, deviceID, nickname, now)
		if err != nil {
			return err
		}
		accessToken, refreshTokenRaw, err := u.issueTokens(ctx, p.ID, now)
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

// Refresh mints a new access token and rotates the refresh token for the holder of the given one.
// The old token is revoked atomically with the issue of the new pair.
func (u *Usecase) Refresh(ctx context.Context, refreshTokenRaw string) (*LoginResult, error) {
	now := clock.Now(ctx)

	var result *LoginResult
	err := u.tx.RW(ctx, func(ctx context.Context) error {
		rt, err := u.refreshTokenRepo.FindByHash(ctx, u.tokens.HashRefresh(refreshTokenRaw), now)
		if err != nil {
			return err
		}
		if !rt.IsActive(now) {
			return player.ErrRefreshTokenInvalid
		}
		if revokeErr := u.refreshTokenRepo.Revoke(ctx, rt.ID, now); revokeErr != nil {
			return revokeErr
		}
		p, err := u.playerRepo.GetByID(ctx, rt.PlayerID)
		if err != nil {
			return err
		}
		accessToken, newRefreshRaw, err := u.issueTokens(ctx, p.ID, now)
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

// Logout revokes every refresh token belonging to the player.
// Access tokens issued before this call stay valid until they expire on their own: the server is stateless about access tokens by design.
func (u *Usecase) Logout(ctx context.Context, playerID uuid.UUID) error {
	now := clock.Now(ctx)
	return u.tx.RW(ctx, func(ctx context.Context) error {
		return u.refreshTokenRepo.RevokeAllForPlayer(ctx, playerID, now)
	})
}

// UpdateProfile lets the player rename themselves.
// A nil nickname leaves the field unchanged, so this RPC can grow to set other optional fields without becoming destructive.
func (u *Usecase) UpdateProfile(ctx context.Context, playerID uuid.UUID, nickname *string) (*player.Player, error) {
	now := clock.Now(ctx)

	var updated *player.Player
	err := u.tx.RW(ctx, func(ctx context.Context) error {
		p, err := u.playerRepo.GetByID(ctx, playerID)
		if err != nil {
			return err
		}
		if nickname != nil {
			if err := p.SetNickname(nickname, now); err != nil {
				return err
			}
		}
		if err := u.playerRepo.UpdateNickname(ctx, p); err != nil {
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

func (u *Usecase) findOrCreatePlayer(ctx context.Context, deviceID, nickname *string, now time.Time) (*player.Player, error) {
	if deviceID != nil {
		existing, err := u.playerRepo.FindPlayerByIdentity(ctx, player.ProviderAnonymous, *deviceID)
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

	p, err := player.New(uuid.New(), nickname, now)
	if err != nil {
		return nil, err
	}
	if err := u.playerRepo.Save(ctx, p); err != nil {
		return nil, fmt.Errorf("auth: save player: %w", err)
	}
	identity := player.NewIdentity(uuid.New(), p.ID, player.ProviderAnonymous, providerUID, now)
	if err := u.playerRepo.SaveIdentity(ctx, identity); err != nil {
		return nil, fmt.Errorf("auth: save identity: %w", err)
	}
	return p, nil
}

func (u *Usecase) issueTokens(ctx context.Context, playerID uuid.UUID, now time.Time) (accessToken, refreshTokenRaw string, err error) {
	accessToken, err = u.tokens.SignAccess(playerID, now, u.accessTokenTTL)
	if err != nil {
		return "", "", err
	}
	refreshRaw, refreshHash, err := u.tokens.NewRefresh()
	if err != nil {
		return "", "", err
	}
	rt := &player.RefreshToken{
		ID:        uuid.New(),
		PlayerID:  playerID,
		Hash:      refreshHash,
		ExpiresAt: now.Add(u.refreshTokenTTL),
		CreatedAt: now,
	}
	if err = u.refreshTokenRepo.Create(ctx, rt); err != nil {
		return "", "", fmt.Errorf("auth: create refresh token: %w", err)
	}
	return accessToken, refreshRaw, nil
}
