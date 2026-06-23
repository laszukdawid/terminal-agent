package agent

import (
	"context"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	utils "github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// externalSchemaTool is a schemaOutputTool that reports itself external-facing.
type externalSchemaTool struct {
	*schemaOutputTool
}

func (t *externalSchemaTool) ExternalFacing() bool { return true }

func TestTaskWithOptionsResultStopsAtTokenBudget(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	sysPrompt := "task system prompt"
	// No scripted responses: the connector returns empty tool-less responses, so
	// the loop keeps running until a budget bound stops it.
	conn := &scriptedToolConnector{}
	agentInstance := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agentInstance.TaskWithOptionsResult(context.Background(), "do something useful", TaskOptions{
		Interaction: &fakeTaskInteraction{},
		TokenBudget: 1,
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.ErrorIs(t, err, ErrTokenBudgetExceeded)
	assert.Greater(t, result.TokensUsed, 0, "token usage should be reported even on budget stop")
}

func TestTaskWithOptionsResultRespectsConfiguredStepBudget(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	sysPrompt := "task system prompt"
	conn := &scriptedToolConnector{queryResponse: "final summary"}
	agentInstance := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agentInstance.TaskWithOptionsResult(context.Background(), "keep thinking", TaskOptions{
		Interaction: &fakeTaskInteraction{},
		MaxTurns:    2,
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, 2, conn.queryToolCalls, "MaxTurns should bound the number of model turns")
	assert.Equal(t, "final summary", result.Response)
}

func TestBuildTaskToolsExternalFacingPolicy(t *testing.T) {
	agentInstance := &Agent{
		Tools: map[string]tools.Tool{
			"local": &schemaOutputTool{name: "local"},
			"ext":   &externalSchemaTool{schemaOutputTool: &schemaOutputTool{name: "ext"}},
		},
	}
	interaction := &fakeTaskInteraction{}

	t.Run("routine default policy disables external-facing tools", func(t *testing.T) {
		got := agentInstance.buildTaskTools(interaction, nil, true)
		assert.Contains(t, got, "local")
		assert.NotContains(t, got, "ext")
		// Task-only tools are always present.
		assert.Contains(t, got, ToolNameFinalAnswer)
		assert.Contains(t, got, UserClarificationToolName)
	})

	t.Run("interactive task default keeps external-facing tools", func(t *testing.T) {
		got := agentInstance.buildTaskTools(interaction, nil, false)
		assert.Contains(t, got, "local")
		assert.Contains(t, got, "ext", "normal task runs must not lose external tools")
	})

	t.Run("explicit allow-list opts external tool back in", func(t *testing.T) {
		got := agentInstance.buildTaskTools(interaction, []string{"ext"}, true)
		assert.Contains(t, got, "ext")
		assert.NotContains(t, got, "local")
	})

	t.Run("explicit allow-list keeps only named local tool", func(t *testing.T) {
		got := agentInstance.buildTaskTools(interaction, []string{"local"}, true)
		assert.Contains(t, got, "local")
		assert.NotContains(t, got, "ext")
	})
}

func TestTaskOptionsDenyBlocksAutoApprovedTool(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	sysPrompt := "task system prompt"
	unixTool := &schemaOutputTool{
		name:     tools.ToolNameUnix,
		category: tools.PermissionExecute,
		output:   "SHOULD NOT RUN",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
		},
	}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{ToolUse: true, ToolName: tools.ToolNameUnix, ToolInput: map[string]any{"command": "rm -rf tmp"}},
		{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
	}}
	agentInstance := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{tools.ToolNameUnix: unixTool},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agentInstance.TaskWithOptionsResult(context.Background(), "clean up", TaskOptions{
		Interaction: &fakeTaskInteraction{},
		AutoApprove: true,
		Deny:        []string{`unix("rm -rf tmp")`},
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	assert.Empty(t, unixTool.inputs, "denied tool must not execute even under auto-approve")
}

func TestUnattendedInteraction(t *testing.T) {
	t.Run("default clarification keeps the run moving", func(t *testing.T) {
		answer, err := UnattendedInteraction{}.Clarify(TaskClarificationRequest{Question: "which dir?"})
		require.NoError(t, err)
		assert.Equal(t, DefaultUnattendedClarification, answer)
	})

	t.Run("custom clarification response", func(t *testing.T) {
		answer, err := UnattendedInteraction{ClarificationResponse: "use /tmp"}.Clarify(TaskClarificationRequest{})
		require.NoError(t, err)
		assert.Equal(t, "use /tmp", answer)
	})

	t.Run("confirm approves", func(t *testing.T) {
		decision, err := UnattendedInteraction{}.Confirm(TaskConfirmationRequest{Action: `unix("ls")`})
		require.NoError(t, err)
		assert.True(t, decision.Allowed)
	})
}
