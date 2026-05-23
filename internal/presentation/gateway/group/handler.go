// Package group wires the GroupService onto the usecase.
//
// The handler reads the caller from the auth context, parses the group id, and maps domain sentinel errors to Connect codes; group rules stay in the domain/usecase.
package group

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	groupv1 "github.com/averak/vfx/gen/go/vfx/v1/group"
	"github.com/averak/vfx/gen/go/vfx/v1/group/groupconnect"
	domaingroup "github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	usecasegroup "github.com/averak/vfx/internal/usecase/group"
)

type Handler struct {
	uc *usecasegroup.Usecase
}

var _ groupconnect.GroupServiceHandler = (*Handler)(nil)

func New(uc *usecasegroup.Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) CreateGroup(ctx context.Context, req *connect.Request[groupv1.CreateGroupRequest]) (*connect.Response[groupv1.CreateGroupResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	g, err := h.uc.CreateGroup(ctx, me, req.Msg.GetName())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&groupv1.CreateGroupResponse{Group: toGroupPb(g)}), nil
}

func (h *Handler) DeleteGroup(ctx context.Context, req *connect.Request[groupv1.DeleteGroupRequest]) (*connect.Response[groupv1.DeleteGroupResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	groupID, err := parseGroupID(req.Msg.GetGroupId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.DeleteGroup(ctx, me, groupID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&groupv1.DeleteGroupResponse{}), nil
}

func (h *Handler) JoinGroup(ctx context.Context, req *connect.Request[groupv1.JoinGroupRequest]) (*connect.Response[groupv1.JoinGroupResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	groupID, err := parseGroupID(req.Msg.GetGroupId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.JoinGroup(ctx, me, groupID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&groupv1.JoinGroupResponse{}), nil
}

func (h *Handler) LeaveGroup(ctx context.Context, req *connect.Request[groupv1.LeaveGroupRequest]) (*connect.Response[groupv1.LeaveGroupResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	groupID, err := parseGroupID(req.Msg.GetGroupId())
	if err != nil {
		return nil, err
	}
	if err := h.uc.LeaveGroup(ctx, me, groupID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&groupv1.LeaveGroupResponse{}), nil
}

func (h *Handler) ListMyGroups(ctx context.Context, _ *connect.Request[groupv1.ListMyGroupsRequest]) (*connect.Response[groupv1.ListMyGroupsResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := h.uc.ListMyGroups(ctx, me)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*groupv1.Group, len(groups))
	for i, g := range groups {
		out[i] = toGroupPb(g)
	}
	return connect.NewResponse(&groupv1.ListMyGroupsResponse{Groups: out}), nil
}

func (h *Handler) ListMembers(ctx context.Context, req *connect.Request[groupv1.ListMembersRequest]) (*connect.Response[groupv1.ListMembersResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	groupID, err := parseGroupID(req.Msg.GetGroupId())
	if err != nil {
		return nil, err
	}
	members, err := h.uc.ListMembers(ctx, me, groupID)
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*groupv1.Member, len(members))
	for i, m := range members {
		out[i] = &groupv1.Member{
			PlayerId: m.PlayerID.String(),
			Nickname: m.Nickname,
			JoinedAt: timestamppb.New(m.JoinedAt),
		}
	}
	return connect.NewResponse(&groupv1.ListMembersResponse{Members: out}), nil
}

func requireAuth(ctx context.Context) (uuid.UUID, error) {
	id, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return id, nil
}

func parseGroupID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid group id"))
	}
	return id, nil
}

func toGroupPb(g *domaingroup.Group) *groupv1.Group {
	return &groupv1.Group{
		Id:        g.ID.String(),
		Name:      g.Name,
		OwnerId:   g.OwnerID.String(),
		CreatedAt: timestamppb.New(g.CreatedAt),
	}
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, domaingroup.ErrInvalidName):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, domaingroup.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, domaingroup.ErrNotOwner):
		return connect.NewError(connect.CodePermissionDenied, err)
	case errors.Is(err, domaingroup.ErrOwnerMustDelete), errors.Is(err, domaingroup.ErrNotMember):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
