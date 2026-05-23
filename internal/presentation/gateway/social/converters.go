// Package social wires the SocialService onto the usecase.
//
// The handler reads the caller from the auth context, parses the target player id, and maps domain sentinel errors to Connect codes; the friend-graph rules stay in the usecase.
package social

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	socialv1 "github.com/averak/vfx/gen/go/vfx/v1/social"
	domainsocial "github.com/averak/vfx/internal/domain/social"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
)

func requireAuth(ctx context.Context) (uuid.UUID, error) {
	id, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return id, nil
}

func parsePlayerID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid player id"))
	}
	return id, nil
}

func toFriendPb(f *domainsocial.Friend) *socialv1.Friend {
	return &socialv1.Friend{
		PlayerId: f.PlayerID.String(),
		Nickname: f.Nickname,
		Since:    timestamppb.New(f.Since),
	}
}

func toFriendRequestPb(r *domainsocial.PendingRequest) *socialv1.FriendRequest {
	return &socialv1.FriendRequest{
		PlayerId:    r.PlayerID.String(),
		Nickname:    r.Nickname,
		RequestedAt: timestamppb.New(r.RequestedAt),
	}
}

func toRequestListPb(requests []*domainsocial.PendingRequest) []*socialv1.FriendRequest {
	out := make([]*socialv1.FriendRequest, len(requests))
	for i, r := range requests {
		out[i] = toFriendRequestPb(r)
	}
	return out
}

func toBlockedListPb(blocked []*domainsocial.BlockedPlayer) []*socialv1.BlockedPlayer {
	out := make([]*socialv1.BlockedPlayer, len(blocked))
	for i, b := range blocked {
		out[i] = &socialv1.BlockedPlayer{
			PlayerId:  b.PlayerID.String(),
			Nickname:  b.Nickname,
			BlockedAt: timestamppb.New(b.BlockedAt),
		}
	}
	return out
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, domainsocial.ErrSelfFriend), errors.Is(err, domainsocial.ErrSelfBlock):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, domainsocial.ErrAlreadyFriends), errors.Is(err, domainsocial.ErrAlreadyRequested):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, domainsocial.ErrRequestNotFound), errors.Is(err, domainsocial.ErrNotFriends):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, domainsocial.ErrBlocked):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
