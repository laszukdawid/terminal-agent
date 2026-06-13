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
	assert.Contains(t, err.Error(), "outside allowed working directories")
}

func TestFileEditToolRunSchemaWithContextAllowsAdditionalRoot(t *testing.T) {
	rootDir := t.TempDir()
	extraRoot := t.TempDir()
	targetPath := filepath.Join(extraRoot, "hello.txt")

	tool := NewFileEditTool(rootDir)
	_, err := tool.RunSchemaWithContext(map[string]any{
		"path":      targetPath,
		"operation": "write",
		"content":   "hello",
	}, ToolExecutionContext{RootDir: rootDir, CurrentDir: rootDir, AllowedRootDirs: []string{extraRoot}})

	require.NoError(t, err)
	content, readErr := os.ReadFile(targetPath)
	require.NoError(t, readErr)
	assert.Equal(t, "hello", string(content))
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
	// Python's os.getcwd() resolves symlinks (e.g. macOS /var -> /private/var), so compare
	// against the resolved current dir rather than the raw t.TempDir()-derived path.
	expectedCurrentDir, evalErr := filepath.EvalSymlinks(currentDir)
	require.NoError(t, evalErr)
	assert.Equal(t, expectedCurrentDir, strings.TrimSpace(result))
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

func TestReadToolRunSchemaWithContextReadsFiles(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "hello.txt"), []byte("line one\nline two\nline three\n"), 0o644))

	tool := NewReadTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{"path": "hello.txt"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: rootDir,
	})

	require.NoError(t, err)
	assert.Contains(t, result, "1: line one")
	assert.Contains(t, result, "2: line two")
	assert.Contains(t, result, "3: line three")
}

func TestReadToolSupportsOffsetAndLimit(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "lines.txt"), []byte("a\nb\nc\nd\ne\n"), 0o644))

	tool := NewReadTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{
		"path":   "lines.txt",
		"offset": 2,
		"limit":  2,
	}, ToolExecutionContext{RootDir: rootDir, CurrentDir: rootDir})

	require.NoError(t, err)
	assert.Contains(t, result, "2: b")
	assert.Contains(t, result, "3: c")
	assert.NotContains(t, result, "1: a")
	assert.NotContains(t, result, "4: d")
}

func TestReadToolReturnsErrorForMissingFile(t *testing.T) {
	rootDir := t.TempDir()
	tool := NewReadTool(rootDir)
	_, err := tool.RunSchemaWithContext(map[string]any{"path": "missing.txt"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: rootDir,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestReadToolRejectsPathsOutsideRoot(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))

	tool := NewReadTool(rootDir)
	_, err := tool.RunSchemaWithContext(map[string]any{"path": "../../etc/passwd"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: currentDir,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside allowed working directories")
}

func TestReadToolResolvesPathsRelativeToCurrentDir(t *testing.T) {
	rootDir := t.TempDir()
	currentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(currentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(currentDir, "local.txt"), []byte("nested content"), 0o644))

	tool := NewReadTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{"path": "local.txt"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: currentDir,
	})

	require.NoError(t, err)
	assert.Contains(t, result, "1: nested content")
}

func TestReadToolHandlesEmptyFile(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "empty.txt"), []byte(""), 0o644))

	tool := NewReadTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{"path": "empty.txt"}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: rootDir,
	})

	require.NoError(t, err)
	assert.Equal(t, "(empty file)", result)
}

func TestReadToolOffsetExceedsFileLength(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "short.txt"), []byte("only one line\n"), 0o644))

	tool := NewReadTool(rootDir)
	result, err := tool.RunSchemaWithContext(map[string]any{"path": "short.txt", "offset": 5}, ToolExecutionContext{
		RootDir:    rootDir,
		CurrentDir: rootDir,
	})

	require.NoError(t, err)
	assert.Contains(t, result, "offset 5 exceeds file length")
}
