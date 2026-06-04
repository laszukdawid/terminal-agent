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

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fixedOutputTool struct {
	name   string
	output string
}

func (t *fixedOutputTool) Name() string                                 { return t.name }
func (t *fixedOutputTool) PermissionCategory() tools.PermissionCategory { return tools.PermissionRead }
func (t *fixedOutputTool) Description() string                          { return "" }
func (t *fixedOutputTool) InputSchema() map[string]any                  { return map[string]any{} }
func (t *fixedOutputTool) HelpText() string                             { return "" }
func (t *fixedOutputTool) RunSchema(map[string]any) (string, error)     { return t.output, nil }
func (t *fixedOutputTool) Run(*string) (string, error)                  { return t.output, nil }

type schemaOutputTool struct {
	name     string
	output   string
	schema   map[string]any
	inputs   []map[string]any
	category tools.PermissionCategory
}

func (t *schemaOutputTool) Name() string { return t.name }
func (t *schemaOutputTool) PermissionCategory() tools.PermissionCategory {
	if t.category != "" {
		return t.category
	}
	return tools.PermissionRead
}
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

func (t *sequentialOutputTool) Name() string { return t.name }
func (t *sequentialOutputTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}
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

type progressOutputTool struct {
	name string
}

func (t *progressOutputTool) Name() string { return t.name }
func (t *progressOutputTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}
func (t *progressOutputTool) Description() string                      { return "" }
func (t *progressOutputTool) InputSchema() map[string]any              { return map[string]any{} }
func (t *progressOutputTool) HelpText() string                         { return "" }
func (t *progressOutputTool) RunSchema(map[string]any) (string, error) { return "done", nil }
func (t *progressOutputTool) Run(*string) (string, error)              { return "done", nil }
func (t *progressOutputTool) RunSchemaWithContext(input map[string]any, ctx tools.ToolExecutionContext) (string, error) {
	if ctx.Progress != nil {
		ctx.Progress("halfway")
	}
	return "tool done", nil
}

type failingOutputTool struct {
	name string
	err  error
}

func (t *failingOutputTool) Name() string { return t.name }
func (t *failingOutputTool) PermissionCategory() tools.PermissionCategory {
	return tools.PermissionRead
}
func (t *failingOutputTool) Description() string                      { return "" }
func (t *failingOutputTool) InputSchema() map[string]any              { return map[string]any{} }
func (t *failingOutputTool) HelpText() string                         { return "" }
func (t *failingOutputTool) RunSchema(map[string]any) (string, error) { return "", t.err }
func (t *failingOutputTool) Run(*string) (string, error)              { return "", t.err }

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

type cancelingTaskConnector struct {
	cancel         context.CancelFunc
	response       connector.LlmResponseWithTools
	queryCalls     int
	queryToolCalls int
}

func (c *cancelingTaskConnector) Query(_ context.Context, _ *connector.QueryParams) (string, error) {
	c.queryCalls++
	return "summary should not run", nil
}

func (c *cancelingTaskConnector) QueryWithTool(_ context.Context, _ *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	c.queryToolCalls++
	if c.cancel != nil {
		c.cancel()
	}
	return c.response, nil
}

func (c *cancelingTaskConnector) SupportsNativeToolCalling() bool {
	return true
}

func TestNewAgentFiltersUnavailableTools(t *testing.T) {
	connector := &scriptedToolConnector{}
	cfg := config.NewDefaultConfig()

	t.Run("websearch excluded when unavailable", func(t *testing.T) {
		t.Setenv("TAVILY_KEY", "")

		agentInstance := NewAgent(connector, tools.NewToolProvider(cfg), cfg, "ask", "task")

		assert.NotContains(t, agentInstance.Tools, tools.ToolNameWebsearch)
	})

	t.Run("websearch included when available", func(t *testing.T) {
		t.Setenv("TAVILY_KEY", "test_key")

		agentInstance := NewAgent(connector, tools.NewToolProvider(cfg), cfg, "ask", "task")

		assert.Contains(t, agentInstance.Tools, tools.ToolNameWebsearch)
	})
}

