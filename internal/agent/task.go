package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	log "github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	MaxToolCalls              = 10
	MaxTurns                  = 50
	maxTaskActionFallbackRuns = 2
	UserClarificationToolName = "user_clarification"
	ToolNameChangeDirectory   = "change_directory"
	ToolNameFinalAnswer       = "final_answer"
)

type TaskOptions struct {
	Allow       []string
	Dirs        TaskDirs
	Interaction TaskInteraction
	OnStep      func(TaskStep)
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

	return confirmationDecision{allowed: decision.Allowed, remember: decision.Remember}, nil
}

type taskPermissionRememberer struct {
	store config.PermissionStore
}

func (r taskPermissionRememberer) Remember(action string, allow bool) error {
	return config.RememberPermission(r.store, action, allow)
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
	logger := log.Sugar()
	ctx, cancel := context.WithTimeout(ctx, 900*time.Second)
	defer cancel()
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
	}, nil
}

func (a *Agent) runTaskIteration(ctx context.Context, logger *zap.SugaredLogger, run *taskExecutionState) (TaskRunResult, bool, error) {
	response, err := a.queryTaskResponse(ctx, run)
	if err != nil {
		logger.Debugw("Error querying model", "iteration", run.state.Iterations, "error", err)
		return TaskRunResult{}, false, fmt.Errorf("error during task processing: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, false, err
	}

	if !response.ToolUse {
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

	allowed, err := run.confirmTool(response)
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
	toolResult, err := runTaskTool(ctx, tool, response.ToolInput, run.state.Dirs)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return TaskRunResult{}, false, ctxErr
		}
		logger.Errorw("Tool execution failed", "tool", response.ToolName, "error", err)
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
		return run.finalAnswerResult(toolResult), true, nil
	}
	if toolInputRequestsFinal(response.ToolInput) && toolSupportsFinal(tool) {
		run.state.Phase = TaskPhaseCompleted
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
	response, err := a.finalizeSummary(ctx, run.state)
	if err != nil {
		run.state.Phase = TaskPhaseFailed
		return TaskRunResult{}, err
	}

	run.state.Phase = TaskPhaseCompleted
	rawOutput := selectRawTaskOutput(run.successfulOutputs)
	return TaskRunResult{Response: response, RawOutput: rawOutput.Output, RawOutputTool: rawOutput.ToolName}, nil
}

func (r *taskExecutionState) confirmTool(response connector.LlmResponseWithTools) (bool, error) {
	if !requiresConfirmation(response.ToolName) {
		return true, nil
	}
	return r.confirmations.Confirm(BuildActionString(response.ToolName, response.ToolInput))
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
	rawOutput := selectRawTaskOutput(r.successfulOutputs)
	return TaskRunResult{Response: answer, RawOutput: rawOutput.Output, RawOutputTool: rawOutput.ToolName}
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

type taskActionFallbackResponse struct {
	Action   string         `json:"action,omitempty"`
	Type     string         `json:"type,omitempty"`
	Tool     string         `json:"tool,omitempty"`
	ToolName string         `json:"tool_name,omitempty"`
	Input    map[string]any `json:"input,omitempty"`
	Answer   string         `json:"answer,omitempty"`
	Thought  string         `json:"thought,omitempty"`
}

const taskActionSystemPromptSuffix = `

For the next response, return exactly one JSON object and no surrounding markdown or prose. Put any brief reasoning inside the "thought" field.`

const taskActionPromptTemplate = `Decide the next task step.

Return exactly one JSON object. Do not return a schema, Markdown, or code fences.

Valid response shapes:
{"action":"tool","tool":"<tool name>","input":{...},"thought":"brief reason"}
{"action":"final_answer","answer":"<final answer>","thought":"brief reason"}

Rules:
- Use "action", not "type".
- Use "tool", not "tool_name".
- The "input" field must contain the actual tool arguments for the chosen tool.
- Do not return a JSON schema.
- If the raw output of unix, python, or file_search fully answers the request, set "final": true in that tool input.

Available tools:
%s

Task context:
%s

Return valid JSON only.`

const taskActionRetryPromptTemplate = `Your previous response was invalid.

Previous response:
%s

Problem:
%s

Return exactly one corrected JSON object and nothing else.

Valid response shapes:
{"action":"tool","tool":"<tool name>","input":{...},"thought":"brief reason"}
{"action":"final_answer","answer":"<final answer>","thought":"brief reason"}

Available tools:
%s

Task context:
%s

Return valid JSON only.`

func (a *Agent) queryTaskActionFallback(ctx context.Context, params *connector.QueryParams, taskTools map[string]tools.Tool) (connector.LlmResponseWithTools, error) {
	fallbackParams := *params
	fallbackParams.Messages = nil
	fallbackParams.Stream = false
	fallbackParams.OnStream = nil
	fallbackSystemPrompt := buildTaskActionSystemPrompt(params.SysPrompt)
	fallbackParams.SysPrompt = &fallbackSystemPrompt

	var lastRaw string
	var lastErr error
	for attempt := 0; attempt < maxTaskActionFallbackRuns; attempt++ {
		if err := ctx.Err(); err != nil {
			return connector.LlmResponseWithTools{}, err
		}
		prompt := a.buildTaskActionPrompt(params, taskTools, lastRaw, lastErr)
		fallbackParams.UserPrompt = &prompt

		raw, err := a.Connector.Query(ctx, &fallbackParams)
		if err != nil {
			return connector.LlmResponseWithTools{}, err
		}

		parsed, err := parseTaskActionFallbackResponse(raw)
		if err != nil {
			lastRaw = raw
			lastErr = err
			continue
		}

		response, err := buildTaskActionResponse(parsed)
		if err != nil {
			lastRaw = raw
			lastErr = err
			continue
		}

		return response, nil
	}

	return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback produced invalid action after %d attempts: %w", maxTaskActionFallbackRuns, lastErr)
}

func buildTaskActionSystemPrompt(base *string) string {
	if strings.TrimSpace(*base) == "" {
		return strings.TrimSpace(taskActionSystemPromptSuffix)
	}
	return strings.TrimSpace(*base) + taskActionSystemPromptSuffix
}

func buildTaskActionResponse(parsed taskActionFallbackResponse) (connector.LlmResponseWithTools, error) {
	response := connector.LlmResponseWithTools{Response: strings.TrimSpace(parsed.Thought)}
	action := parsed.normalizedAction()
	if action == "final_answer" {
		response.ToolUse = true
		response.ToolName = ToolNameFinalAnswer
		response.ToolInput = map[string]any{"answer": strings.TrimSpace(parsed.Answer)}
		return response, nil
	}
	if action != "tool" {
		return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback response has invalid action %q", action)
	}

	toolName := parsed.normalizedToolName()
	if toolName == "" {
		return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback response is missing tool name")
	}
	if parsed.Input == nil {
		parsed.Input = map[string]any{}
	}

	response.ToolUse = true
	response.ToolName = toolName
	response.ToolInput = parsed.Input
	return response, nil
}

func (a *Agent) buildTaskActionPrompt(params *connector.QueryParams, taskTools map[string]tools.Tool, previousRaw string, previousErr error) string {
	toolsText := formatTaskActionTools(taskTools)
	contextText := formatTaskActionContext(params)
	if previousErr == nil {
		return fmt.Sprintf(taskActionPromptTemplate, toolsText, contextText)
	}
	return fmt.Sprintf(
		taskActionRetryPromptTemplate,
		TruncateString(strings.TrimSpace(previousRaw), 1200),
		previousErr.Error(),
		toolsText,
		contextText,
	)
}

func formatTaskActionTools(execTools map[string]tools.Tool) string {
	names := make([]string, 0, len(execTools))
	for name := range execTools {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]string, 0, len(names))
	for _, name := range names {
		tool := execTools[name]
		entry := fmt.Sprintf("- %s: %s", tool.Name(), tool.Description())
		schemaHints := formatTaskActionSchemaHints(tool.InputSchema())
		if schemaHints != "" {
			entry += "\n" + schemaHints
		}
		entries = append(entries, entry)
	}
	return strings.Join(entries, "\n")
}

func formatTaskActionSchemaHints(schema map[string]any) string {
	properties, ok := normalizeSchemaMap(schema["properties"])
	if !ok || len(properties) == 0 {
		return ""
	}

	required := make(map[string]bool)
	for _, name := range schemaStringSlice(schema["required"]) {
		required[name] = true
	}

	fieldNames := make([]string, 0, len(properties))
	for name := range properties {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	lines := make([]string, 0, len(fieldNames)+2)
	for _, fieldName := range fieldNames {
		definition, ok := normalizeSchemaDefinition(properties[fieldName])
		if !ok {
			continue
		}
		line := fmt.Sprintf("  - %s (%s, %s)", fieldName, schemaFieldType(definition), requiredLabel(required[fieldName]))
		if description, _ := definition["description"].(string); description != "" {
			line += ": " + description
		}
		if enumValues := schemaStringSlice(definition["enum"]); len(enumValues) > 0 {
			line += fmt.Sprintf(" Allowed values: %s.", strings.Join(enumValues, ", "))
		}
		lines = append(lines, line)
	}

	if requirementHint := formatTaskActionRequirementHints(schema); requirementHint != "" {
		lines = append(lines, "  - requirement: "+requirementHint)
	}

	return strings.Join(lines, "\n")
}

func schemaFieldType(definition map[string]any) string {
	if typeName, _ := definition["type"].(string); typeName != "" {
		return typeName
	}
	return "value"
}

func requiredLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func formatTaskActionRequirementHints(schema map[string]any) string {
	branches := schemaAnySlice(schema["oneOf"])
	label := "exactly one of"
	if len(branches) == 0 {
		branches = schemaAnySlice(schema["anyOf"])
		label = "at least one of"
	}
	if len(branches) == 0 {
		return ""
	}

	options := make([]string, 0, len(branches))
	for _, branch := range branches {
		branchSchema, ok := normalizeSchemaDefinition(branch)
		if !ok {
			continue
		}
		requiredFields := schemaStringSlice(branchSchema["required"])
		if len(requiredFields) == 0 {
			continue
		}
		options = append(options, strings.Join(requiredFields, ", "))
	}
	if len(options) == 0 {
		return ""
	}
	return fmt.Sprintf("provide %s: %s", label, strings.Join(options, " or "))
}

func formatTaskActionContext(params *connector.QueryParams) string {
	if params == nil {
		return ""
	}
	parts := make([]string, 0, len(params.Messages)+1)
	for _, msg := range params.Messages {
		role := strings.ToUpper(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "USER"
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, msg.Content))
	}
	if params.UserPrompt != nil && *params.UserPrompt != "" {
		parts = append(parts, "USER: "+*params.UserPrompt)
	}
	return strings.Join(parts, "\n\n")
}

func parseTaskActionFallbackResponse(raw string) (taskActionFallbackResponse, error) {
	trimmed := strings.TrimSpace(raw)
	candidates := extractJSONObjects(trimmed)
	if len(candidates) == 0 {
		return taskActionFallbackResponse{}, fmt.Errorf("task fallback response did not contain a valid JSON object")
	}

	var lastErr error
	for _, candidate := range candidates {
		var response taskActionFallbackResponse
		if err := json.Unmarshal([]byte(candidate), &response); err != nil {
			lastErr = fmt.Errorf("failed to parse task fallback response as JSON: %w", err)
			continue
		}

		action := response.normalizedAction()
		if action == "tool" || action == "final_answer" {
			return response, nil
		}
		lastErr = fmt.Errorf("task fallback response has invalid action %q", action)
	}

	if lastErr != nil {
		return taskActionFallbackResponse{}, lastErr
	}
	return taskActionFallbackResponse{}, fmt.Errorf("task fallback response did not contain a valid action object")
}

func (r taskActionFallbackResponse) normalizedAction() string {
	action := strings.TrimSpace(r.Action)
	if action == "" {
		action = strings.TrimSpace(r.Type)
	}
	action = strings.ToLower(action)
	switch action {
	case "final", "answer", "complete", "done":
		return "final_answer"
	default:
		if action == "" {
			if strings.TrimSpace(r.Answer) != "" {
				return "final_answer"
			}
			if r.Input != nil || strings.TrimSpace(r.Tool) != "" || strings.TrimSpace(r.ToolName) != "" {
				return "tool"
			}
		}
		return action
	}
}

func (r taskActionFallbackResponse) normalizedToolName() string {
	if toolName := strings.TrimSpace(r.Tool); toolName != "" {
		return toolName
	}
	return strings.TrimSpace(r.ToolName)
}

func extractJSONObjects(raw string) []string {
	if raw == "" {
		return nil
	}

	objects := []string{}
	depth := 0
	start := -1
	inString := false
	escaped := false

	for i, r := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch r {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				objects = append(objects, raw[start:i+1])
				start = -1
			}
		}
	}

	return objects
}

