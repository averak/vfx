// Package social orchestrates the SocialService (friend graph).
package social

import (
	"context"
	"errors"

	"github.com/google/uuid"

	domainsocial "github.com/averak/vfx/internal/domain/social"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

type Usecase struct {
	rw   tx.ReadWriter
	ro   tx.Reader
	repo domainsocial.Repository
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domainsocial.Repository) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo}
}

// SendFriendRequest sends a request, or forms the friendship immediately when the addressee already has a pending request to the caller (mutual request).
// accepted reports whether a friendship was formed right away.
func (u *Usecase) SendFriendRequest(ctx context.Context, me, addressee uuid.UUID) (bool, error) {
	if me == addressee {
		return false, domainsocial.ErrSelfFriend
	}
	now := clock.Now(ctx)

	var accepted bool
	err := u.rw.RW(ctx, func(ctx context.Context) error {
		blocked, err := u.repo.IsBlocked(ctx, me, addressee)
		if err != nil {
			return err
		}
		if blocked {
			return domainsocial.ErrBlocked
		}

		friends, err := u.repo.AreFriends(ctx, me, addressee)
		if err != nil {
			return err
		}
		if friends {
			return domainsocial.ErrAlreadyFriends
		}

		reverse, err := u.repo.RequestExists(ctx, addressee, me)
		if err != nil {
			return err
		}
		if reverse {
			if delErr := u.repo.DeleteRequest(ctx, addressee, me); delErr != nil {
				return delErr
			}
			accepted = true
			return u.repo.CreateFriendship(ctx, me, addressee, now)
		}

		forward, err := u.repo.RequestExists(ctx, me, addressee)
		if err != nil {
			return err
		}
		if forward {
			return domainsocial.ErrAlreadyRequested
		}
		return u.repo.CreateRequest(ctx, me, addressee, now)
	})
	return accepted, err
}

// AcceptFriendRequest accepts the pending request requester -> me, forming the friendship.
// It returns domainsocial.ErrRequestNotFound when there is no such request.
func (u *Usecase) AcceptFriendRequest(ctx context.Context, me, requester uuid.UUID) error {
	now := clock.Now(ctx)
	return u.rw.RW(ctx, func(ctx context.Context) error {
		if err := u.repo.DeleteRequest(ctx, requester, me); err != nil {
			return err
		}
		return u.repo.CreateFriendship(ctx, requester, me, now)
	})
}

// DeclineFriendRequest rejects the pending request requester -> me.
func (u *Usecase) DeclineFriendRequest(ctx context.Context, me, requester uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		return u.repo.DeleteRequest(ctx, requester, me)
	})
}

// CancelFriendRequest withdraws the pending request me -> addressee.
func (u *Usecase) CancelFriendRequest(ctx context.Context, me, addressee uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		return u.repo.DeleteRequest(ctx, me, addressee)
	})
}

func (u *Usecase) RemoveFriend(ctx context.Context, me, friend uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		return u.repo.DeleteFriendship(ctx, me, friend)
	})
}

// BlockPlayer blocks target and severs any existing relationship: it removes the friendship and pending requests in both directions, ignoring whichever do not exist.
func (u *Usecase) BlockPlayer(ctx context.Context, me, target uuid.UUID) error {
	if me == target {
		return domainsocial.ErrSelfBlock
	}
	now := clock.Now(ctx)
	return u.rw.RW(ctx, func(ctx context.Context) error {
		if err := u.repo.Block(ctx, me, target, now); err != nil {
			return err
		}
		if err := u.repo.DeleteFriendship(ctx, me, target); err != nil && !errors.Is(err, domainsocial.ErrNotFriends) {
			return err
		}
		if err := u.repo.DeleteRequest(ctx, me, target); err != nil && !errors.Is(err, domainsocial.ErrRequestNotFound) {
			return err
		}
		if err := u.repo.DeleteRequest(ctx, target, me); err != nil && !errors.Is(err, domainsocial.ErrRequestNotFound) {
			return err
		}
		return nil
	})
}

func (u *Usecase) UnblockPlayer(ctx context.Context, me, target uuid.UUID) error {
	return u.rw.RW(ctx, func(ctx context.Context) error {
		return u.repo.Unblock(ctx, me, target)
	})
}

func (u *Usecase) ListBlocked(ctx context.Context, me uuid.UUID) ([]*domainsocial.BlockedPlayer, error) {
	var blocked []*domainsocial.BlockedPlayer
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		blocked, err = u.repo.ListBlocked(ctx, me)
		return err
	})
	return blocked, err
}

func (u *Usecase) ListFriends(ctx context.Context, me uuid.UUID) ([]*domainsocial.Friend, error) {
	var friends []*domainsocial.Friend
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		friends, err = u.repo.ListFriends(ctx, me)
		return err
	})
	return friends, err
}

func (u *Usecase) ListIncomingRequests(ctx context.Context, me uuid.UUID) ([]*domainsocial.PendingRequest, error) {
	var requests []*domainsocial.PendingRequest
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		requests, err = u.repo.ListIncoming(ctx, me)
		return err
	})
	return requests, err
}

func (u *Usecase) ListOutgoingRequests(ctx context.Context, me uuid.UUID) ([]*domainsocial.PendingRequest, error) {
	var requests []*domainsocial.PendingRequest
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		requests, err = u.repo.ListOutgoing(ctx, me)
		return err
	})
	return requests, err
}
