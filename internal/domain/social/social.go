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
	ErrAlreadyFriends   = errors.New("social: already friends")
	ErrAlreadyRequested = errors.New("social: a request to this player is already pending")
	ErrRequestNotFound  = errors.New("social: no such pending request")
	ErrNotFriends       = errors.New("social: not friends")
)

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
