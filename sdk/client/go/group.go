package vfxclient

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	groupv1 "github.com/averak/vfx/gen/go/vfx/v1/group"
)

// CreateGroup creates a group with the caller as owner and first member.
func (c *Client) CreateGroup(ctx context.Context, name string) (*groupv1.Group, error) {
	req := connect.NewRequest(&groupv1.CreateGroupRequest{Name: name})
	c.authorize(req.Header())
	resp, err := c.group.CreateGroup(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: create group: %w", err)
	}
	return resp.Msg.GetGroup(), nil
}

// DeleteGroup disbands a group the caller owns.
func (c *Client) DeleteGroup(ctx context.Context, groupID string) error {
	req := connect.NewRequest(&groupv1.DeleteGroupRequest{GroupId: groupID})
	c.authorize(req.Header())
	if _, err := c.group.DeleteGroup(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: delete group: %w", err)
	}
	return nil
}

func (c *Client) JoinGroup(ctx context.Context, groupID string) error {
	req := connect.NewRequest(&groupv1.JoinGroupRequest{GroupId: groupID})
	c.authorize(req.Header())
	if _, err := c.group.JoinGroup(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: join group: %w", err)
	}
	return nil
}

func (c *Client) LeaveGroup(ctx context.Context, groupID string) error {
	req := connect.NewRequest(&groupv1.LeaveGroupRequest{GroupId: groupID})
	c.authorize(req.Header())
	if _, err := c.group.LeaveGroup(ctx, req); err != nil {
		return fmt.Errorf("vfxclient: leave group: %w", err)
	}
	return nil
}

func (c *Client) ListMyGroups(ctx context.Context) ([]*groupv1.Group, error) {
	req := connect.NewRequest(&groupv1.ListMyGroupsRequest{})
	c.authorize(req.Header())
	resp, err := c.group.ListMyGroups(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list my groups: %w", err)
	}
	return resp.Msg.GetGroups(), nil
}

func (c *Client) ListGroupMembers(ctx context.Context, groupID string) ([]*groupv1.Member, error) {
	req := connect.NewRequest(&groupv1.ListMembersRequest{GroupId: groupID})
	c.authorize(req.Header())
	resp, err := c.group.ListMembers(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: list group members: %w", err)
	}
	return resp.Msg.GetMembers(), nil
}
