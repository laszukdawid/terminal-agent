package commands

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/memory"
	"github.com/stretchr/testify/assert"
)

func TestMemoryListCommand(t *testing.T) {
	t.Run("ListEmptyMemory", func(t *testing.T) {
		tempDir := t.TempDir()
		memoryPath := filepath.Join(tempDir, "memory.jsonl")
		mClient := memory.NewMemory(memoryPath)

		cmd := memoryListCommand(mClient)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := cmd.Execute()
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No memory entries found.")
	})

	t.Run("ListWithEntries", func(t *testing.T) {
		tempDir := t.TempDir()
		memoryPath := filepath.Join(tempDir, "memory.jsonl")
		mClient := memory.NewMemory(memoryPath)

		// Add some entries
		mClient.Add("first entry")
		mClient.Add("second entry")

		cmd := memoryListCommand(mClient)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := buf.String()
		assert.Contains(t, output, "first entry")
		assert.Contains(t, output, "second entry")

		// Verify format: timestamp followed by content
		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Len(t, lines, 2)

		// Each line should have timestamp and content
		for _, line := range lines {
			parts := strings.SplitN(line, " ", 2)
			assert.Len(t, parts, 2, "Line should have timestamp and content")
			// Timestamp should contain date markers
			assert.Contains(t, parts[0], "-", "Timestamp should be in RFC3339 format")
			assert.Contains(t, parts[0], ":", "Timestamp should be in RFC3339 format")
		}
	})
}

func TestMemoryAddCommand(t *testing.T) {
	t.Run("AddSingleEntry", func(t *testing.T) {
		tempDir := t.TempDir()
		memoryPath := filepath.Join(tempDir, "memory.jsonl")
		mClient := memory.NewMemory(memoryPath)

		cmd := memoryAddCommand(mClient)
		cmd.SetArgs([]string{"test", "entry"})

		err := cmd.Execute()
		assert.NoError(t, err)

		entries, _ := mClient.List()
		assert.Len(t, entries, 1)
		assert.Equal(t, "test entry", entries[0].Content)
	})

	t.Run("AddQuotedEntry", func(t *testing.T) {
		tempDir := t.TempDir()
		memoryPath := filepath.Join(tempDir, "memory.jsonl")
		mClient := memory.NewMemory(memoryPath)

		cmd := memoryAddCommand(mClient)
		cmd.SetArgs([]string{"viewing images is with catimg"})

		err := cmd.Execute()
		assert.NoError(t, err)

		entries, _ := mClient.List()
		assert.Len(t, entries, 1)
		assert.Equal(t, "viewing images is with catimg", entries[0].Content)
	})
}
