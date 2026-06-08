package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	log "github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	MaxToolCalls              = 50
	MaxTurns                  = 50
	maxTaskActionFallbackRuns = 2
	UserClarificationToolName = "user_clarification"
	ToolNameChangeDirectory   = "change_directory"
	ToolNameFinalAnswer       = "final_answer"
)

type TaskOptions struct {
	Allow        []string
	AutoApprove  bool
	Dirs         TaskDirs
	Interaction  TaskInteraction
	OnStep       func(TaskStep)
	OnStatus     func(TaskStatusEvent)
	OnProgress   func(TaskProgressEvent)
	OnToolOutput func(TaskToolOutputEvent) error
	// Timeout bounds the whole task run. A value of 0 means no task-level
	// timeout (unlimited). Caller context cancellation always takes precedence.
	Timeout time.Duration
}

type TaskToolOutputEvent struct {
	ToolName  string
	ProcessID int
	Output    string
	Err       error
}

type TaskStatusPhase string

const (
	TaskStatusThinking             TaskStatusPhase = "thinking"
	TaskStatusAwaitingConfirmation TaskStatusPhase = "awaiting_confirmation"
	TaskStatusRunningTool          TaskStatusPhase = "running_tool"
	TaskStatusFinalizing           TaskStatusPhase = "finalizing"
	TaskStatusCompleted            TaskStatusPhase = "completed"
	TaskStatusFailed               TaskStatusPhase = "failed"
)

type TaskStatusEvent struct {
	Phase     TaskStatusPhase
	Message   string
	ToolName  string
	ToolInput map[string]any
	Timestamp time.Time
}

type TaskProgressEvent struct {
	ToolName  string
	Message   string
	Timestamp time.Time
}

type TaskRunResult struct {
	Response        string
	RawOutput       string
	RawOutputTool   string
	DirectRawOutput bool
}

type TaskPhase string

const (
	TaskPhaseRunning    TaskPhase = "running"
	TaskPhaseFinalizing TaskPhase = "finalizing"
	TaskPhaseCompleted  TaskPhase = "completed"
	TaskPhaseFailed     TaskPhase = "failed"
)

type TaskDirs struct {
	RootDir    string
	CurrentDir string
}

type taskToolOutput struct {
	ToolName string
	Output   string
}

// TaskState tracks the state of the agent's work on a task.
type TaskState struct {
	OriginalQuery string
	Iterations    int
	ToolCalls     int
	MaxIterations int
	MaxTurns      int
	Phase         TaskPhase
	Dirs          TaskDirs
	Steps         []TaskStep
}

type taskExecutionState struct {
	state             *TaskState
	tools             map[string]tools.Tool
	confirmations     *ConfirmationManager
	successfulOutputs []taskToolOutput
	onStep            func(TaskStep)
	onStatus          func(TaskStatusEvent)
	onProgress        func(TaskProgressEvent)
	onToolOutput      func(TaskToolOutputEvent) error
	autoApprove       bool
}

func (r *taskExecutionState) appendStep(step TaskStep) {
	r.state.appendStep(step)
	if r.onStep != nil {
		r.onStep(r.state.Steps[len(r.state.Steps)-1])
	}
}

type taskUserConfirmationRequester struct {
	interaction TaskInteraction
}

func (r taskUserConfirmationRequester) RequestUserConfirmation(action string) (confirmationDecision, error) {
	if r.interaction == nil {
		return confirmationDecision{}, ErrTaskInteractionRequired
	}

	decision, err := r.interaction.Confirm(TaskConfirmationRequest{Action: action})
	if err != nil {
		return confirmationDecision{}, err
	}

	return confirmationDecision{allowed: decision.Allowed, remember: decision.Remember, patterns: decision.Patterns}, nil
}

type taskPermissionRememberer struct {
	store config.PermissionStore
}

func (r taskPermissionRememberer) Remember(actions []string, allow bool) error {
	return config.RememberPermissions(r.store, actions, allow)
}

func (a *Agent) Task(ctx context.Context, s string) (string, error) {
	return a.TaskWithOptions(ctx, s, TaskOptions{})
}

func (a *Agent) TaskWithOptions(ctx context.Context, s string, options TaskOptions) (string, error) {
	result, err := a.TaskWithOptionsResult(ctx, s, options)
	if err != nil {
		return "", err
	}

	return result.DisplayText(), nil
}