func buildTaskPrompt(state *TaskState) string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Original task: %s\n\n", state.OriginalQuery)
	fmt.Fprintf(&prompt, "Current phase: %s\n", state.Phase)
	fmt.Fprintf(&prompt, "Turn: %d of %d\n", state.Iterations, state.MaxTurns)
	fmt.Fprintf(&prompt, "Tool calls: %d of %d\n", state.ToolCalls, state.MaxIterations)
	fmt.Fprintf(&prompt, "Task root directory: %s\n", state.Dirs.RootDir)
	fmt.Fprintf(&prompt, "Current working directory: %s", state.Dirs.CurrentDir)

	if history := renderTaskHistoryForPrompt(state.Steps); history != "" {
		prompt.WriteString("\n\nOrdered step history:\n")
		prompt.WriteString(history)
	}

	prompt.WriteString("\n\nWhat should I do next to complete this task? Should I use one of provided tools? Is the task finished? If so, provide the final answer.")
	return strings.TrimSpace(prompt.String())
}

func (a *Agent) finalizeSummary(ctx context.Context, state *TaskState) (string, error) {
	var summary strings.Builder
	fmt.Fprintf(&summary, "I've been working on this task: %s\n\n", state.OriginalQuery)
	fmt.Fprintf(&summary, "Task root directory: %s\n", state.Dirs.RootDir)
	fmt.Fprintf(&summary, "Current working directory: %s", state.Dirs.CurrentDir)

	if history := renderTaskHistoryForSummary(state.Steps); history != "" {
		summary.WriteString("\n\nOrdered step history:\n")
		summary.WriteString(history)
	}

	summary.WriteString("\n\nI've reached the task budget limit. Based on the above, provide a comprehensive final answer.")

	qParams := connector.QueryParams{
		UserPrompt: StringPtr(summary.String()),
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens * 2,
		Device:     a.device,
	}

	return a.Connector.Query(ctx, &qParams)
}

