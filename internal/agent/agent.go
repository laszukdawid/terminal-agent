package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
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
		}

		// Query the model with tools
		response, err := a.Connector.QueryWithTool(ctx, &qParams, a.Tools)
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
