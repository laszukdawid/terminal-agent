package app

import "time"

func newEvent(kind RunKind, eventType EventType) Event {
	return Event{
		Kind:      kind,
		Type:      eventType,
		Timestamp: time.Now(),
	}
}

func emitEvent(ch chan<- Event, event Event) error {
	ch <- event
	return nil
}
