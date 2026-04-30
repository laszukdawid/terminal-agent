package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/chat"
	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatPersistsConversationHistory(t *testing.T) {
	service := NewService()
	dbPath := filepath.Join(t.TempDir(), "chat.db")

	_, err := service.Chat(context.Background(), ChatRequest{
		Message:    "first",
		Provider:   "google",
		Model:      "test-model",
		ChatDBPath: dbPath,
		Config:     config.NewDefaultConfig(),
	})
	require.Error(t, err)

	store, err := chat.NewSessionStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	_, err = store.GetOrCreateSession()
	require.NoError(t, err)

	messages, err := store.GetMessages()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "first", messages[0].Content)
}
