package agent

import (
	"context"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type summaryCaptureConnector struct {
	userPrompt string
	maxTokens  int
}

func (c *summaryCaptureConnector) Query(_ context.Context, params *connector.QueryParams) (string, error) {
	if params.UserPrompt != nil {
		c.userPrompt = *params.UserPrompt
	}
	c.maxTokens = params.MaxTokens
	return c.userPrompt, nil
}

func (c *summaryCaptureConnector) QueryWithTool(_ context.Context, _ *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	return connector.LlmResponseWithTools{}, nil
}

func TestFinalizeSummaryUsesRenderedTemplate(t *testing.T) {
	conn := &summaryCaptureConnector{}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.finalizeSummary(context.Background(), &TaskState{
		OriginalQuery: "review the task path",
		Results: map[string]string{
			"file_search": "internal/app/task.go",
			"analysis":    "task prompt resolution is coupled to ask prompt resolution",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, conn.userPrompt, result)
	assert.Contains(t, conn.userPrompt, "review the task path")
	assert.Contains(t, conn.userPrompt, "internal/app/task.go")
	assert.Contains(t, conn.userPrompt, "task prompt resolution is coupled to ask prompt resolution")
	assert.NotContains(t, conn.userPrompt, "{{range")
	assert.NotContains(t, conn.userPrompt, "{{$source}}")
	assert.Equal(t, MaxTokens*2, conn.maxTokens)
}
