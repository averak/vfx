// Package auth exposes the AuthService Connect handler.
//
// The handler owns the proto-to-domain translation and the mapping
// from domain sentinel errors to Connect error codes. Business rules
// stay in the usecase package; this file is intentionally mechanical.
package auth

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
)

// Handler implements [authconnect.AuthServiceHandler].
type Handler struct {
	uc *usecaseauth.Usecase
}

// Verify the contract at compile time.
var _ authconnect.AuthServiceHandler = (*Handler)(nil)

// New wires the handler with its usecase.
func New(uc *usecaseauth.Usecase) *Handler {
	return &Handler{uc: uc}
}

// Login dispatches by credential kind. Only anonymous is supported.
func (h *Handler) Login(ctx context.Context, req *connect.Request[authv1.LoginRequest]) (*connect.Response[authv1.LoginResponse], error) {
	anon := req.Msg.GetAnonymous()
	if anon == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("credential is required"))
	}

	var (
		deviceID *string
		nickname *string
	)
	if anon.DeviceId != nil {
		v := anon.GetDeviceId()
		deviceID = &v
	}
	if anon.Nickname != nil {
		v := anon.GetNickname()
		nickname = &v
	}

	result, err := h.uc.LoginAnonymous(ctx, deviceID, nickname)
	if err != nil {
		return nil, toConnectError(err)
	}

	return connect.NewResponse(&authv1.LoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		Player:       toPlayerPb(result.Player),
	}), nil
}

func (h *Handler) Refresh(ctx context.Context, req *connect.Request[authv1.RefreshRequest]) (*connect.Response[authv1.RefreshResponse], error) {
	if req.Msg.GetRefreshToken() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("refresh_token is required"))
	}
	result, err := h.uc.Refresh(ctx, req.Msg.GetRefreshToken())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&authv1.RefreshResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
	}), nil
}

func (h *Handler) Logout(ctx context.Context, _ *connect.Request[authv1.LogoutRequest]) (*connect.Response[authv1.LogoutResponse], error) {
	playerID, ok := authctx.From(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if err := h.uc.Logout(ctx, playerID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&authv1.LogoutResponse{}), nil
}

func (h *Handler) UpdateProfile(ctx context.Context, req *connect.Request[authv1.UpdateProfileRequest]) (*connect.Response[authv1.UpdateProfileResponse], error) {
	playerID, ok := authctx.From(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	var nickname *string
	if req.Msg.Nickname != nil {
		v := req.Msg.GetNickname()
		nickname = &v
	}

	updated, err := h.uc.UpdateProfile(ctx, playerID, nickname)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&authv1.UpdateProfileResponse{
		Player: toPlayerPb(updated),
	}), nil
}

// toConnectError maps domain sentinel errors to Connect's standard
// codes. Anything else falls through as Internal so unexpected failures
// stay loud.
func toConnectError(err error) error {
	switch {
	case errors.Is(err, player.ErrPlayerNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, player.ErrIdentityNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, player.ErrRefreshTokenInvalid):
		return connect.NewError(connect.CodeUnauthenticated, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
