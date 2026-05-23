package commands

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolExecPlainStringUsesToolRun(t *testing.T) {
	t.Setenv("TAVILY_KEY", "")
	cmd := NewToolCommand(config.NewDefaultConfig())
	cmd.SetArgs([]string{"exec", "websearch", "golang"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "websearch requires TAVILY_KEY environment variable to be set")
}
