package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/memory"
	"github.com/stretchr/testify/assert"
)

func setupTempMemory(t *testing.T) (string, func()) {
	tempDir := t.TempDir()
	memoryPath := filepath.Join(tempDir, "memory.jsonl")

	cleanup := func() {}

	return memoryPath, cleanup
}

func TestBuildAskPrompt(t *testing.T) {
	basePrompt := "You are a helpful assistant."

	t.Run("MemoryDisabled", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := memory.NewMemory(memoryPath)
		m.Add("some memory entry")

		result, err := BuildAskPrompt(basePrompt, false, memoryPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result)
		assert.NotContains(t, result, "<memory>")
	})

	t.Run("MemoryEnabledWithEntries", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := memory.NewMemory(memoryPath)
		m.Add("viewing images is with catimg")
		m.Add("use kubectl for kubernetes")

		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.True(t, strings.HasPrefix(result, "<memory>"))
		assert.Contains(t, result, "viewing images is with catimg")
		assert.Contains(t, result, "use kubectl for kubernetes")
		assert.Contains(t, result, "</memory>")
		assert.True(t, strings.HasSuffix(result, basePrompt))
	})

	t.Run("MemoryEnabledButEmpty", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result)
		assert.NotContains(t, result, "<memory>")
	})

	t.Run("MemoryEnabledNonExistentFile", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentPath := filepath.Join(tempDir, "nonexistent", "memory.jsonl")

		result, err := BuildAskPrompt(basePrompt, true, nonExistentPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result)
	})

	t.Run("MemoryFormatVerification", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := memory.NewMemory(memoryPath)
		m.Add("test memory entry")

		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		expected := "<memory>\ntest memory entry\n</memory>\n\n" + basePrompt
		assert.Equal(t, expected, result)
	})
}

func TestBuildContextFromFiles(t *testing.T) {
	t.Run("SingleFile", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test.md")
		os.WriteFile(filePath, []byte("Hello World"), 0644)

		result, err := BuildContextFromFiles([]string{filePath})
		assert.NoError(t, err)
		assert.Equal(t, "<context>\nHello World\n</context>", result)
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		file1 := filepath.Join(tempDir, "file1.md")
		file2 := filepath.Join(tempDir, "file2.md")
		os.WriteFile(file1, []byte("Content 1"), 0644)
		os.WriteFile(file2, []byte("Content 2"), 0644)

		result, err := BuildContextFromFiles([]string{file1, file2})
		assert.NoError(t, err)
		expected := "<context>\nContent 1\n</context>\n\n<context>\nContent 2\n</context>"
		assert.Equal(t, expected, result)
	})

	t.Run("FileNotFound", func(t *testing.T) {
		_, err := BuildContextFromFiles([]string{"/nonexistent/file.md"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("EmptyFileList", func(t *testing.T) {
		result, err := BuildContextFromFiles([]string{})
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("TrimsWhitespace", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test.md")
		os.WriteFile(filePath, []byte("  content with whitespace  \n\n"), 0644)

		result, err := BuildContextFromFiles([]string{filePath})
		assert.NoError(t, err)
		assert.Equal(t, "<context>\ncontent with whitespace\n</context>", result)
	})
}

func TestConfigMemoryIntegration(t *testing.T) {
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	t.Run("MemoryIncludedViaConfig", func(t *testing.T) {
		tempDir := t.TempDir()
		os.Setenv("HOME", tempDir)

		memoryDir := filepath.Join(tempDir, ".local", "share", "terminal-agent")
		os.MkdirAll(memoryDir, 0755)
		memoryPath := filepath.Join(memoryDir, "memory.jsonl")

		m := memory.NewMemory(memoryPath)
		m.Add("config-enabled memory entry")

		basePrompt := "Base prompt"
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Contains(t, result, "config-enabled memory entry")
		assert.Contains(t, result, "<memory>")
	})

	t.Run("MemoryIncludedViaFlag", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		m := memory.NewMemory(memoryPath)
		m.Add("flag-enabled memory entry")

		basePrompt := "Base prompt"
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Contains(t, result, "flag-enabled memory entry")
		assert.Contains(t, result, "<memory>")
	})
}
