// Package group orchestrates the GroupService.
package group

import (
	"context"

	"github.com/google/uuid"

	domaingroup "github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

type Usecase struct {
	rw      tx.ReadWriter
	ro      tx.Reader
	repo    domaingroup.Repository
	members domaingroup.MembershipRepository
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domaingroup.Repository, members domaingroup.MembershipRepository) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo, members: members}
}

// CreateGroup makes the caller the owner and first member.
func (u *Usecase) CreateGroup(ctx context.Context, owner uuid.UUID, name string) (*domaingroup.Group, error) {
	now := clock.Now(ctx)
	g, valErr := domaingroup.New(uuid.New(), owner, name, now)
	if valErr != nil {
		return nil, valErr
	}
	err := u.rw.RW(ctx, func(ctx context.Context) error {
		if err := u.repo.Save(ctx, g); err != nil {
			return err
		}
		return u.members.Save(ctx, domaingroup.NewMembership(g.ID, owner, now))
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

// DeleteGroup disbands the group; only the owner may do it.
func (u *Usecase) DeleteGroup(ctx context.Context, me, groupID uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		g, err := u.repo.Find(ctx, groupID)
		if err != nil {
			return err
		}
		if g.OwnerID != me {
			return domaingroup.ErrNotOwner
		}
		return u.repo.Delete(ctx, groupID)
	})
}

// JoinGroup adds the caller to the group, returning domaingroup.ErrNotFound for an unknown group; idempotent.
func (u *Usecase) JoinGroup(ctx context.Context, me, groupID uuid.UUID) error {
	now := clock.Now(ctx)
	return u.rw.RW(ctx, func(ctx context.Context) error {
		if _, err := u.repo.Find(ctx, groupID); err != nil {
			return err
		}
		return u.members.Save(ctx, domaingroup.NewMembership(groupID, me, now))
	})
}

// LeaveGroup removes the caller's membership; the owner cannot leave and must delete the group instead.
func (u *Usecase) LeaveGroup(ctx context.Context, me, groupID uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		g, err := u.repo.Find(ctx, groupID)
		if err != nil {
			return err
		}
		if g.OwnerID == me {
			return domaingroup.ErrOwnerMustDelete
		}
		return u.members.Delete(ctx, groupID, me)
	})
}

func (u *Usecase) ListMyGroups(ctx context.Context, me uuid.UUID) ([]*domaingroup.Group, error) {
	var groups []*domaingroup.Group
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		groups, err = u.members.ListGroupsForPlayer(ctx, me)
		return err
	})
	return groups, err
}

// ListMembers returns the group's members; the caller must be a member to view them.
func (u *Usecase) ListMembers(ctx context.Context, me, groupID uuid.UUID) ([]*domaingroup.Member, error) {
	var members []*domaingroup.Member
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		member, err := u.members.IsMember(ctx, groupID, me)
		if err != nil {
			return err
		}
		if !member {
			return domaingroup.ErrNotMember
		}
		members, err = u.members.ListMembers(ctx, groupID)
		return err
	})
	return members, err
}