func (r TaskRunResult) DisplayText() string {
	if r.DirectRawOutput && r.RawOutput != "" {
		return r.RawOutput
	}
	return r.Response
}

func (a *Agent) TaskWithOptionsResult(ctx context.Context, s string, options TaskOptions) (TaskRunResult, error) {
	// A positive timeout bounds the whole run; 0 means unlimited so the caller's
	// context passes through untouched. WithTimeoutCause lets us distinguish a
	// timeout-driven stop (cause == ErrTaskTimeout) from caller cancellation.
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeoutCause(ctx, options.Timeout, ErrTaskTimeout)
		defer cancel()
	}

	result, err := a.runTaskLoop(ctx, s, options)
	if err != nil && context.Cause(ctx) == ErrTaskTimeout {
		return TaskRunResult{}, ErrTaskTimeout
	}
	return result, err
}

func (a *Agent) runTaskLoop(ctx context.Context, s string, options TaskOptions) (TaskRunResult, error) {
	logger := log.Sugar()
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}

	run, err := a.newTaskExecutionState(s, options)
	if err != nil {
		return TaskRunResult{}, err
	}

	for run.state.Phase == TaskPhaseRunning && run.state.Iterations < run.state.MaxTurns && run.state.ToolCalls < run.state.MaxIterations {
		if err := ctx.Err(); err != nil {
			return TaskRunResult{}, err
		}
		run.state.Iterations++

		result, done, err := a.runTaskIteration(ctx, logger, run)
		if err != nil {
			return TaskRunResult{}, err
		}
		if done {
			return result, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}
	return a.finalizeTaskRun(ctx, run)
}

func (a *Agent) newTaskExecutionState(query string, options TaskOptions) (*taskExecutionState, error) {
	taskDirs, err := resolveInitialTaskDirs(options.Dirs, a.config)
	if err != nil {
		return nil, err
	}

	ruleSets, store, err := config.LoadPermissionRuleSets(taskDirs.RootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load permissions: %w", err)
	}

	interaction := options.Interaction
	confirmationRequester := taskUserConfirmationRequester{interaction: interaction}
	rememberer := taskPermissionRememberer{store: store}
	confirmations := NewConfirmationManager(options.Allow, ruleSets, confirmationRequester.RequestUserConfirmation, rememberer.Remember)

	return &taskExecutionState{
		state: &TaskState{
			OriginalQuery: query,
			MaxIterations: MaxToolCalls,
			MaxTurns:      MaxTurns,
			Phase:         TaskPhaseRunning,
			Dirs:          taskDirs,
			Steps:         make([]TaskStep, 0, MaxTurns),
		},
		tools:             a.buildTaskTools(interaction),
		confirmations:     confirmations,
		successfulOutputs: make([]taskToolOutput, 0, 1),
		onStep:            options.OnStep,
		onStatus:          options.OnStatus,
		onProgress:        options.OnProgress,
		onToolOutput:      options.OnToolOutput,
		autoApprove:       options.AutoApprove,
	}, nil
}

