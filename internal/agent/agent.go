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

const MaxTokens = 400

type Agent struct {
	Connector connector.LLMConnector
	Tools     map[string]tools.Tool

	maxTokens        int
	systemPromptAsk  *string
	systemPromptTask *string
}

func NewAgent(connector connector.LLMConnector, toolProvider tools.ToolProvider, config config.Config) *Agent {
	if connector == nil {
		panic("connector is nil")
	}

	spAsk := strings.Replace(SystemPromptAsk, "{{header}}", SystemPromptHeader, 1)
	spTask := strings.Replace(SystemPromptTask, "{{header}}", SystemPromptHeader, 1)

	allTools := toolProvider.GetAllTools()

	askUserTool := NewAskUserTool()
	allTools[askUserTool.Name()] = askUserTool

	finalAnswerTool := NewFinalAnswerTool()
	allTools[finalAnswerTool.Name()] = finalAnswerTool

	return &Agent{
		Connector: connector,
		// toolProvider:     toolProvider,
		Tools:            allTools,
		systemPromptAsk:  &spAsk,
		systemPromptTask: &spTask,
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

func (a *Agent) Task(ctx context.Context, s string) (string, error) {
	logger := utils.Logger.Sugar()
	ctx, cancel := context.WithTimeout(ctx, 900*time.Second) // 15 minutes timeout
	defer cancel()

	// Create initial task state
	taskState := &TaskState{
		OriginalQuery:    s,
		CurrentThought:   "I'll solve this task step by step.",
		CompletionStatus: 0, // 0% complete
		Iterations:       0,
		MaxIterations:    10, // Configurable maximum iterations
		Results:          make(map[string]string),
	}

	// Main agent loop
	for taskState.CompletionStatus < 100 && taskState.Iterations < taskState.MaxIterations {
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
			return "", fmt.Errorf("error during task processing: %w", err)
		}
		taskState.CompletionStatus = min(taskState.CompletionStatus+5, 85)

		if response.ToolUse {
			// Execute the selected tool
			tool := a.Tools[response.ToolName]
			toolResult, err := tool.RunSchema(response.ToolInput)
			if err != nil {
				logger.Errorw("Tool execution failed", "tool", response.ToolName, "error", err)
				taskState.Results["tool_error"] = fmt.Sprintf("Failed to execute %s: %v", response.ToolName, err)
				taskState.Results["tool_input"] = fmt.Sprintf("Provided tool arguments: %v", response.ToolInput)
			} else {
				// TODO: Handle multiple executions of the same tool
				taskState.Results[fmt.Sprintf("%s justification", response.ToolName)] = response.Response
				taskState.Results[response.ToolName] = toolResult
				taskState.CompletionStatus = min(taskState.CompletionStatus+5, 90) // Progress but not complete
				taskState.CurrentThought = ""
			}

			// If the final answer tool is used, set the completion status to 100%
			if response.ToolName == "final_answer" {
				taskState.CompletionStatus = 100
				return toolResult, nil
			}

		}

		logger.Debugw("Task iteration complete",
			"iteration", taskState.Iterations,
			"status", taskState.CompletionStatus)
	}

	// If we reached max iterations without completion
	if taskState.CompletionStatus < 100 {
		// Make a final attempt to synthesize what we've learned
		return a.finalizeSummary(ctx, taskState)
	}

	return "Task completed successfully.", nil
}

// TaskState tracks the state of the agent's work on a task
type TaskState struct {
	OriginalQuery    string
	CurrentThought   string
	CompletionStatus int // 0-100%
	Iterations       int
	MaxIterations    int
	Results          map[string]string // Holds results from tools and user interactions
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

Current progress: {{.CompletionStatus}}%
Iteration: {{.Iterations}} of {{.MaxIterations}}

{{if .HasResults}}Information gathered so far:
{{range $source, $result := .Results}}Results source: {{$source}}
<RESULTS>
{{$result}}
</RESULTS>
{{end}}
{{end}}

Current thought: {{.CurrentThought}}

What should I do next to complete this task? Is the task finished? If so, provide the final answer.
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

Here's what I've learned and done so far:

{{range $source, $result := .Results}}- From {{$source}}: {{$result}}
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
		UserPrompt: StringPtr(summaryPrompt),
		SysPrompt:  a.systemPromptTask,
		MaxTokens:  a.maxTokens * 2, // Allow more tokens for summary
	}

	return a.Connector.Query(ctx, &qParams)
}

// Helper function for string pointer
func StringPtr(s string) *string {
	return &s
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TruncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "... [Truncated]"
	}
	return s
}

type askUserTool struct {
	name        string
	description string
	inputSchema map[string]any
}

func NewAskUserTool() *askUserTool {
	return &askUserTool{
		name:        "user_clarification",
		description: "Ask the user for clarification or additional information.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]string{
					"type":        "string",
					"description": "Ask a question to the user to get more info required to solve or clarify their problem",
				},
			},
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
		name:        "final_answer",
		description: "Provide the final answer to the task.",
		inputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]string{
					"type":        "string",
					"description": "Call this tool when the task is complete and you want to provide the final answer.",
				},
			},
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
