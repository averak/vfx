package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/repository"
)

func TestIdentity_SaveFindAndUniqueViolation(t *testing.T) {
	s := newSession(t)
	repo := repository.NewIdentity()
	p1 := seedPlayer(t, s)
	p2 := seedPlayer(t, s)
	now := time.Now().UTC()

	mustRW(t, s, func(ctx context.Context) error {
		if _, err := repo.Find(ctx, player.ProviderGoogle, "sub-x"); !errors.Is(err, player.ErrIdentityNotFound) {
			t.Errorf("Find unknown = %v, want ErrIdentityNotFound", err)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error {
		return repo.Save(ctx, player.NewIdentity(uuid.New(), p1, player.ProviderGoogle, "sub-1", now))
	})
	mustRW(t, s, func(ctx context.Context) error {
		got, err := repo.Find(ctx, player.ProviderGoogle, "sub-1")
		if err != nil {
			return err
		}
		if got.PlayerID != p1 {
			t.Errorf("PlayerID = %s, want %s", got.PlayerID, p1)
		}
		return nil
	})

	// The same (provider, uid) linked to a different player is the global-uniqueness violation.
	err := s.RW(t.Context(), func(ctx context.Context) error {
		return repo.Save(ctx, player.NewIdentity(uuid.New(), p2, player.ProviderGoogle, "sub-1", now))
	})
	if !errors.Is(err, player.ErrIdentityAlreadyLinked) {
		t.Errorf("duplicate identity = %v, want ErrIdentityAlreadyLinked", err)
	}
}
