package agent

import (
	"context"
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/config"
	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

// ErrEmptyQuery is returned when the query is empty.
var ErrEmptyQuery = fmt.Errorf("empty query")

const MaxTokens = 400

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

	allTools := filterAvailableTools(toolProvider.GetAllTools())

	askUserTool := NewAskUserTool(nil)
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

func filterAvailableTools(allTools map[string]tools.Tool) map[string]tools.Tool {
	filtered := make(map[string]tools.Tool, len(allTools))
	for name, tool := range allTools {
		availabilityAwareTool, ok := tool.(tools.AvailabilityAwareTool)
		if ok && !availabilityAwareTool.IsAvailable() {
			continue
		}

		filtered[name] = tool
	}

	return filtered
}

func (a *Agent) SetDevice(device string) {
	a.device = device
}

// Question sends a question to the agent and returns the response.
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
