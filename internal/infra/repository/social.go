package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/social"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// Social is the storage implementation of [social.Repository].
type Social struct{}

var _ social.Repository = (*Social)(nil)

func NewSocial() *Social {
	return &Social{}
}

func (Social) CreateRequest(ctx context.Context, requester, addressee uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).CreateFriendRequest(ctx, dbgen.CreateFriendRequestParams{
		ID:          uuid.New(),
		RequesterID: requester,
		AddresseeID: addressee,
		CreatedAt:   toTimestamptz(now),
	})
}

func (Social) RequestExists(ctx context.Context, requester, addressee uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).FriendRequestExists(ctx, dbgen.FriendRequestExistsParams{RequesterID: requester, AddresseeID: addressee})
}

func (Social) DeleteRequest(ctx context.Context, requester, addressee uuid.UUID) error {
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

func (Social) ListIncoming(ctx context.Context, addressee uuid.UUID) ([]*social.PendingRequest, error) {
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

func (Social) ListOutgoing(ctx context.Context, requester uuid.UUID) ([]*social.PendingRequest, error) {
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

func (Social) CreateFriendship(ctx context.Context, a, b uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	low, high := social.OrderPair(a, b)
	return dbgen.New(tx).CreateFriendship(ctx, dbgen.CreateFriendshipParams{
		ID:         uuid.New(),
		PlayerLow:  low,
		PlayerHigh: high,
		CreatedAt:  toTimestamptz(now),
	})
}

func (Social) AreFriends(ctx context.Context, a, b uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	low, high := social.OrderPair(a, b)
	return dbgen.New(tx).FriendshipExists(ctx, dbgen.FriendshipExistsParams{PlayerLow: low, PlayerHigh: high})
}

func (Social) DeleteFriendship(ctx context.Context, a, b uuid.UUID) error {
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

func (Social) Block(ctx context.Context, blocker, blocked uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).BlockPlayer(ctx, dbgen.BlockPlayerParams{
		ID:        uuid.New(),
		BlockerID: blocker,
		BlockedID: blocked,
		CreatedAt: toTimestamptz(now),
	})
}

func (Social) Unblock(ctx context.Context, blocker, blocked uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).UnblockPlayer(ctx, dbgen.UnblockPlayerParams{BlockerID: blocker, BlockedID: blocked})
}

func (Social) IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).BlockExistsEitherWay(ctx, dbgen.BlockExistsEitherWayParams{BlockerID: a, BlockedID: b})
}

func (Social) ListBlocked(ctx context.Context, blocker uuid.UUID) ([]*social.BlockedPlayer, error) {
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

func (Social) ListFriends(ctx context.Context, playerID uuid.UUID) ([]*social.Friend, error) {
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
