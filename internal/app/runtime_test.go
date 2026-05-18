package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTaskPromptIgnoresAskPrompt(t *testing.T) {
	workingDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "ask", "system.prompt"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(workingDir, "task"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workingDir, "task", "system.prompt"), []byte("task prompt"), 0644))

	runtime := &Runtime{WorkingDir: workingDir}
	prompt, err := runtime.ResolveTaskPrompt("")

	require.NoError(t, err)
	assert.Equal(t, "task prompt", prompt)
}
