package app

import (
	"context"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/agent"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAskEventsRejectsEmptyMessage(t *testing.T) {
	service := NewService()
	_, err := service.AskEvents(context.Background(), AskRequest{Message: "   "})
	require.ErrorIs(t, err, agent.ErrEmptyQuery)
}

func TestChatEventsRejectsEmptyMessage(t *testing.T) {
	service := NewService()
	_, err := service.ChatEvents(context.Background(), ChatRequest{Message: "   "})
	require.ErrorIs(t, err, agent.ErrEmptyQuery)
}

func TestTaskEventsRejectsEmptyMessage(t *testing.T) {
	service := NewService()
	_, err := service.TaskEvents(context.Background(), TaskRequest{Message: "   "})
	require.ErrorIs(t, err, agent.ErrEmptyQuery)
}

func TestAskEventsEmitsLifecycle(t *testing.T) {
	service := NewService()
	cfg := config.NewDefaultConfig()

	events, err := service.AskEvents(context.Background(), AskRequest{
		Message:  "hello",
		Provider: "ollama",
		Model:    "test-model",
		Config:   cfg,
	})
	require.NoError(t, err)

	var got []Event
	for event := range events {
		got = append(got, event)
	}

	require.Len(t, got, 2)
	assert.Equal(t, EventStarted, got[0].Type)
	assert.Equal(t, RunKindAsk, got[0].Kind)
	assert.Equal(t, EventFailed, got[1].Type)
	assert.Equal(t, RunKindAsk, got[1].Kind)
	assert.Error(t, got[1].Err)
}

func TestChatEventsEmitsLifecycle(t *testing.T) {
	service := NewService()
	cfg := config.NewDefaultConfig()

	events, err := service.ChatEvents(context.Background(), ChatRequest{
		Message:    "hello",
		Provider:   "ollama",
		Model:      "test-model",
		ChatDBPath: t.TempDir() + "/chat.db",
		Config:     cfg,
	})
	require.NoError(t, err)

	var got []Event
	for event := range events {
		got = append(got, event)
	}

	require.Len(t, got, 2)
	assert.Equal(t, EventStarted, got[0].Type)
	assert.Equal(t, RunKindChat, got[0].Kind)
	assert.Equal(t, EventFailed, got[1].Type)
	assert.Equal(t, RunKindChat, got[1].Kind)
	assert.Error(t, got[1].Err)
}

func TestEmitEventReturnsContextErrorWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Event)
	errCh := make(chan error, 1)

	go func() {
		errCh <- emitEvent(ctx, ch, newEvent(RunKindTask, EventStarted))
	}()

	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for emitEvent to return")
	}
}
