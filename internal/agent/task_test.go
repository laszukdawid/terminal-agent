package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type sequentialOutputTool struct {
	name    string
	outputs []string
	schema  map[string]any
	inputs  []map[string]any
}

func (t *sequentialOutputTool) Name() string                { return t.name }
func (t *sequentialOutputTool) Description() string         { return "" }
func (t *sequentialOutputTool) InputSchema() map[string]any { return t.schema }
func (t *sequentialOutputTool) HelpText() string            { return "" }
func (t *sequentialOutputTool) RunSchema(input map[string]any) (string, error) {
	t.inputs = append(t.inputs, input)
	index := len(t.inputs) - 1
	if index >= len(t.outputs) {
		index = len(t.outputs) - 1
	}
	return t.outputs[index], nil
}
func (t *sequentialOutputTool) Run(*string) (string, error) {
	return t.RunSchema(nil)
}

type scriptedToolConnector struct {
	responses      []connector.LlmResponseWithTools
	queryCalls     int
	queryToolCalls int
	toolPrompts    []string
	devices        []string
	queryResponse  string
	queryErr       error
}

func (c *scriptedToolConnector) Query(_ context.Context, params *connector.QueryParams) (string, error) {
	c.queryCalls++
	if params != nil && params.UserPrompt != nil {
		c.toolPrompts = append(c.toolPrompts, *params.UserPrompt)
	}
	if params != nil {
		c.devices = append(c.devices, params.Device)
	}
	if c.queryErr != nil {
		return "", c.queryErr
	}
	if c.queryResponse != "" {
		return c.queryResponse, nil
	}
	return "unexpected query", nil
}

