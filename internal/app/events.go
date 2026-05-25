package app

import (
	"context"
	"time"
)

func newEvent(kind RunKind, eventType EventType) Event {
	return Event{
		Kind:      kind,
		Type:      eventType,
		Timestamp: time.Now(),
	}
}

func emitEvent(ctx context.Context, ch chan<- Event, event Event) error {
	select {
	case ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
