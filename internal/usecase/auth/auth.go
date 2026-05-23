// Package auth orchestrates the AuthService.
//
// Two credentials are supported: anonymous (guest) and OIDC (Google / Apple).
// Anonymous: a non-nil device_id returns the same Player across calls; a nil one mints a fresh Player whose identity carries a random server-side provider_uid and is therefore unrecoverable.
// OIDC: the verified subject is the identity, so the same provider account always maps to the same Player; an anonymous Player can be upgraded by linking an OIDC identity.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

// ErrInvalidCredential is returned when an OIDC token fails verification (bad signature, wrong audience, expired, or an unconfigured provider).
var ErrInvalidCredential = errors.New("auth: invalid credential")

// TokenIssuer is a port, so the usecase depends on the capability rather than the crypto in infra.
type TokenIssuer interface {
	SignAccess(playerID uuid.UUID, now time.Time, ttl time.Duration) (string, error)
	NewRefresh() (raw string, hash []byte, err error)
	HashRefresh(raw string) []byte
}

// VerifiedIdentity is what an OIDCVerifier returns once a provider ID token checks out.
type VerifiedIdentity struct {
	// Subject is the provider's stable user id (the "sub" claim); it becomes the identity's provider_uid.
	Subject string

	// Name is an optional display name from the token, used as the initial nickname when a Player is first created.
	Name string
}

// OIDCVerifier validates a provider ID token and extracts its identity; it is a port so the usecase stays free of JWKS/HTTP concerns.
type OIDCVerifier interface {
	Verify(ctx context.Context, provider player.Provider, idToken string) (*VerifiedIdentity, error)
}

type Usecase struct {
	tx               tx.ReadWriter
	playerRepo       player.Repository
	refreshTokenRepo player.RefreshTokenRepository
	tokens           TokenIssuer
	verifier         OIDCVerifier
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

func New(
	transactor tx.ReadWriter,
	playerRepo player.Repository,
	refreshTokenRepo player.RefreshTokenRepository,
	tokens TokenIssuer,
	verifier OIDCVerifier,
	accessTokenTTL time.Duration,
	refreshTokenTTL time.Duration,
) *Usecase {
	return &Usecase{
		tx:               transactor,
		playerRepo:       playerRepo,
		refreshTokenRepo: refreshTokenRepo,
		tokens:           tokens,
		verifier:         verifier,
		accessTokenTTL:   accessTokenTTL,
		refreshTokenTTL:  refreshTokenTTL,
	}
}

type LoginResult struct {
	AccessToken  string
	RefreshToken string
	Player       *player.Player
}

// LoginAnonymous is idempotent for a non-nil deviceID; a nil one always mints a new Player.
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

// LoginOIDC verifies a provider ID token and logs in the matching Player, creating one on first sign-in.
// The same provider account always maps to the same Player.
func (u *Usecase) LoginOIDC(ctx context.Context, provider player.Provider, idToken string) (*LoginResult, error) {
	identity, verifyErr := u.verifier.Verify(ctx, provider, idToken)
	if verifyErr != nil {
		return nil, ErrInvalidCredential
	}
	now := clock.Now(ctx)

	var result *LoginResult
	err := u.tx.RW(ctx, func(ctx context.Context) error {
		p, err := u.findOrCreateByIdentity(ctx, provider, identity.Subject, nicknameOrNil(identity.Name), now)
		if err != nil {
			return err
		}
		accessToken, refreshTokenRaw, err := u.issueTokens(ctx, p.ID, now)
		if err != nil {
			return err
		}
		result = &LoginResult{AccessToken: accessToken, RefreshToken: refreshTokenRaw, Player: p}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// LinkIdentity attaches a verified OIDC identity to an existing (typically anonymous) Player, upgrading it.
// It returns player.ErrIdentityAlreadyLinked when that identity already belongs to a different Player, and is idempotent when it already belongs to this one.
func (u *Usecase) LinkIdentity(ctx context.Context, playerID uuid.UUID, provider player.Provider, idToken string) (*player.Player, error) {
	identity, verifyErr := u.verifier.Verify(ctx, provider, idToken)
	if verifyErr != nil {
		return nil, ErrInvalidCredential
	}
	now := clock.Now(ctx)

	var linked *player.Player
	err := u.tx.RW(ctx, func(ctx context.Context) error {
		owner, err := u.playerRepo.FindPlayerByIdentity(ctx, provider, identity.Subject)
		switch {
		case err == nil:
			if owner.ID != playerID {
				return player.ErrIdentityAlreadyLinked
			}
			linked = owner // already linked to this player; nothing to do
			return nil
		case errors.Is(err, player.ErrIdentityNotFound):
			// fall through to link it
		default:
			return err
		}

		me, err := u.playerRepo.GetByID(ctx, playerID)
		if err != nil {
			return err
		}
		if err := u.playerRepo.SaveIdentity(ctx, player.NewIdentity(uuid.New(), playerID, provider, identity.Subject, now)); err != nil {
			return fmt.Errorf("auth: link identity: %w", err)
		}
		linked = me
		return nil
	})
	if err != nil {
		return nil, err
	}
	return linked, nil
}

// Refresh rotates the refresh token: the old one is revoked atomically with the issue of the new pair.
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

// Logout revokes the player's refresh tokens; access tokens issued earlier stay valid until they expire, since the server is stateless about them by design.
func (u *Usecase) Logout(ctx context.Context, playerID uuid.UUID) error {
	now := clock.Now(ctx)
	return u.tx.RW(ctx, func(ctx context.Context) error {
		return u.refreshTokenRepo.RevokeAllForPlayer(ctx, playerID, now)
	})
}

// UpdateProfile leaves a field unchanged when its argument is nil, so the RPC can grow to set other optional fields without becoming destructive.
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
		if err := u.playerRepo.Save(ctx, p); err != nil {
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

// findOrCreateByIdentity returns the Player owning (provider, providerUID), creating a fresh Player and identity on first sign-in.
func (u *Usecase) findOrCreateByIdentity(ctx context.Context, provider player.Provider, providerUID string, nickname *string, now time.Time) (*player.Player, error) {
	existing, err := u.playerRepo.FindPlayerByIdentity(ctx, provider, providerUID)
	switch {
	case err == nil:
		return existing, nil
	case errors.Is(err, player.ErrIdentityNotFound):
		// fall through to creation
	default:
		return nil, err
	}

	p, err := player.New(uuid.New(), nickname, now)
	if err != nil {
		return nil, err
	}
	if err := u.playerRepo.Save(ctx, p); err != nil {
		return nil, fmt.Errorf("auth: save player: %w", err)
	}
	if err := u.playerRepo.SaveIdentity(ctx, player.NewIdentity(uuid.New(), p.ID, provider, providerUID, now)); err != nil {
		return nil, fmt.Errorf("auth: save identity: %w", err)
	}
	return p, nil
}

// nicknameOrNil adopts a provider-supplied display name only when it satisfies the Player nickname invariant; otherwise the Player starts unnamed and can set one via UpdateProfile.
func nicknameOrNil(name string) *string {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > player.MaxNicknameLength {
		return nil
	}
	return &name
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
