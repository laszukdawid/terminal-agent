package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

// ErrEmptyQuery is returned when the query is empty.
var ErrEmptyQuery = fmt.Errorf("empty query")

type Agent struct {
	Connector connector.LLMConnector
	// UnixTool  tools.UnixTool
	Tools map[string]tools.Tool

	systemPromptAsk  *string
	systemPromptTask *string
}

func NewAgent(connector connector.LLMConnector) *Agent {
	if connector == nil {
		panic("connector is nil")
	}

	spAsk := strings.Replace(SystemPromptAsk, "{{header}}", SystemPromptHeader, 1)
	spTask := strings.Replace(SystemPromptTask, "{{header}}", SystemPromptHeader, 1)

	unixTool := tools.NewUnixTool(nil)
	websearchTool := tools.NewWebsearchTool()
	tools := map[string]tools.Tool{
		unixTool.Name():      unixTool,
		websearchTool.Name(): websearchTool,
	}

	return &Agent{
		Connector:        connector,
		Tools:            tools,
		systemPromptAsk:  &spAsk,
		systemPromptTask: &spTask,
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
		MaxTokens:  200,
	}
	res, err := a.Connector.Query(ctx, &qParams)
	return res, err
}

// Task is about asking and executing.
// The query is sent to the model with relevant tools.
// The model then decides whether and which tools to use.
// Each tool has its own way of executing the task.
func (a *Agent) Task(ctx context.Context, s string) (string, error) {
	sugar := utils.Logger.Sugar()

	qParams := connector.QueryParams{
		UserPrompt: &s,
		SysPrompt:  a.systemPromptAsk,
	}

	// Query the model with the task
	// TODO: There's only a single iterations with tools; either get it or not.
	res, err := a.Connector.QueryWithTool(ctx, &qParams)
	sugar.Debugw("Query response", "res", res, "err", err)
	return res, err
}
