package vfxclient

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/averak/vfx/gen/go/vfx/v1/chat"
)

// SendDirectMessage sends a DM to recipientPlayerID and returns the stored message.
func (c *Client) SendDirectMessage(ctx context.Context, recipientPlayerID, body string) (*chatv1.Message, error) {
	req := connect.NewRequest(&chatv1.SendDirectMessageRequest{RecipientId: recipientPlayerID, Body: body})
	c.authorize(req.Header())
	resp, err := c.chat.SendDirectMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: send direct message: %w", err)
	}
	return resp.Msg.GetMessage(), nil
}

// ListDirectMessages returns the conversation with otherPlayerID newest-first; pass a zero before for the latest page, or the oldest seen timestamp to page back.
func (c *Client) ListDirectMessages(ctx context.Context, otherPlayerID string, before time.Time, limit int32) ([]*chatv1.Message, error) {
	msg := &chatv1.ListDirectMessagesRequest{OtherPlayerId: otherPlayerID, Limit: limit}
	if !before.IsZero() {
		msg.Before = timestamppb.New(before)
	}
	req := connect.NewRequest(msg)
	c.authorize(req.Header())
	resp, err := c.chat.ListDirectMessages(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list direct messages: %w", err)
	}
	return resp.Msg.GetMessages(), nil
}

// SendChannelMessage posts to a channel (a group the caller belongs to) and returns the stored message.
func (c *Client) SendChannelMessage(ctx context.Context, channelID, body string) (*chatv1.ChannelMessage, error) {
	req := connect.NewRequest(&chatv1.SendChannelMessageRequest{ChannelId: channelID, Body: body})
	c.authorize(req.Header())
	resp, err := c.chat.SendChannelMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: send channel message: %w", err)
	}
	return resp.Msg.GetMessage(), nil
}

// ListChannelMessages returns a channel's history newest-first; pass a zero before for the latest page, or the oldest seen timestamp to page back.
func (c *Client) ListChannelMessages(ctx context.Context, channelID string, before time.Time, limit int32) ([]*chatv1.ChannelMessage, error) {
	msg := &chatv1.ListChannelMessagesRequest{ChannelId: channelID, Limit: limit}
	if !before.IsZero() {
		msg.Before = timestamppb.New(before)
	}
	req := connect.NewRequest(msg)
	c.authorize(req.Header())
	resp, err := c.chat.ListChannelMessages(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list channel messages: %w", err)
	}
	return resp.Msg.GetMessages(), nil
}
