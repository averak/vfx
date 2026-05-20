package match

import "time"

// Event is the union of states a Ticket can be observed in by a
// WatchTicket subscriber.
type Event interface {
	isEvent()
}

// EventQueued reports that the ticket is waiting in the queue.
type EventQueued struct {
	QueuedAt   time.Time
	QueueDepth int32
}

func (EventQueued) isEvent() {}

// EventMatched reports that the matchmaker has paired the ticket with
// others and reserved a room.
type EventMatched struct {
	Assignment *Assignment
}

func (EventMatched) isEvent() {}

// EventFailed reports a terminal failure (timeout, cancel, internal).
type EventFailed struct {
	Reason  string
	Message string
}

func (EventFailed) isEvent() {}