func StringPtr(s string) *string {
	return &s
}

func TruncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "... [Truncated]"
	}
	return s
}

func selectRawTaskOutput(outputs []taskToolOutput) taskToolOutput {
	return selectTaskRawOutput(outputs)
}

func toolInputRequestsFinal(input map[string]any) bool {
	return taskToolInputRequestsFinal(input)
}

func requiresConfirmation(toolName string) bool {
	switch toolName {
	case tools.ToolNameUnix, tools.ToolNameFileEdit, tools.ToolNamePython:
		return true
	default:
		return false
	}
}

type askUserTool struct {
	name        string
	description string
	inputSchema map[string]any
	interaction TaskInteraction
}

func NewAskUserTool(interaction TaskInteraction) *askUserTool {
	return &askUserTool{
		name:        UserClarificationToolName,
		description: "Ask the user for clarification or additional information. This tool is used when the model needs more context to proceed with the task.",
		interaction: interaction,
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]string{
					"type":        "string",
					"description": "Ask a question to the user to get more info required to solve or clarify their problem",
				},
			},
			"required": []string{"question"},
		},
	}
}

func (t *askUserTool) Name() string {
	return t.name
}

func (t *askUserTool) Description() string {
	return t.description
}

func (t *askUserTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *askUserTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s", t.Name(), t.Description(), t.InputSchema())
}

