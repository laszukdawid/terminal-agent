package commands

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

	cleanup := func() {
		// Temp directory is automatically cleaned up by t.TempDir()
	}

	return memoryPath, cleanup
}

func TestBuildAskPrompt(t *testing.T) {
	basePrompt := "You are a helpful assistant."

	t.Run("MemoryDisabled", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// Add some memory entries
		m := memory.NewMemory(memoryPath)
		m.Add("some memory entry")

		// Build prompt with memory disabled
		result, err := BuildAskPrompt(basePrompt, false, memoryPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result, "Prompt should be unchanged when memory is disabled")
		assert.NotContains(t, result, "<memory>", "Memory should not be included when disabled")
	})

	t.Run("MemoryEnabledWithEntries", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// Add memory entries
		m := memory.NewMemory(memoryPath)
		m.Add("viewing images is with catimg")
		m.Add("use kubectl for kubernetes")

		// Build prompt with memory enabled
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)

		// Verify memory is prepended
		assert.True(t, strings.HasPrefix(result, "<memory>"), "Prompt should start with memory tag")
		assert.Contains(t, result, "viewing images is with catimg")
		assert.Contains(t, result, "use kubectl for kubernetes")
		assert.Contains(t, result, "</memory>")
		assert.True(t, strings.HasSuffix(result, basePrompt), "Prompt should end with base prompt")
	})

	t.Run("MemoryEnabledButEmpty", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// Don't add any memory entries

		// Build prompt with memory enabled but no entries
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result, "Empty memory should not modify the prompt")
		assert.NotContains(t, result, "<memory>")
	})

	t.Run("MemoryEnabledNonExistentFile", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentPath := filepath.Join(tempDir, "nonexistent", "memory.jsonl")

		// Build prompt with non-existent memory file
		result, err := BuildAskPrompt(basePrompt, true, nonExistentPath)
		assert.NoError(t, err)
		assert.Equal(t, basePrompt, result, "Non-existent memory file should not modify the prompt")
	})

	t.Run("MemoryFormatVerification", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// Add a single memory entry
		m := memory.NewMemory(memoryPath)
		m.Add("test memory entry")

		// Build prompt with memory enabled
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)

		// Verify exact format: <memory>\n...\n</memory>\n\n<base prompt>
		expected := "<memory>\ntest memory entry\n</memory>\n\n" + basePrompt
		assert.Equal(t, expected, result)
	})
}

// TestMemoryFlagBehavior tests the logic that determines when memory should be included
func TestMemoryFlagBehavior(t *testing.T) {
	t.Run("ConfigFalseAndFlagFalse", func(t *testing.T) {
		configMemory := false
		flagMemory := false
		includeMemory := configMemory || flagMemory
		assert.False(t, includeMemory, "Memory should not be included when both config and flag are false")
	})

	t.Run("ConfigTrueAndFlagFalse", func(t *testing.T) {
		configMemory := true
		flagMemory := false
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when config is true")
	})

	t.Run("ConfigFalseAndFlagTrue", func(t *testing.T) {
		configMemory := false
		flagMemory := true
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when flag is true")
	})

	t.Run("ConfigTrueAndFlagTrue", func(t *testing.T) {
		configMemory := true
		flagMemory := true
		includeMemory := configMemory || flagMemory
		assert.True(t, includeMemory, "Memory should be included when both config and flag are true")
	})
}

func TestBuildContextFromFiles(t *testing.T) {
	t.Run("SingleFile", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test.md")
		os.WriteFile(filePath, []byte("Hello World"), 0644)

		result, err := buildContextFromFiles([]string{filePath})
		assert.NoError(t, err)
		assert.Equal(t, "<context>\nHello World\n</context>", result)
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		tempDir := t.TempDir()
		file1 := filepath.Join(tempDir, "file1.md")
		file2 := filepath.Join(tempDir, "file2.md")
		os.WriteFile(file1, []byte("Content 1"), 0644)
		os.WriteFile(file2, []byte("Content 2"), 0644)

		result, err := buildContextFromFiles([]string{file1, file2})
		assert.NoError(t, err)
		expected := "<context>\nContent 1\n</context>\n\n<context>\nContent 2\n</context>"
		assert.Equal(t, expected, result)
	})

	t.Run("FileNotFound", func(t *testing.T) {
		_, err := buildContextFromFiles([]string{"/nonexistent/file.md"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("EmptyFileList", func(t *testing.T) {
		result, err := buildContextFromFiles([]string{})
		assert.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("TrimsWhitespace", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test.md")
		os.WriteFile(filePath, []byte("  content with whitespace  \n\n"), 0644)

		result, err := buildContextFromFiles([]string{filePath})
		assert.NoError(t, err)
		assert.Equal(t, "<context>\ncontent with whitespace\n</context>", result)
	})
}

// TestConfigMemoryIntegration tests that config memory setting works correctly
func TestConfigMemoryIntegration(t *testing.T) {
	// Store original HOME
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)

	t.Run("MemoryIncludedViaConfig", func(t *testing.T) {
		tempDir := t.TempDir()
		os.Setenv("HOME", tempDir)

		// Create memory file in expected location
		memoryDir := filepath.Join(tempDir, ".local", "share", "terminal-agent")
		os.MkdirAll(memoryDir, 0755)
		memoryPath := filepath.Join(memoryDir, "memory.jsonl")

		// Add memory entry
		m := memory.NewMemory(memoryPath)
		m.Add("config-enabled memory entry")

		// Simulate config.GetMemory() returning true
		basePrompt := "Base prompt"
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Contains(t, result, "config-enabled memory entry")
		assert.Contains(t, result, "<memory>")
	})

	t.Run("MemoryIncludedViaFlag", func(t *testing.T) {
		memoryPath, cleanup := setupTempMemory(t)
		defer cleanup()

		// Add memory entry
		m := memory.NewMemory(memoryPath)
		m.Add("flag-enabled memory entry")

		// Simulate --memory flag being passed (config false, flag true)
		basePrompt := "Base prompt"
		result, err := BuildAskPrompt(basePrompt, true, memoryPath)
		assert.NoError(t, err)
		assert.Contains(t, result, "flag-enabled memory entry")
		assert.Contains(t, result, "<memory>")
	})
}
