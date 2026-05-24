package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/averak/vfx/internal/domain/social"
	"github.com/averak/vfx/internal/infra/repository"
)

func TestFriendRequests_SaveExistsDeleteList(t *testing.T) {
	s := newSession(t)
	repo := repository.NewFriendRequests()
	a := seedPlayer(t, s)
	b := seedPlayer(t, s)

	mustRW(t, s, func(ctx context.Context) error { return repo.Save(ctx, social.NewFriendRequest(a, b)) })

	mustRW(t, s, func(ctx context.Context) error {
		ok, err := repo.Exists(ctx, a, b)
		if err != nil {
			return err
		}
		if !ok {
			t.Error("Exists(a->b) = false, want true")
		}
		incoming, err := repo.ListIncoming(ctx, b)
		if err != nil {
			return err
		}
		if len(incoming) != 1 || incoming[0].PlayerID != a {
			t.Errorf("ListIncoming(b) = %+v, want one from a", incoming)
		}
		outgoing, err := repo.ListOutgoing(ctx, a)
		if err != nil {
			return err
		}
		if len(outgoing) != 1 || outgoing[0].PlayerID != b {
			t.Errorf("ListOutgoing(a) = %+v, want one to b", outgoing)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error { return repo.Delete(ctx, a, b) })
	if err := s.RW(t.Context(), func(ctx context.Context) error { return repo.Delete(ctx, a, b) }); !errors.Is(err, social.ErrRequestNotFound) {
		t.Errorf("re-delete = %v, want ErrRequestNotFound", err)
	}
}

func TestFriendships_CanonicalPairOrderIndependent(t *testing.T) {
	s := newSession(t)
	repo := repository.NewFriendships()
	a := seedPlayer(t, s)
	b := seedPlayer(t, s)

	mustRW(t, s, func(ctx context.Context) error { return repo.Save(ctx, social.NewFriendship(a, b)) })

	mustRW(t, s, func(ctx context.Context) error {
		ab, err := repo.Exists(ctx, a, b)
		if err != nil {
			return err
		}
		ba, err := repo.Exists(ctx, b, a)
		if err != nil {
			return err
		}
		if !ab || !ba {
			t.Errorf("Exists not order-independent: a,b=%v b,a=%v", ab, ba)
		}
		friends, err := repo.ListFriends(ctx, a)
		if err != nil {
			return err
		}
		if len(friends) != 1 || friends[0].PlayerID != b {
			t.Errorf("ListFriends(a) = %+v, want b", friends)
		}
		return nil
	})

	// Delete with the pair reversed still removes it.
	mustRW(t, s, func(ctx context.Context) error { return repo.Delete(ctx, b, a) })
	if err := s.RW(t.Context(), func(ctx context.Context) error { return repo.Delete(ctx, a, b) }); !errors.Is(err, social.ErrNotFriends) {
		t.Errorf("re-delete = %v, want ErrNotFriends", err)
	}
}

func TestBlocks_IdempotentAndEitherDirection(t *testing.T) {
	s := newSession(t)
	repo := repository.NewBlocks()
	a := seedPlayer(t, s)
	b := seedPlayer(t, s)

	mustRW(t, s, func(ctx context.Context) error {
		if err := repo.Save(ctx, social.NewBlock(a, b)); err != nil {
			return err
		}
		// Re-block is a no-op.
		return repo.Save(ctx, social.NewBlock(a, b))
	})

	mustRW(t, s, func(ctx context.Context) error {
		ab, err := repo.IsBlocked(ctx, a, b)
		if err != nil {
			return err
		}
		ba, err := repo.IsBlocked(ctx, b, a)
		if err != nil {
			return err
		}
		if !ab || !ba {
			t.Errorf("IsBlocked not symmetric: a,b=%v b,a=%v", ab, ba)
		}
		blocked, err := repo.ListBlocked(ctx, a)
		if err != nil {
			return err
		}
		if len(blocked) != 1 || blocked[0].PlayerID != b {
			t.Errorf("ListBlocked(a) = %+v, want b", blocked)
		}
		return nil
	})

	// Unblock is idempotent: a second one is not an error.
	mustRW(t, s, func(ctx context.Context) error { return repo.Delete(ctx, a, b) })
	mustRW(t, s, func(ctx context.Context) error { return repo.Delete(ctx, a, b) })
}
