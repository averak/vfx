package match

import (
	"time"

	"github.com/google/uuid"
)

// Assignment is the matchmaker's output: which match the player ended
// up in and how to connect to it.
type Assignment struct {
	MatchID      uuid.UUID
	Endpoint     string
	SessionToken string
	ExpiresAt    time.Time
}