func (a *Agent) runTaskIteration(ctx context.Context, logger *zap.SugaredLogger, run *taskExecutionState) (TaskRunResult, bool, error) {
	run.emitStatus(TaskStatusThinking, "Thinking", "", nil)
	response, err := a.queryTaskResponse(ctx, run)
	if err != nil {
		logger.Debugw("Error querying model", "iteration", run.state.Iterations, "error", err)
		return TaskRunResult{}, false, fmt.Errorf("error during task processing: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	if !response.ToolUse {
		if response.Response != "" {
			run.state.Phase = TaskPhaseCompleted
			run.recordSuccess(connector.LlmResponseWithTools{
				Response: response.Response,
				ToolUse:  true,
				ToolName: ToolNameFinalAnswer,
				ToolInput: map[string]any{
					"answer": response.Response,
				},
			}, response.Response)
			run.emitStatus(TaskStatusCompleted, "Task completed.", ToolNameFinalAnswer, map[string]any{"answer": response.Response})
			return run.finalAnswerResult(response.Response), true, nil
		}
		run.recordThought(response.Response)
		logger.Debugw("Task iteration complete", "iteration", run.state.Iterations, "phase", run.state.Phase)
		return TaskRunResult{}, false, nil
	}

	return a.handleTaskToolResponse(ctx, logger, run, response)
}

func (a *Agent) queryTaskResponse(ctx context.Context, run *taskExecutionState) (connector.LlmResponseWithTools, error) {
	promptWithState := buildTaskPrompt(run.state)
	qParams := connector.QueryParams{
		UserPrompt: &promptWithState,
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens,
		Device:     a.device,
	}

	response, err := a.nextTaskResponse(ctx, &qParams, run.tools)
	if err != nil {
		return connector.LlmResponseWithTools{}, err
	}
	response.Response = strings.TrimSpace(response.Response)
	return response, nil
}

func (a *Agent) handleTaskToolResponse(ctx context.Context, logger *zap.SugaredLogger, run *taskExecutionState, response connector.LlmResponseWithTools) (TaskRunResult, bool, error) {
	tool, err := resolveTaskToolCall(response.ToolName, response.ToolInput, run.tools)
	if err != nil {
		logger.Errorw("Tool validation failed", "tool", response.ToolName, "error", err)
		run.recordFailure(response, err)
		return TaskRunResult{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	if response.ToolName == ToolNameChangeDirectory {
		run.handleDirectoryChange(response, logger)
		run.state.ToolCalls++
		return TaskRunResult{}, false, nil
	}

	allowed, err := run.confirmTool(tool, response)
	if err != nil {
		logger.Errorw("Tool confirmation failed", "tool", response.ToolName, "error", err)
		return TaskRunResult{}, false, fmt.Errorf("tool confirmation failed: %w", err)
	}
	if !allowed {
		run.recordDeclined(response)
		return TaskRunResult{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	return a.executeTaskTool(ctx, logger, run, tool, response)
}

func (a *Agent) executeTaskTool(ctx context.Context, logger *zap.SugaredLogger, run *taskExecutionState, tool tools.Tool, response connector.LlmResponseWithTools) (TaskRunResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}
	run.emitStatus(TaskStatusRunningTool, formatRunningToolStatus(response.ToolName, response.ToolInput), response.ToolName, response.ToolInput)
	toolResult, err := runTaskTool(ctx, tool, response.ToolInput, run.state.Dirs, newTaskToolOutputWriter(ctx, response.ToolName, run.onToolOutput), run.progressReporter(response.ToolName))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return TaskRunResult{}, false, ctxErr
		}
		run.emitStatus(TaskStatusFailed, fmt.Sprintf("%s failed: %v", response.ToolName, err), response.ToolName, response.ToolInput)
		logger.Debugw("Tool execution failed", "tool", response.ToolName, "error", err)
		run.recordFailure(response, err)
		return TaskRunResult{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	run.state.ToolCalls++
	run.recordSuccess(response, toolResult)
	if response.ToolName == ToolNameFinalAnswer {
		run.state.Phase = TaskPhaseCompleted
		run.emitStatus(TaskStatusCompleted, "Task completed.", response.ToolName, response.ToolInput)
		return run.finalAnswerResult(toolResult), true, nil
	}
	if toolInputRequestsFinal(response.ToolInput) && toolSupportsFinal(tool) {
		run.state.Phase = TaskPhaseCompleted
		run.emitStatus(TaskStatusCompleted, "Task completed.", response.ToolName, response.ToolInput)
		return TaskRunResult{
			Response:        toolResult,
			RawOutput:       toolResult,
			RawOutputTool:   response.ToolName,
			DirectRawOutput: true,
		}, true, nil
	}

	run.successfulOutputs = append(run.successfulOutputs, taskToolOutput{ToolName: response.ToolName, Output: toolResult})
	return TaskRunResult{}, false, nil
}

func (a *Agent) finalizeTaskRun(ctx context.Context, run *taskExecutionState) (TaskRunResult, error) {
	if run.state.Phase != TaskPhaseRunning {
		return TaskRunResult{}, fmt.Errorf("task ended without an explicit completion path")
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}

	run.state.Phase = TaskPhaseFinalizing
	run.emitStatus(TaskStatusFinalizing, "Finalizing...", "", nil)
	response, err := a.finalizeSummary(ctx, run.state)
	if err != nil {
		run.state.Phase = TaskPhaseFailed
		return TaskRunResult{}, err
	}

	run.state.Phase = TaskPhaseCompleted
	run.emitStatus(TaskStatusCompleted, "Task completed.", "", nil)
	rawOutput := selectRawTaskOutput(run.successfulOutputs)
	return TaskRunResult{Response: response, RawOutput: rawOutput.Output, RawOutputTool: rawOutput.ToolName}, nil
}

func (r *taskExecutionState) confirmTool(tool tools.Tool, response connector.LlmResponseWithTools) (bool, error) {
	autoAllow := r.autoAllowsTool(tool, response.ToolInput)
	if !autoAllow && !r.autoApprove {
		r.emitStatus(TaskStatusAwaitingConfirmation, fmt.Sprintf("Awaiting confirmation for %s...", response.ToolName), response.ToolName, response.ToolInput)
	}
	return r.confirmations.ConfirmWithPolicy(BuildActionString(response.ToolName, response.ToolInput), autoAllow, r.autoApprove)
}

func (r *taskExecutionState) emitStatus(phase TaskStatusPhase, message string, toolName string, toolInput map[string]any) {
	if r.onStatus == nil {
		return
	}
	r.onStatus(TaskStatusEvent{Phase: phase, Message: message, ToolName: toolName, ToolInput: toolInput, Timestamp: time.Now().UTC()})
}

func formatRunningToolStatus(toolName string, toolInput map[string]any) string {
	switch toolName {
	case tools.ToolNameRead:
		if status := formatReadStatus(toolInput); status != "" {
			return status
		}
	case tools.ToolNameFileSearch:
		if status := formatFileSearchStatus(toolInput); status != "" {
			return status
		}
	case tools.ToolNameFileEdit:
		if status := formatFileEditStatus(toolInput); status != "" {
			return status
		}
	}

	return fmt.Sprintf("Running %s...", toolName)
}

func formatReadStatus(toolInput map[string]any) string {
	path := trimmedStringInput(toolInput, "path")
	if path == "" {
		return ""
	}

	parts := []string{fmt.Sprintf("file=%q", path)}
	if offset, ok := integerInput(toolInput, "offset"); ok {
		parts = append(parts, fmt.Sprintf("offset=%d", offset))
	}
	if limit, ok := integerInput(toolInput, "limit"); ok {
		parts = append(parts, fmt.Sprintf("limit=%d", limit))
	}
	return "Read: " + strings.Join(parts, " ")
}

func formatFileSearchStatus(toolInput map[string]any) string {
	parts := make([]string, 0, 3)
	if pattern := trimmedStringInput(toolInput, "name_pattern"); pattern != "" {
		parts = append(parts, fmt.Sprintf("files=%q", pattern))
	}
	if contains := trimmedStringInput(toolInput, "contains"); contains != "" {
		parts = append(parts, fmt.Sprintf("with=%q", contains))
	}
	if root := trimmedStringInput(toolInput, "root"); root != "" {
		parts = append(parts, fmt.Sprintf("at=%q", root))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Search: " + strings.Join(parts, " ")
}

func formatFileEditStatus(toolInput map[string]any) string {
	path := trimmedStringInput(toolInput, "path")
	if path == "" {
		return ""
	}

	parts := make([]string, 0, 2)
	if operation := trimmedStringInput(toolInput, "operation"); operation != "" {
		parts = append(parts, fmt.Sprintf("op=%q", operation))
	}
	parts = append(parts, fmt.Sprintf("file=%q", path))
	return "Edit: " + strings.Join(parts, " ")
}

func trimmedStringInput(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return strings.TrimSpace(value)
}

func integerInput(input map[string]any, key string) (int, bool) {
	switch value := input[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func (r *taskExecutionState) progressReporter(toolName string) func(string) {
	if r.onProgress == nil {
		return nil
	}
	return func(message string) {
		if strings.TrimSpace(message) == "" {
			return
		}
		r.onProgress(TaskProgressEvent{ToolName: toolName, Message: message, Timestamp: time.Now().UTC()})
	}
}

// autoAllowsTool reports the default confirmation decision for a tool when no
// explicit allow/deny/ask rule matches: read tools and in-workspace writes run
// without prompting; arbitrary execution and undeclared tools are gated.
func (r *taskExecutionState) autoAllowsTool(tool tools.Tool, input map[string]any) bool {
	if tool.Name() == tools.ToolNameUnix {
		command, _ := input["command"].(string)
		return isReadOnlyUnixCommandInDirs(command, r.state.Dirs)
	}

	switch permissionCategoryFor(tool) {
	case tools.PermissionRead:
		return true
	case tools.PermissionWrite:
		path, _ := input["path"].(string)
		return tools.PathWithinRoot(path, tools.ToolExecutionContext{
			RootDir:    r.state.Dirs.RootDir,
			CurrentDir: r.state.Dirs.CurrentDir,
		})
	default:
		return false
	}
}

func (r *taskExecutionState) recordThought(thought string) {
	if thought == "" {
		return
	}
	r.appendStep(TaskStep{Status: TaskStepStatusThought, Thought: thought})
}

func (r *taskExecutionState) recordFailure(response connector.LlmResponseWithTools, err error) {
	r.appendStep(TaskStep{
		Status:    TaskStepStatusFailed,
		Thought:   response.Response,
		ToolName:  response.ToolName,
		ToolInput: response.ToolInput,
		Error:     fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err),
	})
}

func (r *taskExecutionState) recordDeclined(response connector.LlmResponseWithTools) {
	r.appendStep(TaskStep{
		Status:    TaskStepStatusDeclined,
		Thought:   response.Response,
		ToolName:  response.ToolName,
		ToolInput: response.ToolInput,
		Message:   "user declined execution",
	})
}

func (r *taskExecutionState) recordSuccess(response connector.LlmResponseWithTools, toolResult string) {
	step := TaskStep{
		Status:     TaskStepStatusSucceeded,
		Thought:    response.Response,
		ToolName:   response.ToolName,
		ToolInput:  response.ToolInput,
		ToolOutput: toolResult,
	}
	if response.ToolName == ToolNameFinalAnswer {
		step.Status = TaskStepStatusFinalAnswer
		step.ToolOutput = ""
		step.FinalAnswer = toolResult
	}
	r.appendStep(step)
}

func (r *taskExecutionState) handleDirectoryChange(response connector.LlmResponseWithTools, logger *zap.SugaredLogger) {
	changeMessage, err := changeTaskDirectory(response.ToolInput, &r.state.Dirs)
	if err != nil {
		logger.Errorw("Directory change failed", "tool", response.ToolName, "error", err)
		r.recordFailure(response, err)
		return
	}
	r.recordSuccess(response, changeMessage)
}

func (r *taskExecutionState) finalAnswerResult(answer string) TaskRunResult {
	return TaskRunResult{Response: answer}
}

func (a *Agent) buildTaskTools(interaction TaskInteraction) map[string]tools.Tool {
	taskTools := make(map[string]tools.Tool, len(a.Tools))
	for name, tool := range a.Tools {
		taskTools[name] = tool
	}

	askUserTool := NewAskUserTool(interaction)
	taskTools[askUserTool.Name()] = askUserTool

	finalAnswerTool := NewFinalAnswerTool()
	taskTools[finalAnswerTool.Name()] = finalAnswerTool

	changeDirectoryTool := NewChangeDirectoryTool()
	taskTools[changeDirectoryTool.Name()] = changeDirectoryTool

	return taskTools
}

func (a *Agent) nextTaskResponse(ctx context.Context, params *connector.QueryParams, taskTools map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	if toolConnector, ok := a.Connector.(connector.ToolCallingConnector); ok && toolConnector.SupportsNativeToolCalling() {
		return toolConnector.QueryWithTool(ctx, params, taskTools)
	}
	return a.queryTaskActionFallback(ctx, params, taskTools)
}

func selectRawTaskOutput(outputs []taskToolOutput) taskToolOutput {
	return selectTaskRawOutput(outputs)
}

func toolInputRequestsFinal(input map[string]any) bool {
	return taskToolInputRequestsFinal(input)
}

// permissionCategoryFor returns a tool's declared permission category, treating
// tools that do not declare one (e.g. MCP tools) as PermissionExecute so they
// are gated by default.
func permissionCategoryFor(tool tools.Tool) tools.PermissionCategory {
	if categorized, ok := tool.(tools.CategorizedTool); ok {
		return categorized.PermissionCategory()
	}
	return tools.PermissionExecute
}
