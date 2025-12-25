package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupTempMemory(t *testing.T) (string, func()) {
	tempDir := t.TempDir()
	memoryPath := filepath.Join(tempDir, "memory.jsonl")

	cleanup := func() {
		// Temp directory is automatically cleaned up by t.TempDir()
	}

	return memoryPath, cleanup
}

func TestMemoryAdd(t *testing.T) {
	t.Run("AddNewEntry", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)

		err := m.Add("test entry")
		assert.NoError(t, err)

		entries, err := m.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, "test entry", entries[0].Content)
		assert.NotEmpty(t, entries[0].Timestamp)
	})

	t.Run("AddDuplicateEntry", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)

		err := m.Add("duplicate entry")
		assert.NoError(t, err)

		err = m.Add("duplicate entry")
		assert.NoError(t, err)

		entries, err := m.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 1, "Duplicate entry should be silently skipped")
	})

	t.Run("AddMultipleEntries", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)

		err := m.Add("first entry")
		assert.NoError(t, err)

		err = m.Add("second entry")
		assert.NoError(t, err)

		entries, err := m.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Equal(t, "first entry", entries[0].Content)
		assert.Equal(t, "second entry", entries[1].Content)
	})
}

func TestMemoryList(t *testing.T) {
	t.Run("ListEmptyMemory", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)

		entries, err := m.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 0)
	})

	t.Run("ListNonExistentFile", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentPath := filepath.Join(tempDir, "nonexistent", "memory.jsonl")

		m := NewMemory(nonExistentPath)

		entries, err := m.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 0)
	})
}

func TestMemoryFormatAsPrompt(t *testing.T) {
	t.Run("FormatEmptyMemory", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)

		prompt, err := m.FormatAsPrompt()
		assert.NoError(t, err)
		assert.Empty(t, prompt, "Empty memory should return empty prompt")
	})

	t.Run("FormatSingleEntry", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)
		m.Add("viewing images is with catimg")

		prompt, err := m.FormatAsPrompt()
		assert.NoError(t, err)
		assert.Equal(t, "<memory>\nviewing images is with catimg\n</memory>", prompt)
	})

	t.Run("FormatMultipleEntries", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := NewMemory(memoryPath)
		m.Add("first thing to remember")
		m.Add("second thing to remember")
		m.Add("third thing to remember")

		prompt, err := m.FormatAsPrompt()
		assert.NoError(t, err)
		expected := "<memory>\nfirst thing to remember\nsecond thing to remember\nthird thing to remember\n</memory>"
		assert.Equal(t, expected, prompt)
	})

	t.Run("FormatNonExistentFile", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentPath := filepath.Join(tempDir, "nonexistent", "memory.jsonl")

		m := NewMemory(nonExistentPath)

		prompt, err := m.FormatAsPrompt()
		assert.NoError(t, err)
		assert.Empty(t, prompt, "Non-existent memory file should return empty prompt")
	})
}

func TestMemoryPersistence(t *testing.T) {
	t.Run("PersistAcrossInstances", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// First instance adds entries
		m1 := NewMemory(memoryPath)
		m1.Add("persistent entry 1")
		m1.Add("persistent entry 2")

		// Second instance should see the entries
		m2 := NewMemory(memoryPath)
		entries, err := m2.List()
		assert.NoError(t, err)
		assert.Len(t, entries, 2)
		assert.Equal(t, "persistent entry 1", entries[0].Content)
		assert.Equal(t, "persistent entry 2", entries[1].Content)
	})

	t.Run("FileCreatedOnFirstAdd", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// File should not exist initially
		_, err := os.Stat(memoryPath)
		assert.True(t, os.IsNotExist(err))

		m := NewMemory(memoryPath)
		m.Add("first entry")

		// File should exist after add
		_, err = os.Stat(memoryPath)
		assert.NoError(t, err)
	})
}
