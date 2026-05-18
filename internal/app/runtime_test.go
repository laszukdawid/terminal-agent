package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
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

func TestNewRuntimeReturnsConnectorInitializationErrors(t *testing.T) {
	runtime, err := NewRuntime(RuntimeRequest{
		Provider: "nope",
		Model:    "model",
		Config:   config.NewDefaultConfig(),
	})

	require.Error(t, err)
	assert.Nil(t, runtime)
	assert.EqualError(t, err, "unsupported provider: nope")
}

func TestNewRuntimeAllowsEmptyModelWhenConnectorHasDefault(t *testing.T) {
	runtime, err := NewRuntime(RuntimeRequest{
		Provider: connector.OpenaiProvider,
		Model:    "",
		Config:   config.NewDefaultConfig(),
	})

	require.NoError(t, err)
	require.NotNil(t, runtime)
	assert.IsType(t, &connector.OpenAIConnector{}, runtime.Connector)
}
