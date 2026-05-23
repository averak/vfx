package vfxclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	leaderboardv1 "github.com/averak/vfx/gen/go/vfx/v1/leaderboard"
)

// SubmitScore records score on the leaderboard under its keep-best rule and returns the player's resulting entry and whether it improved their best.
func (c *Client) SubmitScore(ctx context.Context, leaderboardID string, score int64) (*leaderboardv1.RankEntry, bool, error) {
	req := connect.NewRequest(&leaderboardv1.SubmitScoreRequest{LeaderboardId: leaderboardID, Score: score})
	c.authorize(req.Header())
	resp, err := c.leaderboard.SubmitScore(ctx, req)
	if err != nil {
		return nil, false, fmt.Errorf("vfxclient: submit score: %w", err)
	}
	return resp.Msg.GetEntry(), resp.Msg.GetImproved(), nil
}

func (c *Client) ListRanks(ctx context.Context, leaderboardID string, offset, limit int32) ([]*leaderboardv1.RankEntry, error) {
	req := connect.NewRequest(&leaderboardv1.ListRanksRequest{LeaderboardId: leaderboardID, Offset: offset, Limit: limit})
	c.authorize(req.Header())
	resp, err := c.leaderboard.ListRanks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list ranks: %w", err)
	}
	return resp.Msg.GetEntries(), nil
}

// GetPlayerRank returns the authenticated player's rank on the leaderboard.
func (c *Client) GetPlayerRank(ctx context.Context, leaderboardID string) (*leaderboardv1.RankEntry, error) {
	req := connect.NewRequest(&leaderboardv1.GetPlayerRankRequest{LeaderboardId: leaderboardID})
	c.authorize(req.Header())
	resp, err := c.leaderboard.GetPlayerRank(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: get player rank: %w", err)
	}
	return resp.Msg.GetEntry(), nil
}

func (c *Client) ListRanksAroundPlayer(ctx context.Context, leaderboardID string, radius int32) ([]*leaderboardv1.RankEntry, error) {
	req := connect.NewRequest(&leaderboardv1.ListRanksAroundPlayerRequest{LeaderboardId: leaderboardID, Radius: radius})
	c.authorize(req.Header())
	resp, err := c.leaderboard.ListRanksAroundPlayer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list ranks around player: %w", err)
	}
	return resp.Msg.GetEntries(), nil
}
