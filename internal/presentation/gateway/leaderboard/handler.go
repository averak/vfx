package leaderboard

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	leaderboardv1 "github.com/averak/vfx/gen/go/vfx/v1/leaderboard"
	"github.com/averak/vfx/gen/go/vfx/v1/leaderboard/leaderboardconnect"
	usecaseleaderboard "github.com/averak/vfx/internal/usecase/leaderboard"
)

type Handler struct {
	uc *usecaseleaderboard.Usecase
}

var _ leaderboardconnect.LeaderboardServiceHandler = (*Handler)(nil)

func New(uc *usecaseleaderboard.Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) SubmitScore(ctx context.Context, req *connect.Request[leaderboardv1.SubmitScoreRequest]) (*connect.Response[leaderboardv1.SubmitScoreResponse], error) {
	playerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	entry, improved, err := h.uc.SubmitScore(ctx, req.Msg.GetLeaderboardId(), playerID, req.Msg.GetScore())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&leaderboardv1.SubmitScoreResponse{
		Entry:    toRankEntryPb(entry),
		Improved: improved,
	}), nil
}

func (h *Handler) ListRanks(ctx context.Context, req *connect.Request[leaderboardv1.ListRanksRequest]) (*connect.Response[leaderboardv1.ListRanksResponse], error) {
	if _, err := requireAuth(ctx); err != nil {
		return nil, err
	}
	entries, err := h.uc.ListRanks(ctx, req.Msg.GetLeaderboardId(), int(req.Msg.GetOffset()), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&leaderboardv1.ListRanksResponse{Entries: toRankEntryListPb(entries)}), nil
}

func (h *Handler) GetPlayerRank(ctx context.Context, req *connect.Request[leaderboardv1.GetPlayerRankRequest]) (*connect.Response[leaderboardv1.GetPlayerRankResponse], error) {
	caller, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	target := caller
	if raw := req.Msg.GetPlayerId(); raw != "" {
		parsed, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid player_id"))
		}
		target = parsed
	}
	entry, err := h.uc.GetPlayerRank(ctx, req.Msg.GetLeaderboardId(), target)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&leaderboardv1.GetPlayerRankResponse{Entry: toRankEntryPb(entry)}), nil
}

func (h *Handler) ListRanksAroundPlayer(ctx context.Context, req *connect.Request[leaderboardv1.ListRanksAroundPlayerRequest]) (*connect.Response[leaderboardv1.ListRanksAroundPlayerResponse], error) {
	playerID, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := h.uc.ListRanksAroundPlayer(ctx, req.Msg.GetLeaderboardId(), playerID, int(req.Msg.GetRadius()))
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&leaderboardv1.ListRanksAroundPlayerResponse{Entries: toRankEntryListPb(entries)}), nil
}
