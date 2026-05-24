package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/repository"
)

func seedGroup(t *testing.T, s *db.Session, owner uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustRW(t, s, func(ctx context.Context) error {
		g, err := group.New(id, owner, "clan", time.Now().UTC())
		if err != nil {
			return err
		}
		return repository.NewGroup().Save(ctx, g)
	})
	return id
}

func TestMembership_JoinIdempotentLeaveAndQueries(t *testing.T) {
	s := newSession(t)
	repo := repository.NewGroupMembership()
	owner := seedPlayer(t, s)
	other := seedPlayer(t, s)
	gid := seedGroup(t, s, owner)
	now := time.Now().UTC()

	mustRW(t, s, func(ctx context.Context) error {
		if err := repo.Save(ctx, group.NewMembership(gid, owner, now)); err != nil {
			return err
		}
		// Re-join is a no-op, not an error.
		if err := repo.Save(ctx, group.NewMembership(gid, owner, now)); err != nil {
			return err
		}
		return repo.Save(ctx, group.NewMembership(gid, other, now))
	})

	mustRW(t, s, func(ctx context.Context) error {
		ok, err := repo.IsMember(ctx, gid, owner)
		if err != nil {
			return err
		}
		if !ok {
			t.Error("IsMember(owner) = false, want true")
		}
		members, err := repo.ListMembers(ctx, gid)
		if err != nil {
			return err
		}
		if len(members) != 2 {
			t.Errorf("ListMembers = %d, want 2", len(members))
		}
		groups, err := repo.ListGroupsForPlayer(ctx, other)
		if err != nil {
			return err
		}
		if len(groups) != 1 || groups[0].ID != gid {
			t.Errorf("ListGroupsForPlayer(other) = %+v, want the one group", groups)
		}
		return nil
	})

	mustRW(t, s, func(ctx context.Context) error { return repo.Delete(ctx, gid, other) })
	if err := s.RW(t.Context(), func(ctx context.Context) error { return repo.Delete(ctx, gid, other) }); !errors.Is(err, group.ErrNotMember) {
		t.Errorf("second leave = %v, want ErrNotMember", err)
	}
}
