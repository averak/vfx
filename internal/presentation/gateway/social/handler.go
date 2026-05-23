package social

import (
	"context"

	"connectrpc.com/connect"

	socialv1 "github.com/averak/vfx/gen/go/vfx/v1/social"
	"github.com/averak/vfx/gen/go/vfx/v1/social/socialconnect"
	usecasesocial "github.com/averak/vfx/internal/usecase/social"
)

type Handler struct {
	uc *usecasesocial.Usecase
}

var _ socialconnect.SocialServiceHandler = (*Handler)(nil)

func New(uc *usecasesocial.Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) SendFriendRequest(ctx context.Context, req *connect.Request[socialv1.SendFriendRequestRequest]) (*connect.Response[socialv1.SendFriendRequestResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	addressee, err := parsePlayerID(req.Msg.GetAddresseePlayerId())
	if err != nil {
		return nil, err
	}
	accepted, err := h.uc.SendFriendRequest(ctx, me, addressee)
	if err != nil {
		return nil, toConnectError(err)
	}
	status := socialv1.RequestStatus_REQUEST_STATUS_PENDING
	if accepted {
		status = socialv1.RequestStatus_REQUEST_STATUS_ACCEPTED
	}
	return connect.NewResponse(&socialv1.SendFriendRequestResponse{Status: status}), nil
}

func (h *Handler) AcceptFriendRequest(ctx context.Context, req *connect.Request[socialv1.AcceptFriendRequestRequest]) (*connect.Response[socialv1.AcceptFriendRequestResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	requester, err := parsePlayerID(req.Msg.GetRequesterPlayerId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.AcceptFriendRequest(ctx, me, requester); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.AcceptFriendRequestResponse{}), nil
}

func (h *Handler) DeclineFriendRequest(ctx context.Context, req *connect.Request[socialv1.DeclineFriendRequestRequest]) (*connect.Response[socialv1.DeclineFriendRequestResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	requester, err := parsePlayerID(req.Msg.GetRequesterPlayerId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeclineFriendRequest(ctx, me, requester); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.DeclineFriendRequestResponse{}), nil
}

func (h *Handler) CancelFriendRequest(ctx context.Context, req *connect.Request[socialv1.CancelFriendRequestRequest]) (*connect.Response[socialv1.CancelFriendRequestResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	addressee, err := parsePlayerID(req.Msg.GetAddresseePlayerId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.CancelFriendRequest(ctx, me, addressee); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.CancelFriendRequestResponse{}), nil
}

func (h *Handler) ListFriends(ctx context.Context, _ *connect.Request[socialv1.ListFriendsRequest]) (*connect.Response[socialv1.ListFriendsResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	friends, err := h.uc.ListFriends(ctx, me)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*socialv1.Friend, len(friends))
	for i, f := range friends {
		out[i] = toFriendPb(f)
	}
	return connect.NewResponse(&socialv1.ListFriendsResponse{Friends: out}), nil
}

func (h *Handler) ListIncomingRequests(ctx context.Context, _ *connect.Request[socialv1.ListIncomingRequestsRequest]) (*connect.Response[socialv1.ListIncomingRequestsResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	requests, err := h.uc.ListIncomingRequests(ctx, me)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.ListIncomingRequestsResponse{Requests: toRequestListPb(requests)}), nil
}

func (h *Handler) ListOutgoingRequests(ctx context.Context, _ *connect.Request[socialv1.ListOutgoingRequestsRequest]) (*connect.Response[socialv1.ListOutgoingRequestsResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	requests, err := h.uc.ListOutgoingRequests(ctx, me)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.ListOutgoingRequestsResponse{Requests: toRequestListPb(requests)}), nil
}

func (h *Handler) RemoveFriend(ctx context.Context, req *connect.Request[socialv1.RemoveFriendRequest]) (*connect.Response[socialv1.RemoveFriendResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	friend, err := parsePlayerID(req.Msg.GetFriendPlayerId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.RemoveFriend(ctx, me, friend); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&socialv1.RemoveFriendResponse{}), nil
}
