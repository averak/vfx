// Package social is the friend-graph aggregate.
//
// A friend request is directed (requester to addressee); an accepted friendship is undirected and stored once in a canonical (low, high) ordering.
package social

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrSelfFriend       = errors.New("social: cannot friend yourself")
	ErrSelfBlock        = errors.New("social: cannot block yourself")
	ErrAlreadyFriends   = errors.New("social: already friends")
	ErrAlreadyRequested = errors.New("social: a request to this player is already pending")
	ErrRequestNotFound  = errors.New("social: no such pending request")
	ErrNotFriends       = errors.New("social: not friends")
	ErrBlocked          = errors.New("social: blocked between these players")
)

// FriendRequest is an aggregate root: a directed, pending request from Requester to Addressee.
type FriendRequest struct {
	Requester uuid.UUID
	Addressee uuid.UUID
	CreatedAt time.Time
}

func NewFriendRequest(requester, addressee uuid.UUID, now time.Time) *FriendRequest {
	return &FriendRequest{Requester: requester, Addressee: addressee, CreatedAt: now}
}

// Friendship is an aggregate root: an undirected friendship stored once. NewFriendship canonicalizes the pair into (Low, High).
type Friendship struct {
	Low       uuid.UUID
	High      uuid.UUID
	CreatedAt time.Time
}

func NewFriendship(a, b uuid.UUID, now time.Time) *Friendship {
	low, high := OrderPair(a, b)
	return &Friendship{Low: low, High: high, CreatedAt: now}
}

// Block is an aggregate root: a directed block from Blocker to Blocked.
type Block struct {
	Blocker   uuid.UUID
	Blocked   uuid.UUID
	CreatedAt time.Time
}

func NewBlock(blocker, blocked uuid.UUID, now time.Time) *Block {
	return &Block{Blocker: blocker, Blocked: blocked, CreatedAt: now}
}

// Friend is a read model: an accepted friend with their display name and when the friendship formed.
type Friend struct {
	PlayerID uuid.UUID
	Nickname *string
	Since    time.Time
}

// PendingRequest is a read model: the other party of a pending request and when it was sent.
type PendingRequest struct {
	PlayerID    uuid.UUID
	Nickname    *string
	RequestedAt time.Time
}

// BlockedPlayer is a read model: a player the caller has blocked and when.
type BlockedPlayer struct {
	PlayerID  uuid.UUID
	Nickname  *string
	BlockedAt time.Time
}

// OrderPair returns the two ids in canonical order (low, high) so an undirected friendship has a single representation.
func OrderPair(a, b uuid.UUID) (low, high uuid.UUID) {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return a, b
		case a[i] > b[i]:
			return b, a
		}
	}
	return a, b
}
