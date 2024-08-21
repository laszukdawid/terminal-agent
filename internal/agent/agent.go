package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
)

type Agent struct {
	Connector connector.LLMConnector
	UnixTool  tools.UnixTool

	systemPromptAsk  *string
	systemPromptTask *string
}

func NewAgent(connector connector.LLMConnector) *Agent {

	spAsk := strings.Replace(SystemPromptAsk, "{{header}}", SystemPromptHeader, 1)
	spTask := strings.Replace(SystemPromptTask, "{{header}}", SystemPromptHeader, 1)

	unixTool := tools.NewUnixTool(nil)

	return &Agent{
		Connector:        connector,
		UnixTool:         *unixTool,
		systemPromptAsk:  &spAsk,
		systemPromptTask: &spTask,
	}
}

// Question sends a question to the agent and returns the response.
// It queries the model using the provided question string and the system prompt.
// If an error occurs during the query, it returns an empty string and an error.
func (a *Agent) Question(ctx context.Context, s string) (string, error) {
	res, err := a.Connector.Query(&s, a.systemPromptAsk)

	if err != nil {
		return "", fmt.Errorf("failed to query model: %w", err)
	}
	return res, nil
}

// Task is about asking and executing.
// The query is sent to the model with relevant tools.
// The model then decides whether and which tools to use.
// Each tool has its own way of executing the task.
func (a *Agent) Task(ctx context.Context, s string) (string, error) {
	sugar := utils.Logger.Sugar()

	// Query the model with the task
	// TODO: There's only a single iterations with tools; either get it or not.
	res, err := a.Connector.QueryWithTool(&s, a.systemPromptTask)
	sugar.Debugw("Query response", "res", res, "err", err)
	return res, err
}
