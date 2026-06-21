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
	// tokenEstimateCharsPerToken is the divisor for the fallback token estimate
	// used when the provider does not report usage: characters exchanged ÷ 5.
	tokenEstimateCharsPerToken = 5
)

type TaskOptions struct {
	Allow        []string
	Deny         []string
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
	// MaxTurns and MaxToolCalls bound the run's step budget. A value of 0 means
	// "use the package default" (MaxTurns / MaxToolCalls consts).
	MaxTurns     int
	MaxToolCalls int
	// TokenBudget caps the estimated total tokens for the run. 0 means unlimited.
	TokenBudget int
	// EnabledTools selects which tools the run may use. A nil slice exposes all
	// available tools (subject to DisableExternalTools); a non-nil slice is an
	// explicit allow-list of tool names (external-facing tools run only when named
	// here).
	EnabledTools []string
	// DisableExternalTools, when true and EnabledTools is nil, drops external-facing
	// tools (web search, MCP) from the run. Routines set this; interactive task runs
	// leave it false to preserve access to all available tools.
	DisableExternalTools bool
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
	// TokensUsed is the run's estimated (or provider-reported) total token usage,
	// populated for both successful and failed runs.
	TokensUsed int
}

type TaskPhase string

const (
	TaskPhaseRunning    TaskPhase = "running"
	TaskPhaseFinalizing TaskPhase = "finalizing"
	TaskPhaseCompleted  TaskPhase = "completed"
	TaskPhaseFailed     TaskPhase = "failed"
)

type TaskDirs struct {
	RootDir           string
	CurrentDir        string
	ReadAllowedRoots  []string
	WriteAllowedPaths []string
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
	// TokenBudget caps the estimated tokens for the run; 0 means unlimited.
	// TokensUsed accumulates the running estimate (or provider-reported usage
	// when available) across model calls.
	TokenBudget int
	TokensUsed  int
	Phase       TaskPhase
	Dirs        TaskDirs
	Steps       []TaskStep
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
		return TaskRunResult{TokensUsed: result.TokensUsed}, ErrTaskTimeout
	}
	return result, err
}

