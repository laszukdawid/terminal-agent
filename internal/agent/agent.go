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

	unixTool := tools.NewUnixTool(connector, nil)

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

func (a *Agent) Task(ctx context.Context, s string) (string, error) {
	sugar := utils.Logger.Sugar()

	// First ask the model for the task
	res, err := a.Connector.Query(&s, a.systemPromptTask)
	sugar.Debugw("First query response", "res", res, "err", err)

	if err != nil {
		return "", fmt.Errorf("failed to query model: %w", err)
	}

	// Check whether the provided instruction is the solution
	toolQuery, err := utils.FindToolQuery(&res)
	sugar.Debugw("Result of FindToolQuery", "codeObject", toolQuery, "err", err)

	// If no error, then the toolQuery is found.
	// Check also if the toolQuery is solved.
	if err == nil && toolQuery.Solved {
		// If solved then execute the instruction as a code
		return a.UnixTool.ExecCode(toolQuery.Instruction)
	}

	// If no code found then we check whether tool needs to be used
	sugar.Infoln("No code found, checking for tool")
	toolRes, err := a.UnixTool.Run(&res)
	sugar.Debugw("Tool response", "toolRes", toolRes, "err", err)

	if err != nil {
		return "", fmt.Errorf("failed to run tool: %w", err)
	}

	toolQuery, err = utils.FindToolQuery(&toolRes)
	sugar.Infow("Code object from FindCodeObject", "toolQuery", toolQuery)
	if err == nil {
		return a.UnixTool.ExecCode(toolQuery.Instruction)
	}

	sugar.Debugw("Returning anything", "codeObject", toolQuery)
	return toolQuery.Instruction, nil
}