func (c *scriptedToolConnector) QueryWithTool(_ context.Context, params *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	c.queryToolCalls++
	if params != nil && params.UserPrompt != nil {
		c.toolPrompts = append(c.toolPrompts, *params.UserPrompt)
	}
	if params != nil {
		c.devices = append(c.devices, params.Device)
	}
	if len(c.responses) == 0 {
		return connector.LlmResponseWithTools{}, nil
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func (c *scriptedToolConnector) SupportsNativeToolCalling() bool {
	return true
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

type fallbackTaskConnector struct {
	queryCalls     int
	toolCalls      int
	queryResponse  string
	queryResponses []string
	queryErr       error
	devices        []string
	prompts        []string
}

func (c *fallbackTaskConnector) Query(_ context.Context, params *connector.QueryParams) (string, error) {
	c.queryCalls++
	if params != nil && params.UserPrompt != nil {
		c.prompts = append(c.prompts, *params.UserPrompt)
	}
	if params != nil {
		c.devices = append(c.devices, params.Device)
	}
	if c.queryErr != nil {
		return "", c.queryErr
	}
	if len(c.queryResponses) > 0 {
		response := c.queryResponses[0]
		c.queryResponses = c.queryResponses[1:]
		return response, nil
	}
	return c.queryResponse, nil
}

func (c *fallbackTaskConnector) QueryWithTool(_ context.Context, _ *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	c.toolCalls++
	return connector.LlmResponseWithTools{}, errors.New("native tool calling should not be used")
}

func (c *fallbackTaskConnector) SupportsNativeToolCalling() bool {
	return false
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
		Dirs:          TaskDirs{RootDir: "/repo", CurrentDir: "/repo/internal"},
		Steps: []TaskStep{
			{
				Iteration: 1,
				Timestamp: time.Date(2026, time.May, 24, 10, 0, 0, 0, time.UTC),
				Status:    TaskStepStatusThought,
				Thought:   "task prompt resolution is coupled to ask prompt resolution",
			},
			{
				Iteration:  2,
				Timestamp:  time.Date(2026, time.May, 24, 10, 1, 0, 0, time.UTC),
				Status:     TaskStepStatusSucceeded,
				ToolName:   tools.ToolNameFileSearch,
				ToolInput:  map[string]any{"name_pattern": "task.go"},
				ToolOutput: "internal/app/task.go",
			},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, conn.userPrompt, result)
	assert.Contains(t, conn.userPrompt, "review the task path")
	assert.Contains(t, conn.userPrompt, "Ordered step history:")
	assert.Contains(t, conn.userPrompt, "Timestamp: 2026-05-24T10:00:00Z")
	assert.Contains(t, conn.userPrompt, "Status: thought")
	assert.Contains(t, conn.userPrompt, "internal/app/task.go")
	assert.Contains(t, conn.userPrompt, "task prompt resolution is coupled to ask prompt resolution")
	assert.Equal(t, MaxTokens*2, conn.maxTokens)
}

func TestBuildTaskPromptUsesOrderedStructuredHistory(t *testing.T) {
	prompt := buildTaskPrompt(&TaskState{
		OriginalQuery: "trace repeated tool calls",
		Iterations:    3,
		ToolCalls:     2,
		MaxIterations: MaxToolCalls,
		MaxTurns:      MaxTurns,
		Phase:         TaskPhaseRunning,
		Dirs:          TaskDirs{RootDir: "/repo", CurrentDir: "/repo/internal"},
		Steps: []TaskStep{
			{
				Iteration:  1,
				Timestamp:  time.Date(2026, time.May, 24, 9, 0, 0, 0, time.UTC),
				Status:     TaskStepStatusSucceeded,
				Thought:    "Start with a broad search.",
				ToolName:   tools.ToolNameFileSearch,
				ToolInput:  map[string]any{"name_pattern": "*.go"},
				ToolOutput: "internal/agent/agent.go\ninternal/app/task.go",
			},
			{
				Iteration: 2,
				Timestamp: time.Date(2026, time.May, 24, 9, 1, 0, 0, time.UTC),
				Status:    TaskStepStatusFailed,
				Thought:   "Inspect git state before editing.",
				ToolName:  tools.ToolNameUnix,
				ToolInput: map[string]any{"command": "git status --short"},
				Error:     "Failed to execute unix: exit status 1",
			},
			{
				Iteration:  3,
				Timestamp:  time.Date(2026, time.May, 24, 9, 2, 0, 0, time.UTC),
				Status:     TaskStepStatusSucceeded,
				Thought:    "Narrow the search to task state usage.",
				ToolName:   tools.ToolNameFileSearch,
				ToolInput:  map[string]any{"contains": "TaskState"},
				ToolOutput: "internal/agent/agent.go:730",
			},
		},
	})

	assert.Contains(t, prompt, "Ordered step history:")
	assert.Contains(t, prompt, "Timestamp: 2026-05-24T09:00:00Z")
	assert.Contains(t, prompt, "Status: failed")
	assert.Contains(t, prompt, `Input: {"name_pattern":"*.go"}`)
	assert.Contains(t, prompt, `Input: {"command":"git status --short"}`)
	assert.Contains(t, prompt, "Failed to execute unix: exit status 1")
	assert.Contains(t, prompt, "Narrow the search to task state usage.")

	firstSearch := strings.Index(prompt, `Input: {"name_pattern":"*.go"}`)
	secondSearch := strings.Index(prompt, `Input: {"contains":"TaskState"}`)
	require.NotEqual(t, -1, firstSearch)
	require.NotEqual(t, -1, secondSearch)
	assert.Less(t, firstSearch, secondSearch)
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

func TestTaskWithOptionsResultPropagatesDevice(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{{
			ToolUse:   true,
			ToolName:  ToolNameFinalAnswer,
			ToolInput: map[string]any{"answer": "done"},
		}},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{ToolNameFinalAnswer: NewFinalAnswerTool()},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
		device:           "cpu",
	}

	_, err := agent.TaskWithOptionsResult(context.Background(), "finish on cpu", TaskOptions{})

	require.NoError(t, err)
	require.NotEmpty(t, conn.devices)
	assert.Equal(t, "cpu", conn.devices[0])
}

func TestTaskWithOptionsResultFallsBackWithoutNativeToolCalling(t *testing.T) {
	utils.GetLogger()

	conn := &fallbackTaskConnector{
		queryResponse: `{"action":"final_answer","answer":"done","thought":"task complete"}`,
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{ToolNameFinalAnswer: NewFinalAnswerTool()},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
		device:           "gpu",
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "finish without native tools", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	assert.Equal(t, 1, conn.queryCalls)
	assert.Empty(t, conn.toolCalls)
	require.NotEmpty(t, conn.prompts)
	assert.Contains(t, conn.prompts[0], `{"action":"tool"`)
	assert.NotContains(t, conn.prompts[0], `task system prompt`)
	assert.Equal(t, "gpu", conn.devices[0])
}

func TestTaskWithOptionsResultRetriesMalformedFallbackResponse(t *testing.T) {
	utils.GetLogger()

	conn := &fallbackTaskConnector{
		queryResponses: []string{
			`{"type":"object","properties":{"answer":{"type":"string"}}}`,
			`{"action":"final_answer","answer":"done","thought":"task complete"}`,
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{ToolNameFinalAnswer: NewFinalAnswerTool()},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "finish after one malformed response", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	assert.Equal(t, 2, conn.queryCalls)
	require.Len(t, conn.prompts, 2)
	assert.Contains(t, conn.prompts[1], "Your previous response was invalid.")
	assert.Contains(t, conn.prompts[1], `{"type":"object"`)
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
	assert.Contains(t, conn.toolPrompts[1], "Status: failed")
	assert.Contains(t, conn.toolPrompts[1], "Tool: missing_tool")
	assert.Contains(t, conn.toolPrompts[1], `Input: {"command":"noop"}`)
	assert.Contains(t, conn.toolPrompts[1], "Failed to execute missing_tool: unknown tool: missing_tool")
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
	assert.Contains(t, conn.toolPrompts[1], "Status: failed")
	assert.Contains(t, conn.toolPrompts[1], "Tool: schema_tool")
	assert.Contains(t, conn.toolPrompts[1], `Input: {"command":true}`)
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

func TestTaskWithOptionsResultPreservesRepeatedToolStepsInPrompt(t *testing.T) {
	utils.GetLogger()

	repeatedTool := &sequentialOutputTool{
		name:    "schema_tool",
		outputs: []string{"first-output", "second-output"},
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
			{ToolUse: true, ToolName: repeatedTool.Name(), ToolInput: map[string]any{"command": "first"}, Response: "Run the first pass."},
			{ToolUse: true, ToolName: repeatedTool.Name(), ToolInput: map[string]any{"command": "second"}, Response: "Run the second pass."},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			repeatedTool.Name(): repeatedTool,
			ToolNameFinalAnswer: NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "run the tool twice", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, conn.toolPrompts, 3)
	assert.Contains(t, conn.toolPrompts[2], `Input: {"command":"first"}`)
	assert.Contains(t, conn.toolPrompts[2], "first-output")
	assert.Contains(t, conn.toolPrompts[2], `Input: {"command":"second"}`)
	assert.Contains(t, conn.toolPrompts[2], "second-output")
	assert.Less(t, strings.Index(conn.toolPrompts[2], "first-output"), strings.Index(conn.toolPrompts[2], "second-output"))
}

func TestTaskWithOptionsResultAcceptsJSONNumberIntegerToolInput(t *testing.T) {
	utils.GetLogger()

	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "match.txt"), []byte("task\nother\n"), 0o644))

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: tools.ToolNameFileSearch, ToolInput: map[string]any{"contains": "task", "max_results": json.Number("1")}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			tools.ToolNameFileSearch: tools.NewFileSearchTool(rootDir),
			ToolNameFinalAnswer:      NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "search with integer limit", TaskOptions{
		Dirs: TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, conn.toolPrompts, 2)
	assert.Contains(t, conn.toolPrompts[1], `Input: {"contains":"task","max_results":1}`)
	assert.Contains(t, conn.toolPrompts[1], "match.txt:1: task")
	assert.NotContains(t, conn.toolPrompts[1], "match.txt:2")
}

func TestTaskWithOptionsResultUsesSummaryFallbackAtMaxIterations(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{queryResponse: "summary output"}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "finish without tools", TaskOptions{})

	require.NoError(t, err)
	assert.Equal(t, "summary output", result.Response)
	assert.Equal(t, MaxTurns, conn.queryToolCalls)
	assert.Equal(t, 1, conn.queryCalls)
}

func TestTaskWithOptionsResultReturnsErrorWhenSummaryFallbackFails(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{queryErr: errors.New("summary failed")}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	_, err := agent.TaskWithOptionsResult(context.Background(), "finish without tools", TaskOptions{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "summary failed")
	assert.Equal(t, MaxTurns, conn.queryToolCalls)
	assert.Equal(t, 1, conn.queryCalls)
}

func TestTaskWithOptionsResultUpdatesCurrentDirectoryExplicitly(t *testing.T) {
	utils.GetLogger()

	rootDir := t.TempDir()
	subDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "match.txt"), []byte("hello"), 0o644))

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: ToolNameChangeDirectory, ToolInput: map[string]any{"path": "nested"}},
			{ToolUse: true, ToolName: tools.ToolNameFileSearch, ToolInput: map[string]any{"name_pattern": "*.txt"}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			ToolNameChangeDirectory:  NewChangeDirectoryTool(),
			tools.ToolNameFileSearch: tools.NewFileSearchTool(rootDir),
			ToolNameFinalAnswer:      NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "search after changing directory", TaskOptions{
		Dirs: TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, conn.toolPrompts, 3)
	assert.Contains(t, conn.toolPrompts[1], "Current working directory: "+subDir)
	assert.Contains(t, conn.toolPrompts[2], "match.txt")
}
