package app

import (
	"context"
	"os/user"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/sessionlog"
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

func buildMeta(kind, provider, model, cwd, command string) sessionlog.Meta {
	username := ""
	if u, err := user.Current(); err == nil {
		username = strings.TrimSpace(u.Username)
	}
	return sessionlog.Meta{
		Kind:     kind,
		User:     username,
		Cwd:      cwd,
		Provider: provider,
		Model:    model,
		Command:  command,
	}
}