type fakeTaskInteraction struct {
	confirmations  []TaskConfirmationRequest
	clarifications []TaskClarificationRequest
	decision       TaskConfirmationDecision
	answer         string
}

func (i *fakeTaskInteraction) Confirm(req TaskConfirmationRequest) (TaskConfirmationDecision, error) {
	i.confirmations = append(i.confirmations, req)
	return i.decision, nil
}

func (i *fakeTaskInteraction) Clarify(req TaskClarificationRequest) (string, error) {
	i.clarifications = append(i.clarifications, req)
	return i.answer, nil
}

// uncategorizedStubTool stands in for an MCP/third-party tool that does not
// declare a permission category, so it must default to gated.
type uncategorizedStubTool struct{}

func (uncategorizedStubTool) RunSchema(map[string]any) (string, error) { return "", nil }
func (uncategorizedStubTool) Run(*string) (string, error)              { return "", nil }
func (uncategorizedStubTool) Name() string                             { return "mcp_thing" }
func (uncategorizedStubTool) Description() string                      { return "external tool" }
func (uncategorizedStubTool) InputSchema() map[string]any              { return map[string]any{} }
func (uncategorizedStubTool) HelpText() string                         { return "" }

func TestConfirmToolByCategory(t *testing.T) {
	cases := []struct {
		name       string
		tool       tools.Tool
		input      map[string]any
		wantPrompt bool
	}{
		{"read never prompts", tools.NewReadTool("/repo"), map[string]any{"path": "x"}, false},
		{"write inside root", tools.NewFileEditTool("/repo"), map[string]any{"path": "notes.txt", "operation": "write"}, false},
		{"write outside root", tools.NewFileEditTool("/repo"), map[string]any{"path": "../escape.txt", "operation": "write"}, true},
		{"execute always prompts", tools.NewUnixTool(nil), map[string]any{"command": "ls"}, true},
		{"undeclared tool gated", uncategorizedStubTool{}, map[string]any{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interaction := &fakeTaskInteraction{decision: TaskConfirmationDecision{Allowed: true}}
			requester := taskUserConfirmationRequester{interaction: interaction}
			run := &taskExecutionState{
				state:         &TaskState{Dirs: TaskDirs{RootDir: "/repo", CurrentDir: "/repo"}},
				confirmations: NewConfirmationManager(nil, nil, requester.RequestUserConfirmation, nil),
			}
			response := connector.LlmResponseWithTools{ToolName: tc.tool.Name(), ToolInput: tc.input}

			allowed, err := run.confirmTool(tc.tool, response)
			if err != nil {
				t.Fatalf("confirmTool error: %v", err)
			}
			if !allowed {
				t.Fatal("expected allow (interaction approves)")
			}
			if gotPrompt := len(interaction.confirmations) > 0; gotPrompt != tc.wantPrompt {
				t.Fatalf("prompted = %v, want %v", gotPrompt, tc.wantPrompt)
			}
		})
	}
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

func TestTaskUsesInjectedClarificationInteraction(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	interaction := &fakeTaskInteraction{answer: "Use the internal directory."}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{
			ToolUse:  true,
			ToolName: UserClarificationToolName,
			ToolInput: map[string]any{
				"question": "Which directory should I inspect?",
			},
		},
		{
			ToolUse:  true,
			ToolName: ToolNameFinalAnswer,
			ToolInput: map[string]any{
				"answer": "done",
			},
		},
	}}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "inspect the repo", TaskOptions{
		Interaction: interaction,
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, interaction.clarifications, 1)
	assert.Equal(t, "Which directory should I inspect?", interaction.clarifications[0].Question)
}

