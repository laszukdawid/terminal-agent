package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

// ErrEmptyQuery is returned when the query is empty.
var ErrEmptyQuery = fmt.Errorf("empty query")

const (
	MaxTokens                 = 400
	MaxIterations             = 10
	maxTaskActionFallbackRuns = 2
	UserClarificationToolName = "user_clarification"
	ToolNameChangeDirectory   = "change_directory"
	ToolNameFinalAnswer       = "final_answer"
)

type Agent struct {
	Connector connector.LLMConnector
	Tools     map[string]tools.Tool
	config    config.Config

	maxTokens        int
	systemPromptAsk  *string
	systemPromptTask *string
	device           string
}

func NewAgent(connector connector.LLMConnector, toolProvider tools.ToolProvider, config config.Config, systemPromptAsk, systemPromptTask string) *Agent {
	if connector == nil {
		panic("connector is nil")
	}

	allTools := toolProvider.GetAllTools()

	askUserTool := NewAskUserTool()
	allTools[askUserTool.Name()] = askUserTool

	finalAnswerTool := NewFinalAnswerTool()
	allTools[finalAnswerTool.Name()] = finalAnswerTool

	changeDirectoryTool := NewChangeDirectoryTool()
	allTools[changeDirectoryTool.Name()] = changeDirectoryTool

	return &Agent{
		Connector:        connector,
		Tools:            allTools,
		systemPromptAsk:  &systemPromptAsk,
		systemPromptTask: &systemPromptTask,
		config:           config,
		maxTokens:        MaxTokens,
	}
}

func (a *Agent) SetDevice(device string) {
	a.device = device
}

// Question sends a question to the agent and returns the response.
// It queries the model using the provided question string and the system prompt.
// If an error occurs during the query, it returns an empty string and an error.
func (a *Agent) Question(ctx context.Context, s string, isStream bool) (string, error) {
	if s == "" {
		return "", ErrEmptyQuery
	}
	qParams := connector.QueryParams{
		UserPrompt: &s,
		SysPrompt:  a.systemPromptAsk,
		Stream:     isStream,
		MaxTokens:  a.maxTokens,
		Device:     a.device,
	}
	res, err := a.Connector.Query(ctx, &qParams)
	return res, err
}

// Chat sends a message with conversation history to the agent and returns the response.
func (a *Agent) Chat(ctx context.Context, userMessage string, history []connector.Message, isStream bool) (string, error) {
	if userMessage == "" {
		return "", ErrEmptyQuery
	}
	qParams := connector.QueryParams{
		UserPrompt: &userMessage,
		SysPrompt:  a.systemPromptAsk,
		Messages:   history,
		Stream:     isStream,
		MaxTokens:  a.maxTokens,
		Device:     a.device,
	}
	res, err := a.Connector.Query(ctx, &qParams)
	return res, err
}

