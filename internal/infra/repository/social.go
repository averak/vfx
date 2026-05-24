package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/social"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

type FriendRequests struct{}

var _ social.FriendRequestRepository = (*FriendRequests)(nil)

func NewFriendRequests() *FriendRequests {
	return &FriendRequests{}
}

func (FriendRequests) Save(ctx context.Context, r *social.FriendRequest) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).CreateFriendRequest(ctx, dbgen.CreateFriendRequestParams{
		ID:          uuid.New(),
		RequesterID: r.Requester,
		AddresseeID: r.Addressee,
	})
}

func (FriendRequests) Delete(ctx context.Context, requester, addressee uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	affected, err := dbgen.New(tx).DeleteFriendRequest(ctx, dbgen.DeleteFriendRequestParams{RequesterID: requester, AddresseeID: addressee})
	if err != nil {
		return err
	}
	if affected == 0 {
		return social.ErrRequestNotFound
	}
	return nil
}

func (FriendRequests) Exists(ctx context.Context, requester, addressee uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).FriendRequestExists(ctx, dbgen.FriendRequestExistsParams{RequesterID: requester, AddresseeID: addressee})
}

func (FriendRequests) ListIncoming(ctx context.Context, addressee uuid.UUID) ([]*social.PendingRequest, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListIncomingRequests(ctx, addressee)
	if err != nil {
		return nil, err
	}
	out := make([]*social.PendingRequest, len(rows))
	for i, row := range rows {
		out[i] = &social.PendingRequest{PlayerID: row.ID, Nickname: row.Nickname, RequestedAt: row.CreatedAt.Time}
	}
	return out, nil
}

func (FriendRequests) ListOutgoing(ctx context.Context, requester uuid.UUID) ([]*social.PendingRequest, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListOutgoingRequests(ctx, requester)
	if err != nil {
		return nil, err
	}
	out := make([]*social.PendingRequest, len(rows))
	for i, row := range rows {
		out[i] = &social.PendingRequest{PlayerID: row.ID, Nickname: row.Nickname, RequestedAt: row.CreatedAt.Time}
	}
	return out, nil
}

type Friendships struct{}

var _ social.FriendshipRepository = (*Friendships)(nil)

func NewFriendships() *Friendships {
	return &Friendships{}
}

func (Friendships) Save(ctx context.Context, f *social.Friendship) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).CreateFriendship(ctx, dbgen.CreateFriendshipParams{
		ID:         uuid.New(),
		PlayerLow:  f.Low,
		PlayerHigh: f.High,
	})
}

func (Friendships) Delete(ctx context.Context, a, b uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	low, high := social.OrderPair(a, b)
	affected, err := dbgen.New(tx).DeleteFriendship(ctx, dbgen.DeleteFriendshipParams{PlayerLow: low, PlayerHigh: high})
	if err != nil {
		return err
	}
	if affected == 0 {
		return social.ErrNotFriends
	}
	return nil
}

func (Friendships) Exists(ctx context.Context, a, b uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	low, high := social.OrderPair(a, b)
	return dbgen.New(tx).FriendshipExists(ctx, dbgen.FriendshipExistsParams{PlayerLow: low, PlayerHigh: high})
}

func (Friendships) ListFriends(ctx context.Context, playerID uuid.UUID) ([]*social.Friend, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListFriends(ctx, playerID)
	if err != nil {
		return nil, err
	}
	out := make([]*social.Friend, len(rows))
	for i, row := range rows {
		out[i] = &social.Friend{PlayerID: row.ID, Nickname: row.Nickname, Since: row.CreatedAt.Time}
	}
	return out, nil
}

type Blocks struct{}

var _ social.BlockRepository = (*Blocks)(nil)

func NewBlocks() *Blocks {
	return &Blocks{}
}

func (Blocks) Save(ctx context.Context, b *social.Block) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).BlockPlayer(ctx, dbgen.BlockPlayerParams{
		ID:        uuid.New(),
		BlockerID: b.Blocker,
		BlockedID: b.Blocked,
	})
}

func (Blocks) Delete(ctx context.Context, blocker, blocked uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).UnblockPlayer(ctx, dbgen.UnblockPlayerParams{BlockerID: blocker, BlockedID: blocked})
}

func (Blocks) IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).BlockExistsEitherWay(ctx, dbgen.BlockExistsEitherWayParams{BlockerID: a, BlockedID: b})
}

func (Blocks) ListBlocked(ctx context.Context, blocker uuid.UUID) ([]*social.BlockedPlayer, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListBlocked(ctx, blocker)
	if err != nil {
		return nil, err
	}
	out := make([]*social.BlockedPlayer, len(rows))
	for i, row := range rows {
		out[i] = &social.BlockedPlayer{PlayerID: row.ID, Nickname: row.Nickname, BlockedAt: row.CreatedAt.Time}
	}
	return out, nil
}