func TestTaskUsesInjectedConfirmationInteraction(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	interaction := &fakeTaskInteraction{decision: TaskConfirmationDecision{Allowed: true, Remember: true}}
	unixTool := &schemaOutputTool{
		name:     tools.ToolNameUnix,
		category: tools.PermissionExecute,
		output:   "status output",
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
			},
		},
	}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{
			ToolUse:  true,
			ToolName: tools.ToolNameUnix,
			ToolInput: map[string]any{
				"command": "git status",
			},
		},
		{
			ToolUse:  true,
			ToolName: ToolNameFinalAnswer,
			ToolInput: map[string]any{
				"answer": "done",
			},
		},
	}}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{tools.ToolNameUnix: unixTool},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "inspect git state", TaskOptions{
		Interaction: interaction,
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.Len(t, interaction.confirmations, 1)
	assert.Equal(t, `unix("git status")`, interaction.confirmations[0].Action)
}

func TestTaskWithOptionsResultEmitsStatusAndProgress(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	tool := &progressOutputTool{name: "progress_tool"}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{
			ToolUse:   true,
			ToolName:  "progress_tool",
			ToolInput: map[string]any{},
		},
		{
			ToolUse:  true,
			ToolName: ToolNameFinalAnswer,
			ToolInput: map[string]any{
				"answer": "done",
			},
		},
	}}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{"progress_tool": tool},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	var statuses []TaskStatusEvent
	var progress []TaskProgressEvent
	result, err := agent.TaskWithOptionsResult(context.Background(), "run progress tool", TaskOptions{
		Dirs:       TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
		OnStatus:   func(event TaskStatusEvent) { statuses = append(statuses, event) },
		OnProgress: func(event TaskProgressEvent) { progress = append(progress, event) },
	})

	require.NoError(t, err)
	assert.Equal(t, "done", result.Response)
	require.NotEmpty(t, statuses)
	assert.Equal(t, TaskStatusThinking, statuses[0].Phase)
	assert.Contains(t, statusPhases(statuses), TaskStatusRunningTool)
	assert.Contains(t, statusPhases(statuses), TaskStatusCompleted)
	require.Len(t, progress, 1)
	assert.Equal(t, "progress_tool", progress[0].ToolName)
	assert.Equal(t, "halfway", progress[0].Message)
}

func TestTaskWithOptionsResultEmitsFailureStatusWithError(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	tool := &failingOutputTool{name: "read", err: errors.New("path outside working directory: /repo/Makefile")}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{
			ToolUse:   true,
			ToolName:  "read",
			ToolInput: map[string]any{"path": "/repo/Makefile"},
		},
		{
			ToolUse:  true,
			ToolName: ToolNameFinalAnswer,
			ToolInput: map[string]any{
				"answer": "cannot read file",
			},
		},
	}}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{"read": tool},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	var statuses []TaskStatusEvent
	result, err := agent.TaskWithOptionsResult(context.Background(), "read file", TaskOptions{
		Dirs:     TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
		OnStatus: func(event TaskStatusEvent) { statuses = append(statuses, event) },
	})

	require.NoError(t, err)
	assert.Equal(t, "cannot read file", result.Response)
	require.Contains(t, statusMessages(statuses), "read failed: path outside working directory: /repo/Makefile")
}

func statusPhases(statuses []TaskStatusEvent) []TaskStatusPhase {
	phases := make([]TaskStatusPhase, 0, len(statuses))
	for _, status := range statuses {
		phases = append(phases, status.Phase)
	}
	return phases
}

func statusMessages(statuses []TaskStatusEvent) []string {
	messages := make([]string, 0, len(statuses))
	for _, status := range statuses {
		messages = append(messages, status.Message)
	}
	return messages
}