func (t *askUserTool) RunSchema(input map[string]any) (string, error) {
	question, ok := input["question"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract question from tool input")
	}

	if t.interaction == nil {
		return "", ErrTaskInteractionRequired
	}

	userInput, err := t.interaction.Clarify(TaskClarificationRequest{Question: question})
	if err != nil {
		return "", fmt.Errorf("user input error: %w", err)
	}

	return userInput, nil
}

func (t *askUserTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"question": *input})
}

type finalAnswerTool struct {
	name        string
	description string
	inputSchema map[string]any
}

func NewFinalAnswerTool() *finalAnswerTool {
	return &finalAnswerTool{
		name:        ToolNameFinalAnswer,
		description: "Provide the final answer to the task.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]string{
					"type":        "string",
					"description": "Call this tool when the task is complete and you want to provide the final answer.",
				},
			},
			"required": []string{"answer"},
		},
	}
}

func (t *finalAnswerTool) Name() string {
	return t.name
}

func (t *finalAnswerTool) Description() string {
	return t.description
}

func (t *finalAnswerTool) InputSchema() map[string]any {
	return t.inputSchema
}

func (t *finalAnswerTool) HelpText() string {
	return fmt.Sprintf("Help for %s: %s\n\n%s", t.Name(), t.Description(), t.InputSchema())
}

func (t *finalAnswerTool) RunSchema(input map[string]any) (string, error) {
	answer, ok := input["answer"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract answer from tool input")
	}

	return answer, nil
}

func (t *finalAnswerTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"answer": *input})
}