// runTaskLoop uses a named result so a deferred stamp records the run's token
// usage on every return path once the execution state exists, including error
// returns (timeout, token-budget, model/tool failures).
func (a *Agent) runTaskLoop(ctx context.Context, s string, options TaskOptions) (result TaskRunResult, err error) {
	logger := log.Sugar()
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}

	run, err := a.newTaskExecutionState(s, options)
	if err != nil {
		return TaskRunResult{}, err
	}
	defer func() { result.TokensUsed = run.state.TokensUsed }()

	for run.state.Phase == TaskPhaseRunning && run.state.Iterations < run.state.MaxTurns && run.state.ToolCalls < run.state.MaxIterations {
		if err := ctx.Err(); err != nil {
			return TaskRunResult{}, err
		}
		if run.tokenBudgetExceeded() {
			return TaskRunResult{}, ErrTokenBudgetExceeded
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

	// A run that consumed its token budget stops here rather than spending more
	// tokens on a finalizing summary; the caller surfaces the distinct error.
	if run.tokenBudgetExceeded() {
		return TaskRunResult{}, ErrTokenBudgetExceeded
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

	maxTurns := MaxTurns
	if options.MaxTurns > 0 {
		maxTurns = options.MaxTurns
	}
	maxToolCalls := MaxToolCalls
	if options.MaxToolCalls > 0 {
		maxToolCalls = options.MaxToolCalls
	}

	interaction := options.Interaction
	confirmationRequester := taskUserConfirmationRequester{interaction: interaction}
	rememberer := taskPermissionRememberer{store: store}
	confirmations := NewConfirmationManager(options.Allow, ruleSets, confirmationRequester.RequestUserConfirmation, rememberer.Remember)
	// Per-run deny rules (e.g. a routine's lockdown) are appended above the per-run
	// allow rules (which NewConfirmationManager places at maxPriority+1), so a deny
	// always wins — including under auto-approve and over any config/local allow.
	if len(options.Deny) > 0 {
		confirmations.appendPatterns(options.Deny, ruleDeny, confirmations.maxPriority+2)
	}

	return &taskExecutionState{
		state: &TaskState{
			OriginalQuery: query,
			MaxIterations: maxToolCalls,
			MaxTurns:      maxTurns,
			TokenBudget:   options.TokenBudget,
			Phase:         TaskPhaseRunning,
			Dirs:          taskDirs,
			Steps:         make([]TaskStep, 0, maxTurns),
		},
		tools:             a.buildTaskTools(interaction, options.EnabledTools, options.DisableExternalTools),
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
	run.accountTokens(promptWithState, response.Response)
	return response, nil
}

// tokenBudgetExceeded reports whether the run has reached its token budget. A
// budget of 0 means unlimited and is never exceeded.
func (r *taskExecutionState) tokenBudgetExceeded() bool {
	return r.state.TokenBudget > 0 && r.state.TokensUsed >= r.state.TokenBudget
}

// accountTokens adds one model exchange to the running token total. No connector
// currently reports usage through the response type, so this uses the
// characters-exchanged ÷ 5 estimate; if a usage field is added later, prefer it
// here.
func (r *taskExecutionState) accountTokens(prompt, response string) {
	r.state.TokensUsed += estimateTokens(prompt, response)
}

func estimateTokens(parts ...string) int {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	return total / tokenEstimateCharsPerToken
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
	run.expandAllowedScopeForApprovedTool(tool, response)
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	return a.executeTaskTool(ctx, logger, run, tool, response)
}

func (r *taskExecutionState) expandAllowedScopeForApprovedTool(tool tools.Tool, response connector.LlmResponseWithTools) {
	scope := r.requestedAdditionalScope(tool, response.ToolInput)
	if scope == "" {
		return
	}
	switch tool.Name() {
	case tools.ToolNameFileSearch:
		r.state.Dirs.ReadAllowedRoots = appendAllowedRoot(r.state.Dirs.ReadAllowedRoots, scope)
	case tools.ToolNameFileEdit:
		r.state.Dirs.WriteAllowedPaths = appendAllowedPath(r.state.Dirs.WriteAllowedPaths, scope)
	}
	if r.state.Dirs.CurrentDir == "" {
		r.state.Dirs.CurrentDir = r.state.Dirs.RootDir
	}
}

func (r *taskExecutionState) requestedAdditionalScope(tool tools.Tool, input map[string]any) string {
	switch tool.Name() {
	case tools.ToolNameFileEdit:
		path, _ := input["path"].(string)
		return additionalWritePathForFilePath(path, r.state.Dirs)
	case tools.ToolNameFileSearch:
		root, _ := input["root"].(string)
		return additionalRootForDirPath(root, r.state.Dirs)
	default:
		return ""
	}
}

func (a *Agent) executeTaskTool(ctx context.Context, logger *zap.SugaredLogger, run *taskExecutionState, tool tools.Tool, response connector.LlmResponseWithTools) (TaskRunResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}
	run.emitStatus(TaskStatusRunningTool, formatRunningToolStatus(tool, response.ToolInput), response.ToolName, response.ToolInput)
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

func formatRunningToolStatus(tool tools.Tool, toolInput map[string]any) string {
	if formatter, ok := tool.(tools.StatusFormatter); ok {
		if status := formatter.ToolStatus(toolInput); status != "" {
			return status
		}
	}
	return fmt.Sprintf("Running %s...", tool.Name())
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
		if tool.Name() == tools.ToolNameFileSearch {
			root, _ := input["root"].(string)
			return additionalRootForDirPath(root, r.state.Dirs) == ""
		}
		return true
	case tools.PermissionWrite:
		path, _ := input["path"].(string)
		return tools.PathAllowedInContext(path, tools.ToolExecutionContext{
			RootDir:         r.state.Dirs.RootDir,
			CurrentDir:      r.state.Dirs.CurrentDir,
			AllowedRootDirs: []string{r.state.Dirs.RootDir},
			AllowedPaths:    r.state.Dirs.WriteAllowedPaths,
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

// buildTaskTools assembles the tools available for a run. enabledTools selects
// which of the agent's tools are exposed: a nil slice exposes every available
// tool, except that when disableExternal is set, external-facing tools (web
// search and MCP tools) are dropped (the routine default policy). A non-nil
// enabledTools is an explicit allow-list of tool names, and a named
// external-facing tool is included (explicit opt-in). The task-only tools
// (user_clarification, final_answer, change_directory) are always appended.
func (a *Agent) buildTaskTools(interaction TaskInteraction, enabledTools []string, disableExternal bool) map[string]tools.Tool {
	taskTools := make(map[string]tools.Tool, len(a.Tools))
	if enabledTools == nil {
		for name, tool := range a.Tools {
			if disableExternal && tools.IsExternalFacing(tool) {
				continue
			}
			taskTools[name] = tool
		}
	} else {
		allowed := make(map[string]struct{}, len(enabledTools))
		for _, name := range enabledTools {
			allowed[name] = struct{}{}
		}
		for name, tool := range a.Tools {
			if _, ok := allowed[name]; ok {
				taskTools[name] = tool
			}
		}
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