func TestTaskWithOptionsResultStopsWhenContextCanceled(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	conn := &cancelingTaskConnector{
		cancel:   cancel,
		response: connector.LlmResponseWithTools{Response: "still thinking"},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	_, err := agent.TaskWithOptionsResult(ctx, "keep going", TaskOptions{
		Dirs: TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, conn.queryToolCalls)
	assert.Equal(t, 0, conn.queryCalls)
}

func TestTaskWithOptionsResultDoesNotExecuteToolAfterContextCanceled(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	tool := &schemaOutputTool{
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
	conn := &cancelingTaskConnector{
		cancel: cancel,
		response: connector.LlmResponseWithTools{
			ToolUse:   true,
			ToolName:  tool.Name(),
			ToolInput: map[string]any{"command": "pwd"},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			tool.Name(): tool,
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	_, err := agent.TaskWithOptionsResult(ctx, "run the tool", TaskOptions{
		Dirs: TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, conn.queryToolCalls)
	assert.Empty(t, tool.inputs)
}

func TestTaskWithOptionsResultCancelsActiveUnixCommand(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	interaction := &fakeTaskInteraction{decision: TaskConfirmationDecision{Allowed: true}}
	conn := &scriptedToolConnector{responses: []connector.LlmResponseWithTools{
		{
			ToolUse:  true,
			ToolName: tools.ToolNameUnix,
			ToolInput: map[string]any{
				"command": "sleep 5",
			},
		},
	}}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			tools.ToolNameUnix: tools.NewUnixTool(nil),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}
	steps := []TaskStep{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := agent.TaskWithOptionsResult(ctx, "run a long command", TaskOptions{
		Interaction: interaction,
		Dirs:        TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
		OnStep: func(step TaskStep) {
			steps = append(steps, step)
		},
	})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second)
	assert.Empty(t, steps)
	assert.Equal(t, 1, conn.queryToolCalls)
	require.Len(t, interaction.confirmations, 1)
	assert.Equal(t, `unix("sleep 5")`, interaction.confirmations[0].Action)
}

// blockingTaskConnector blocks each query until the context is done, then
// returns the context error. It is used to exercise task-level timeouts.
type blockingTaskConnector struct {
	queryCalls     int
	queryToolCalls int
}

func (c *blockingTaskConnector) Query(ctx context.Context, _ *connector.QueryParams) (string, error) {
	c.queryCalls++
	<-ctx.Done()
	return "", ctx.Err()
}

func (c *blockingTaskConnector) QueryWithTool(ctx context.Context, _ *connector.QueryParams, _ map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	c.queryToolCalls++
	<-ctx.Done()
	return connector.LlmResponseWithTools{}, ctx.Err()
}

func (c *blockingTaskConnector) SupportsNativeToolCalling() bool {
	return true
}

func TestTaskWithOptionsResultReturnsErrTaskTimeoutWhenTimeoutElapses(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	conn := &blockingTaskConnector{}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	start := time.Now()
	_, err := agent.TaskWithOptionsResult(context.Background(), "keep going forever", TaskOptions{
		Timeout: 100 * time.Millisecond,
		Dirs:    TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, ErrTaskTimeout)
	assert.NotErrorIs(t, err, context.Canceled)
	assert.Less(t, elapsed, 2*time.Second)
}

func TestTaskWithOptionsResultCallerCancellationTakesPrecedenceOverTimeout(t *testing.T) {
	utils.Logger = zap.NewNop()
	t.Setenv("HOME", t.TempDir())
	rootDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	conn := &cancelingTaskConnector{
		cancel:   cancel,
		response: connector.LlmResponseWithTools{Response: "still thinking"},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector:        conn,
		Tools:            map[string]tools.Tool{},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	_, err := agent.TaskWithOptionsResult(ctx, "keep going", TaskOptions{
		Timeout: 10 * time.Second,
		Dirs:    TaskDirs{RootDir: rootDir, CurrentDir: rootDir},
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.NotErrorIs(t, err, ErrTaskTimeout)
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

	finalSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"final": map[string]string{
				"type":        "boolean",
				"description": "Set to true only when the query results themselves fully answer the user's request and should be returned directly without another model summary round.",
			},
		},
	}

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{{
			ToolUse:   true,
			ToolName:  "query_db",
			ToolInput: map[string]any{"sql": "SELECT 1", "final": true},
		}},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			"query_db": &schemaOutputTool{name: "query_db", output: "column\n------\n1", schema: finalSchema},
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "query the database", TaskOptions{})

	require.NoError(t, err)
	assert.True(t, result.DirectRawOutput)
	assert.Equal(t, "column\n------\n1", result.RawOutput)
	assert.Equal(t, "query_db", result.RawOutputTool)
	assert.Equal(t, "column\n------\n1", result.DisplayText())
	assert.Equal(t, 1, conn.queryToolCalls)
	assert.Equal(t, 0, conn.queryCalls)
}

func TestTaskActionPromptFinalGuidanceIsSelective(t *testing.T) {
	userPrompt := "task context"
	prompt := (&Agent{}).buildTaskActionPrompt(
		&connector.QueryParams{UserPrompt: &userPrompt},
		map[string]tools.Tool{},
		"",
		nil,
	)

	assert.Contains(t, prompt, `Use "final": true ONLY when raw output is definitely the final user-facing answer`)
	assert.Contains(t, prompt, `If raw output needs interpretation, filtering, grouping, cleanup, explanation, or validation, do not set "final": true`)
	assert.NotContains(t, prompt, "fully answers the request")
}

func TestToolSupportsFinal(t *testing.T) {
	t.Run("built-in tool opt-in", func(t *testing.T) {
		assert.True(t, toolSupportsFinal(tools.NewReadTool("")))
	})

	t.Run("custom schema with matching final semantics", func(t *testing.T) {
		tool := &schemaOutputTool{
			name:   "query_db",
			output: "ok",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"final": map[string]string{
						"type":        "boolean",
						"description": "Set to true only when the query results themselves fully answer the user's request and should be returned directly without another model summary round.",
					},
				},
			},
		}

		assert.True(t, toolSupportsFinal(tool))
	})

	t.Run("tool without final field", func(t *testing.T) {
		assert.False(t, toolSupportsFinal(&fixedOutputTool{name: "api_call", output: "ok"}))
	})

	t.Run("tool with conflicting final semantics", func(t *testing.T) {
		tool := &schemaOutputTool{
			name:   "doc_finalize",
			output: "done",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"final": map[string]string{
						"type":        "boolean",
						"description": "Set to true to finalize the document and prevent further edits.",
					},
				},
			},
		}

		assert.False(t, toolSupportsFinal(tool))
	})
}

func TestTaskWithOptionsResultIgnoresFinalForToolWithoutSchemaField(t *testing.T) {
	utils.GetLogger()

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: "api_call", ToolInput: map[string]any{"endpoint": "/data", "final": true}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			"api_call":          &fixedOutputTool{name: "api_call", output: "api result"},
			ToolNameFinalAnswer: NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "call api then finish", TaskOptions{})

	require.NoError(t, err)
	assert.False(t, result.DirectRawOutput)
	assert.Equal(t, "done", result.Response)
	assert.Equal(t, 2, conn.queryToolCalls)
}

func TestTaskWithOptionsResultIgnoresFinalWithConflictingSemantics(t *testing.T) {
	utils.GetLogger()

	conflictingSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"document_id": map[string]string{"type": "string"},
			"final": map[string]string{
				"type":        "boolean",
				"description": "Set to true to finalize the document and prevent further edits.",
			},
		},
	}

	conn := &scriptedToolConnector{
		responses: []connector.LlmResponseWithTools{
			{ToolUse: true, ToolName: "doc_finalize", ToolInput: map[string]any{"document_id": "abc", "final": true}},
			{ToolUse: true, ToolName: ToolNameFinalAnswer, ToolInput: map[string]any{"answer": "done"}},
		},
	}
	sysPrompt := "task system prompt"
	agent := &Agent{
		Connector: conn,
		Tools: map[string]tools.Tool{
			"doc_finalize":      &schemaOutputTool{name: "doc_finalize", output: "document finalized", schema: conflictingSchema},
			ToolNameFinalAnswer: NewFinalAnswerTool(),
		},
		systemPromptTask: &sysPrompt,
		maxTokens:        MaxTokens,
	}

	result, err := agent.TaskWithOptionsResult(context.Background(), "finalize document", TaskOptions{})

	require.NoError(t, err)
	assert.False(t, result.DirectRawOutput)
	assert.Equal(t, "done", result.Response)
	assert.Equal(t, 2, conn.queryToolCalls)
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
