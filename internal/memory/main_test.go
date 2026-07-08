package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMemoryListRobustReads(t *testing.T) {
	const first = `{"content":"first","timestamp":"2024-01-01T00:00:00Z"}`
	const second = `{"content":"second","timestamp":"2024-01-02T00:00:00Z"}`
	bigContent := strings.Repeat("y", 200*1024) // > default 64 KB scanner limit
	bigLine := `{"content":"` + bigContent + `","timestamp":"2024-01-03T00:00:00Z"}`

	tests := []struct {
		name         string
		lines        []string
		wantContents []string
	}{
		{
			name:         "all valid",
			lines:        []string{first, second},
			wantContents: []string{"first", "second"},
		},
		{
			name:         "corrupt json line mid-file is skipped",
			lines:        []string{first, "{broken", second},
			wantContents: []string{"first", "second"},
		},
		{
			name:         "truncated last line is skipped",
			lines:        []string{first, second, `{"content":"trunc`},
			wantContents: []string{"first", "second"},
		},
		{
			name:         "blank lines are ignored",
			lines:        []string{first, "", "  ", second},
			wantContents: []string{"first", "second"},
		},
		{
			name:         "entry larger than 64KB is read",
			lines:        []string{first, bigLine},
			wantContents: []string{"first", bigContent},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "memory.jsonl")
			require.NoError(t, os.WriteFile(path, []byte(strings.Join(tc.lines, "\n")), 0o644))

			m := NewMemory(path)
			entries, err := m.List()
			require.NoError(t, err)

			gotContents := make([]string, 0, len(entries))
			for _, e := range entries {
				gotContents = append(gotContents, e.Content)
			}
			assert.Equal(t, tc.wantContents, gotContents)
		})
	}
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
