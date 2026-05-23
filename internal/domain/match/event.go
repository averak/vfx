package match

import "time"

// Event is a sealed union of the states a WatchTicket subscriber observes.
type Event interface {
	isEvent()
}

type EventQueued struct {
	QueuedAt   time.Time
	QueueDepth int32
}

func (EventQueued) isEvent() {}

type EventMatched struct {
	Assignment *Assignment
}

func (EventMatched) isEvent() {}

// EventFailed is terminal: timeout, cancel, or internal error.
type EventFailed struct {
	Reason  string
	Message string
}

func (EventFailed) isEvent() {}