type TaskOptions struct {
	Allow []string
	Dirs  TaskDirs
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
	logger := utils.Logger.Sugar()
	ctx, cancel := context.WithTimeout(ctx, 900*time.Second) // 15 minutes timeout
	defer cancel()
	successfulToolOutputs := make([]taskToolOutput, 0, 1)

	taskDirs, err := resolveInitialTaskDirs(options.Dirs, a.config)
	if err != nil {
		return TaskRunResult{}, err
	}

	ruleSets, store, err := config.LoadPermissionRuleSets(taskDirs.RootDir)
	if err != nil {
		return TaskRunResult{}, fmt.Errorf("failed to load permissions: %w", err)
	}

	confirmations := NewConfirmationManager(options.Allow, ruleSets, func(action string, allow bool) error {
		return config.RememberPermission(store, action, allow)
	})

	// Create initial task state
	taskState := &TaskState{
		OriginalQuery:  s,
		CurrentThought: "I'll solve this task step by step.",
		Iterations:     0,
		MaxIterations:  MaxIterations, // Configurable maximum iterations
		Phase:          TaskPhaseRunning,
		Dirs:           taskDirs,
		Results:        make(map[string]string),
	}

	// Main agent loop
	for taskState.Phase == TaskPhaseRunning && taskState.Iterations < taskState.MaxIterations {
		taskState.Iterations++

		// Build context-aware prompt that includes previous steps and current state
		promptWithState := buildTaskPrompt(taskState)

		qParams := connector.QueryParams{
			UserPrompt: &promptWithState,
			SysPrompt:  a.systemPromptTask, // Using task-specific system prompt
			MaxTokens:  a.maxTokens,
			Device:     a.device,
		}

		response, err := a.nextTaskResponse(ctx, &qParams)
		if err != nil {
			logger.Errorw("Error querying model", "iteration", taskState.Iterations, "error", err)
			return TaskRunResult{}, fmt.Errorf("error during task processing: %w", err)
		}
		if response.Response != "" {
			taskState.CurrentThought = response.Response // Store the model's response
		}

		if response.ToolUse {
			// Reject unknown tools and malformed inputs before confirmation or execution.
			tool, err := resolveTaskToolCall(response.ToolName, response.ToolInput, a.Tools)
			if err != nil {
				logger.Errorw("Tool validation failed", "tool", response.ToolName, "error", err)
				taskState.LastError = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
				taskState.Results["tool_error"] = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
				taskState.Results["tool_input"] = fmt.Sprintf("Provided tool arguments: %v", response.ToolInput)
				continue
			}

			if response.ToolName == ToolNameChangeDirectory {
				changeMessage, err := changeTaskDirectory(response.ToolInput, &taskState.Dirs)
				if err != nil {
					logger.Errorw("Directory change failed", "tool", response.ToolName, "error", err)
					taskState.LastError = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
					taskState.Results["tool_error"] = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
					taskState.Results["tool_input"] = fmt.Sprintf("Provided tool arguments: %v", response.ToolInput)
				} else {
					taskState.Results[response.ToolName] = changeMessage
					taskState.LastError = ""
					taskState.CurrentThought = ""
				}
				continue
			}

			// Execute the selected tool
			action := BuildActionString(response.ToolName, response.ToolInput)
			if requiresConfirmation(response.ToolName) {
				allowed, err := confirmations.Confirm(action)
				if err != nil {
					logger.Errorw("Tool confirmation failed", "tool", response.ToolName, "error", err)
					return TaskRunResult{}, fmt.Errorf("tool confirmation failed: %w", err)
				}
				if !allowed {
					taskState.Results[fmt.Sprintf("%s confirmation", response.ToolName)] = "user declined execution"
					continue
				}
			}

			toolResult, err := runTaskTool(tool, response.ToolInput, taskState.Dirs)
			if err != nil {
				logger.Errorw("Tool execution failed", "tool", response.ToolName, "error", err)
				taskState.LastError = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
				taskState.Results["tool_error"] = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
				taskState.Results["tool_input"] = fmt.Sprintf("Provided tool arguments: %v", response.ToolInput)
			} else {
				if toolInputRequestsFinal(response.ToolInput) && isDisplayOrientedTool(response.ToolName) {
					taskState.Phase = TaskPhaseCompleted
					return TaskRunResult{
						Response:        toolResult,
						RawOutput:       toolResult,
						RawOutputTool:   response.ToolName,
						DirectRawOutput: true,
					}, nil
				}
				if response.ToolName != ToolNameFinalAnswer {
					successfulToolOutputs = append(successfulToolOutputs, taskToolOutput{
						ToolName: response.ToolName,
						Output:   toolResult,
					})
				}
				// TODO: Handle multiple executions of the same tool
				taskState.Results[fmt.Sprintf("%s justification", response.ToolName)] = response.Response
				taskState.Results[response.ToolName] = toolResult
				taskState.LastError = ""
				taskState.CurrentThought = ""
			}

			// If the final answer tool is used, the task is complete.
			if response.ToolName == ToolNameFinalAnswer {
				rawOutput := selectRawTaskOutput(successfulToolOutputs)
				taskState.Phase = TaskPhaseCompleted
				return TaskRunResult{Response: toolResult, RawOutput: rawOutput.Output, RawOutputTool: rawOutput.ToolName}, nil
			}

		}

		logger.Debugw("Task iteration complete",
			"iteration", taskState.Iterations,
			"phase", taskState.Phase)
	}

	// If we reached max iterations without completion
	if taskState.Phase == TaskPhaseRunning && taskState.Iterations >= taskState.MaxIterations {
		// Make a final attempt to synthesize what we've learned
		taskState.Phase = TaskPhaseFinalizing
		response, err := a.finalizeSummary(ctx, taskState)
		if err != nil {
			taskState.Phase = TaskPhaseFailed
			return TaskRunResult{}, err
		}
		rawOutput := selectRawTaskOutput(successfulToolOutputs)
		taskState.Phase = TaskPhaseCompleted
		return TaskRunResult{Response: response, RawOutput: rawOutput.Output, RawOutputTool: rawOutput.ToolName}, nil
	}

	return TaskRunResult{}, fmt.Errorf("task ended without an explicit completion path")
}

