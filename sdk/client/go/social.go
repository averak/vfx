package vfxclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	socialv1 "github.com/averak/vfx/gen/go/vfx/v1/social"
)

// SendFriendRequest sends a friend request; the returned status is ACCEPTED when the addressee already had a pending request to the caller (mutual), otherwise PENDING.
func (c *Client) SendFriendRequest(ctx context.Context, addresseePlayerID string) (socialv1.RequestStatus, error) {
	req := connect.NewRequest(&socialv1.SendFriendRequestRequest{AddresseePlayerId: addresseePlayerID})
	c.authorize(req.Header())
	resp, err := c.social.SendFriendRequest(ctx, req)
	if err != nil {
		return socialv1.RequestStatus_REQUEST_STATUS_UNSPECIFIED, fmt.Errorf("vfxclient: send friend request: %w", err)
	}
	return resp.Msg.GetStatus(), nil
}

func (c *Client) AcceptFriendRequest(ctx context.Context, requesterPlayerID string) error {
	req := connect.NewRequest(&socialv1.AcceptFriendRequestRequest{RequesterPlayerId: requesterPlayerID})
	c.authorize(req.Header())
	if _, err := c.social.AcceptFriendRequest(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: accept friend request: %w", err)
	}
	return nil
}

func (c *Client) DeclineFriendRequest(ctx context.Context, requesterPlayerID string) error {
	req := connect.NewRequest(&socialv1.DeclineFriendRequestRequest{RequesterPlayerId: requesterPlayerID})
	c.authorize(req.Header())
	if _, err := c.social.DeclineFriendRequest(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: decline friend request: %w", err)
	}
	return nil
}

func (c *Client) CancelFriendRequest(ctx context.Context, addresseePlayerID string) error {
	req := connect.NewRequest(&socialv1.CancelFriendRequestRequest{AddresseePlayerId: addresseePlayerID})
	c.authorize(req.Header())
	if _, err := c.social.CancelFriendRequest(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: cancel friend request: %w", err)
	}
	return nil
}

func (c *Client) ListFriends(ctx context.Context) ([]*socialv1.Friend, error) {
	req := connect.NewRequest(&socialv1.ListFriendsRequest{})
	c.authorize(req.Header())
	resp, err := c.social.ListFriends(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list friends: %w", err)
	}
	return resp.Msg.GetFriends(), nil
}

func (c *Client) ListIncomingFriendRequests(ctx context.Context) ([]*socialv1.FriendRequest, error) {
	req := connect.NewRequest(&socialv1.ListIncomingRequestsRequest{})
	c.authorize(req.Header())
	resp, err := c.social.ListIncomingRequests(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list incoming requests: %w", err)
	}
	return resp.Msg.GetRequests(), nil
}

func (c *Client) ListOutgoingFriendRequests(ctx context.Context) ([]*socialv1.FriendRequest, error) {
	req := connect.NewRequest(&socialv1.ListOutgoingRequestsRequest{})
	c.authorize(req.Header())
	resp, err := c.social.ListOutgoingRequests(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list outgoing requests: %w", err)
	}
	return resp.Msg.GetRequests(), nil
}

func (c *Client) RemoveFriend(ctx context.Context, friendPlayerID string) error {
	req := connect.NewRequest(&socialv1.RemoveFriendRequest{FriendPlayerId: friendPlayerID})
	c.authorize(req.Header())
	if _, err := c.social.RemoveFriend(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: remove friend: %w", err)
	}
	return nil
}

// BlockPlayer blocks a player, severing any friendship and pending requests; idempotent.
func (c *Client) BlockPlayer(ctx context.Context, playerID string) error {
	req := connect.NewRequest(&socialv1.BlockPlayerRequest{PlayerId: playerID})
	c.authorize(req.Header())
	if _, err := c.social.BlockPlayer(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: block player: %w", err)
	}
	return nil
}

func (c *Client) UnblockPlayer(ctx context.Context, playerID string) error {
	req := connect.NewRequest(&socialv1.UnblockPlayerRequest{PlayerId: playerID})
	c.authorize(req.Header())
	if _, err := c.social.UnblockPlayer(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: unblock player: %w", err)
	}
	return nil
}

func (c *Client) ListBlocked(ctx context.Context) ([]*socialv1.BlockedPlayer, error) {
	req := connect.NewRequest(&socialv1.ListBlockedRequest{})
	c.authorize(req.Header())
	resp, err := c.social.ListBlocked(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list blocked: %w", err)
	}
	return resp.Msg.GetBlocked(), nil
}
