package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthLoginRejectsUnsupportedModeForProvider(t *testing.T) {
	t.Run("openai rejects oauth", func(t *testing.T) {
		cmd := NewAuthCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"login", "openai"})

		err := cmd.Execute()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "openai does not support OAuth auth")
	})

	t.Run("codex rejects api key", func(t *testing.T) {
		cmd := NewAuthCommand()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"login", "codex", "--api-key", "--key", "sk-test"})

		err := cmd.Execute()

		require.Error(t, err)
		assert.Contains(t, err.Error(), "codex does not support API-key auth")
	})
}

func TestAuthLoginOpenAIStoresAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := NewAuthCommand()
	output := &bytes.Buffer{}
	cmd.SetOut(output)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"login", "openai", "--api-key", "--key", "sk-test"})

	err := cmd.Execute()

	require.NoError(t, err)
	assert.Contains(t, output.String(), "Stored OpenAI API key")
}
