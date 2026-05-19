package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSearchToolRunSchemaWithContextUsesCurrentDir(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(currentDir, "match.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "root.txt"), []byte("root"), 0o644))

	tool := NewFileSearchTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{"name_pattern": "*.txt"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: currentDir,
	})

	require.NoError(t, err)
	assert.Equal(t, "match.txt", strings.TrimSpace(result))
}

func TestFileEditToolRunSchemaWithContextResolvesPathsFromCurrentDir(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))

	tool := NewFileEditTool(rootDir)
	_, err := tool.RunSchemaWithContext(map[string]any{
		"path":      "note.txt",
		"operation": "write",
		"content":   "hello",
	}, ToolExecutionContext{RootDir: rootDir, CurrentDir: currentDir})

	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(currentDir, "note.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestFileEditToolRunSchemaWithContextRejectsPathsOutsideRoot(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))

	tool := NewFileEditTool(rootDir)
	_, err := tool.RunSchemaWithContext(map[string]any{
		"path":      "../../escape.txt",
		"operation": "write",
		"content":   "hello",
	}, ToolExecutionContext{RootDir: rootDir, CurrentDir: currentDir})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside working directory")
}

func TestPythonToolRunSchemaWithContextUsesCurrentDir(t *testing.T) {
	if _, err := choosePythonRunner("auto"); err != nil {
		t.Skip("python runner not available")
	}

	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))

	tool := NewPythonTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{
		"code": "import os; print(os.getcwd())",
	}, ToolExecutionContext{RootDir: rootDir, CurrentDir: currentDir})

	require.NoError(t, err)
	assert.Equal(t, currentDir, strings.TrimSpace(result))
}

func TestUnixToolRunSchemaWithContextUsesCurrentDir(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))

	tool := NewUnixTool(nil)
	result, err := tool.RunSchemaWithContext(map[string]any{"command": "pwd"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: currentDir,
	})

	require.NoError(t, err)
	assert.Equal(t, currentDir, strings.TrimSpace(result))
}
