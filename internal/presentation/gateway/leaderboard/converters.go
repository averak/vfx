// Package leaderboard wires the LeaderboardService onto the usecase.
//
// The handler owns proto-to-domain translation and the mapping from domain sentinel errors to Connect codes; the ranking rules stay in the usecase and domain.
package leaderboard

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	leaderboardv1 "github.com/averak/vfx/gen/go/vfx/v1/leaderboard"
	domainleaderboard "github.com/averak/vfx/internal/domain/leaderboard"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
)

func requireAuth(ctx context.Context) (uuid.UUID, error) {
	id, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return id, nil
}

func toRankEntryPb(e *domainleaderboard.RankedEntry) *leaderboardv1.RankEntry {
	return &leaderboardv1.RankEntry{
		Rank:       e.Rank,
		PlayerId:   e.PlayerID.String(),
		Nickname:   e.Nickname,
		Score:      e.Score,
		AchievedAt: timestamppb.New(e.AchievedAt),
	}
}

func toRankEntryListPb(entries []*domainleaderboard.RankedEntry) []*leaderboardv1.RankEntry {
	out := make([]*leaderboardv1.RankEntry, len(entries))
	for i, e := range entries {
		out[i] = toRankEntryPb(e)
	}
	return out
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, domainleaderboard.ErrLeaderboardNotFound), errors.Is(err, domainleaderboard.ErrEntryNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