func (a *Agent) nextTaskResponse(ctx context.Context, params *connector.QueryParams) (connector.LlmResponseWithTools, error) {
	if toolConnector, ok := a.Connector.(connector.ToolCallingConnector); ok && toolConnector.SupportsNativeToolCalling() {
		return toolConnector.QueryWithTool(ctx, params, a.Tools)
	}
	return a.queryTaskActionFallback(ctx, params)
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

func (a *Agent) queryTaskActionFallback(ctx context.Context, params *connector.QueryParams) (connector.LlmResponseWithTools, error) {
	if params == nil {
		return connector.LlmResponseWithTools{}, fmt.Errorf("task query params cannot be nil")
	}

	fallbackParams := *params
	fallbackParams.Messages = nil
	fallbackParams.Stream = false
	fallbackParams.OnStream = nil
	fallbackSystemPrompt := buildTaskActionSystemPrompt(params.SysPrompt)
	fallbackParams.SysPrompt = &fallbackSystemPrompt

	var lastRaw string
	var lastErr error
	for attempt := 0; attempt < maxTaskActionFallbackRuns; attempt++ {
		prompt := a.buildTaskActionPrompt(params, lastRaw, lastErr)
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

		response := connector.LlmResponseWithTools{Response: strings.TrimSpace(parsed.Thought)}
		switch parsed.normalizedAction() {
		case "final_answer":
			response.ToolUse = true
			response.ToolName = ToolNameFinalAnswer
			response.ToolInput = map[string]any{"answer": strings.TrimSpace(parsed.Answer)}
		case "tool":
			response.ToolUse = true
			response.ToolName = parsed.normalizedToolName()
			if response.ToolName == "" {
				lastRaw = raw
				lastErr = fmt.Errorf("task fallback response is missing tool name")
				continue
			}
			if parsed.Input == nil {
				parsed.Input = map[string]any{}
			}
			response.ToolInput = parsed.Input
		default:
			lastRaw = raw
			lastErr = fmt.Errorf("task fallback response has invalid action %q", parsed.normalizedAction())
			continue
		}

		return response, nil
	}

	return connector.LlmResponseWithTools{}, fmt.Errorf("task fallback produced invalid action after %d attempts: %w", maxTaskActionFallbackRuns, lastErr)
}


func buildTaskActionSystemPrompt(base *string) string {
	if base == nil || strings.TrimSpace(*base) == "" {
		return strings.TrimSpace(taskActionSystemPromptSuffix)
	}
	return strings.TrimSpace(*base) + taskActionSystemPromptSuffix
}

func (a *Agent) buildTaskActionPrompt(params *connector.QueryParams, previousRaw string, previousErr error) string {
	toolsText := formatTaskActionTools(a.Tools)
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
		return taskActionFallbackResponse{}, fmt.Errorf("task fallback response did not contain a JSON object: %q", trimmed)
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

// TaskState tracks the state of the agent's work on a task
type TaskState struct {
	OriginalQuery  string
	CurrentThought string
	Iterations     int
	MaxIterations  int
	Phase          TaskPhase
	Dirs           TaskDirs
	Results        map[string]string // Holds results from tools and user interactions
	LastError      string
}

// AgentDecision represents a parsed agent response
type AgentDecision struct {
	ActionType  string // "complete", "use_tool", "ask_user", "think"
	ToolName    string
	ToolInput   map[string]any // Input for the tool
	UserQuery   string
	NextThought string
	FinalAnswer string
}

// buildTaskPrompt constructs a context-aware prompt for the agent
func buildTaskPrompt(state *TaskState) string {
	const promptTemplate = `Original task: {{.OriginalQuery}}

Current phase: {{.Phase}}
Iteration: {{.Iterations}} of {{.MaxIterations}}
Task root directory: {{.Dirs.RootDir}}
Current working directory: {{.Dirs.CurrentDir}}

{{if .HasResults}}Information gathered so far:
{{range $source, $result := .Results}}Results source: {{$source}}
<RESULTS>
{{$result}}
</RESULTS>
{{end}}
{{end}}

{{if .LastError}}Latest error:
{{.LastError}}
{{end}}

Current thought: {{.CurrentThought}}

What should I do next to complete this task? Should I use one of provided tools? Is the task finished?  If so, provide the final answer.
`

	// Prepare template data
	data := struct {
		*TaskState
		HasResults bool
	}{
		TaskState:  state,
		HasResults: len(state.Results) > 0,
	}

	// Process each result to truncate if needed
	processedResults := make(map[string]string, len(state.Results))
	for source, result := range state.Results {
		processedResults[source] = TruncateString(result, 2000)
		// processedResults[source] = result
	}
	data.Results = processedResults

	// Parse and execute the template
	tmpl, err := template.New("prompt").Parse(promptTemplate)
	if err != nil {
		// Fallback to simple string format in case of template error
		return fmt.Sprintf("Original task: %s\nError creating prompt template: %v", state.OriginalQuery, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Fallback to simple string format in case of template execution error
		return fmt.Sprintf("Original task: %s\nError executing prompt template: %v", state.OriginalQuery, err)
	}

	return strings.TrimSpace(buf.String())
}

// getUserInput reads input from the user
func getUserInput() (string, error) {
	fmt.Print("> ")
	var input string
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input = scanner.Text()
	}
	return input, scanner.Err()
}

// finalizeSummary creates a final answer when max iterations are reached
func (a *Agent) finalizeSummary(ctx context.Context, state *TaskState) (string, error) {
	summaryPrompt := `I've been working on this task: {{.OriginalQuery}}

Task root directory: {{.Dirs.RootDir}}
Current working directory: {{.Dirs.CurrentDir}}

Here's what I've learned and done so far:

{{range $source, $result := .Results}}- From {{$source}}: {{$result}}
{{end}}
{{if .LastError}}
Latest error encountered:
{{.LastError}}
{{end}}
I've reached the maximum number of iterations. Based on the above, provide a comprehensive final answer.
`

	// Prepare template data
	data := struct {
		*TaskState
	}{
		TaskState: state,
	}
	summaryTemplate, err := template.New("summary").Parse(summaryPrompt)
	if err != nil {
		return "", fmt.Errorf("error creating summary template: %w", err)
	}

	var summary bytes.Buffer
	if err := summaryTemplate.Execute(&summary, data); err != nil {
		// Fallback to simple string format in case of template execution error
		return fmt.Sprintf("Couldn't finalize the answer for question: %s.\n\nErr: %v", state.OriginalQuery, err), nil
	}

	qParams := connector.QueryParams{
		UserPrompt: StringPtr(summary.String()),
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens * 2, // Allow more tokens for summary
		Device:     a.device,
	}

	return a.Connector.Query(ctx, &qParams)
}

// Helper function for string pointer
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

func isDisplayOrientedTool(toolName string) bool {
	return isTaskDisplayOrientedTool(toolName)
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
}

func NewAskUserTool() *askUserTool {
	return &askUserTool{
		name:        UserClarificationToolName,
		description: "Ask the user for clarification or additional information. This tool is used when the model needs more context to proceed with the task.",
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
	return fmt.Sprintf("Help for %s: %s\n\n%s",
		t.Name(), t.Description(), t.InputSchema())
}

func (t *askUserTool) RunSchema(input map[string]any) (string, error) {
	// Extract question from input
	question, ok := input["question"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract question from tool input")
	}

	// Ask the user for clarification
	fmt.Println("\nNeed clarification:", question)
	userInput, err := getUserInput()
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
	return fmt.Sprintf("Help for %s: %s\n\n%s",
		t.Name(), t.Description(), t.InputSchema())
}

func (t *finalAnswerTool) RunSchema(input map[string]any) (string, error) {
	// Extract answer from input
	answer, ok := input["answer"].(string)
	if !ok {
		return "", fmt.Errorf("failed to extract answer from tool input")
	}

	// Provide the final answer
	return answer, nil
}

func (t *finalAnswerTool) Run(input *string) (string, error) {
	return t.RunSchema(map[string]any{"answer": *input})
}
