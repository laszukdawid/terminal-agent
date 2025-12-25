package chat

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionStore(t *testing.T) {
	t.Run("CreateNewSession", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		store, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		sessionID, err := store.NewSession()
		require.NoError(t, err)
		assert.Equal(t, int64(1), sessionID)
		assert.Equal(t, int64(1), store.CurrentSessionID())
	})

	t.Run("GetOrCreateSession", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		store, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		// First call should create a new session
		sessionID1, err := store.GetOrCreateSession()
		require.NoError(t, err)
		assert.Equal(t, int64(1), sessionID1)

		// Second call should return the same session
		sessionID2, err := store.GetOrCreateSession()
		require.NoError(t, err)
		assert.Equal(t, sessionID1, sessionID2)
	})

	t.Run("AddAndGetMessages", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		store, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		_, err = store.NewSession()
		require.NoError(t, err)

		// Add messages
		err = store.AddMessage("user", "Hello, how are you?")
		require.NoError(t, err)
		err = store.AddMessage("assistant", "I'm doing well, thanks!")
		require.NoError(t, err)

		// Get messages
		messages, err := store.GetMessages()
		require.NoError(t, err)
		assert.Len(t, messages, 2)
		assert.Equal(t, "user", messages[0].Role)
		assert.Equal(t, "Hello, how are you?", messages[0].Content)
		assert.Equal(t, "assistant", messages[1].Role)
		assert.Equal(t, "I'm doing well, thanks!", messages[1].Content)
	})

	t.Run("NewSessionClearsMessages", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		store, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		// Create first session with messages
		_, err = store.NewSession()
		require.NoError(t, err)
		err = store.AddMessage("user", "First session message")
		require.NoError(t, err)

		// Create new session
		_, err = store.NewSession()
		require.NoError(t, err)

		// New session should have no messages
		messages, err := store.GetMessages()
		require.NoError(t, err)
		assert.Len(t, messages, 0)
	})

	t.Run("PersistSessionAcrossReopens", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		// First open: create session and add message
		store1, err := NewSessionStore(dbPath)
		require.NoError(t, err)

		_, err = store1.NewSession()
		require.NoError(t, err)
		err = store1.AddMessage("user", "Persisted message")
		require.NoError(t, err)
		store1.Close()

		// Second open: should find existing session
		store2, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store2.Close()

		messages, err := store2.GetMessages()
		require.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Equal(t, "Persisted message", messages[0].Content)
	})

	t.Run("NoActiveSession", func(t *testing.T) {
		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test.db")

		store, err := NewSessionStore(dbPath)
		require.NoError(t, err)
		defer store.Close()

		// Should fail to add message without a session
		err = store.AddMessage("user", "No session")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no active session")
	})
}
