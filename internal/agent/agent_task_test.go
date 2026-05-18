package agent

import (
	"context"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedOutputTool struct {
	name   string
	output string
}

func (t *fixedOutputTool) Name() string                             { return t.name }
func (t *fixedOutputTool) Description() string                      { return "" }
func (t *fixedOutputTool) InputSchema() map[string]any              { return map[string]any{} }
func (t *fixedOutputTool) HelpText() string                         { return "" }
func (t *fixedOutputTool) RunSchema(map[string]any) (string, error) { return t.output, nil }
func (t *fixedOutputTool) Run(*string) (string, error)              { return t.output, nil }

type schemaOutputTool struct {
	name   string
	output string
	schema map[string]any
	inputs []map[string]any
}

func (t *schemaOutputTool) Name() string                { return t.name }
func (t *schemaOutputTool) Description() string         { return "" }
func (t *schemaOutputTool) InputSchema() map[string]any { return t.schema }
func (t *schemaOutputTool) HelpText() string            { return "" }
func (t *schemaOutputTool) RunSchema(input map[string]any) (string, error) {
	t.inputs = append(t.inputs, input)
	return t.output, nil
}
func (t *schemaOutputTool) Run(*string) (string, error) { return t.output, nil }

type scriptedToolConnector struct {
	responses      []connector.LlmResponseWithTools
	queryCalls     int
	queryToolCalls int
	toolPrompts    []string
}

func (c *scriptedToolConnector) Query(_ context.Context, params *connector.QueryParams) (string, error) {
	c.queryCalls++
	if params != nil && params.UserPrompt != nil {
		c.toolPrompts = append(c.toolPrompts, *params.UserPrompt)
	}
	return "unexpected query", nil
}

func (c *scriptedToolConnector) QueryWithTool(_ context.Context, params *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	c.queryToolCalls++
	if params != nil && params.UserPrompt != nil {
		c.toolPrompts = append(c.toolPrompts, *params.UserPrompt)
	}
	if len(c.responses) == 0 {
		return connector.LlmResponseWithTools{}, nil
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

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
			tools.ToolNameFileSearch: "internal/app/task.go",
			"analysis":               "task prompt resolution is coupled to ask prompt resolution",
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

func TestSelectRawTaskOutput(t *testing.T) {
	t.Run("single display tool with multiline output", func(t *testing.T) {
		selected := selectRawTaskOutput([]taskToolOutput{{
			ToolName: tools.ToolNameUnix,
			Output:   "drwxr-xr-x\t2 user staff\n-rw-r--r--\t1 user staff",
		}})

		assert.Equal(t, tools.ToolNameUnix, selected.ToolName)
		assert.Contains(t, selected.Output, "\t2 user staff\n")
	})

	t.Run("single display tool with single-line output", func(t *testing.T) {
		selected := selectRawTaskOutput([]taskToolOutput{{
			ToolName: tools.ToolNameUnix,
			Output:   "/tmp/project",
		}})

		assert.Empty(t, selected.ToolName)
		assert.Empty(t, selected.Output)
	})

	t.Run("multiple tool outputs", func(t *testing.T) {
		selected := selectRawTaskOutput([]taskToolOutput{
			{ToolName: tools.ToolNameUnix, Output: "one\ntwo"},
			{ToolName: tools.ToolNameFileSearch, Output: "a.go\nb.go"},
		})

		assert.Equal(t, tools.ToolNameFileSearch, selected.ToolName)
		assert.Equal(t, "a.go\nb.go", selected.Output)
	})

	t.Run("repeated same tool outputs", func(t *testing.T) {
		selected := selectRawTaskOutput([]taskToolOutput{
			{ToolName: tools.ToolNameUnix, Output: "old\nlisting"},
			{ToolName: tools.ToolNameUnix, Output: "new\tlisting\nwith details"},
		})

		assert.Equal(t, tools.ToolNameUnix, selected.ToolName)
		assert.Equal(t, "new\tlisting\nwith details", selected.Output)
	})

	t.Run("non display tool", func(t *testing.T) {
		selected := selectRawTaskOutput([]taskToolOutput{{
			ToolName: tools.ToolNameFileEdit,
			Output:   "wrote 32 bytes to file.txt\n",
		}})

		assert.Empty(t, selected.ToolName)
	})
}

func TestTaskWithOptionsResultReturnsDirectRawOutputForFinalTool(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{{
			ToolUse:   true,
			ToolName:  tools.ToolNameFileSearch,
			ToolInput: map[string]any{"contains": "task", "final": true},
		}},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			tools.ToolNameFileSearch: &fixedOutputTool{name: tools.ToolNameFileSearch, output: "a.go\nb.go"},
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "list matching files", TaskOptions{})

	require.NoError(t, err)
	assert.True(t, result.DirectRawOutput)
	assert.Equal(t, "a.go\nb.go", result.RawOutput)
	assert.Equal(t, tools.ToolNameFileSearch, result.RawOutputTool)
	assert.Equal(t, "a.go\nb.go", result.DisplayText())
	assert.Equal(t, 1, conn.queryToolCalls)
	assert.Equal(t, 0, conn.queryCalls)
}

func TestTaskWithOptionsResultHandlesUnknownToolWithoutPanicking(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: "missing_tool", ToolInput: map[string]any{"command": "noop"}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			ToolNameFinalAnswer: NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "list matching files", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, conn.toolPrompts, 2)
	assert.Contains(t, conn.toolPrompts[1], "Failed to execute missing_tool: unknown tool: missing_tool")
	assert.Contains(t, conn.toolPrompts[1], "Provided tool arguments: map[command:noop]")
}

func TestTaskWithOptionsResultRejectsMalformedToolInputBeforeExecution(t *testing.T) {
	utils.GetLogger()

	validatedTool := &schemaOutputTool{
		name:   "schema_tool",
		output: "tool-output",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	}
	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: validatedTool.Name(), ToolInput: map[string]any{"command": true}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			validatedTool.Name(): validatedTool,
			ToolNameFinalAnswer:  NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "run validated tool", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	assert.Empty(t, validatedTool.inputs)
	require.Len(t, conn.toolPrompts, 2)
	assert.Contains(t, conn.toolPrompts[1], "Failed to execute schema_tool: invalid tool input: field \"command\" must be a string")
}

func TestTaskWithOptionsResultExecutesValidToolInput(t *testing.T) {
	utils.GetLogger()

	validatedTool := &schemaOutputTool{
		name:   "schema_tool",
		output: "tool-output",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
			"required": []string{"command"},
		},
	}
	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: validatedTool.Name(), ToolInput: map[string]any{"command": "pwd"}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			validatedTool.Name(): validatedTool,
			ToolNameFinalAnswer:  NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "run validated tool", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, validatedTool.inputs, 1)
	require.Len(t, conn.toolPrompts, 2)
	assert.Contains(t, conn.toolPrompts[1], "tool-output")
	assert.NotContains(t, conn.toolPrompts[1], "Failed to execute schema_tool")
}
