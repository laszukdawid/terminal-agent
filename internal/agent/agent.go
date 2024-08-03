package agent

import (
	"fmt"

	"github.com/laszukdawid/terminal-agent/internal/connector"
)

type Agent struct {
	Connector    *connector.LLMConnector
	systemPrompt *string
}

func NewAgent(connector *connector.LLMConnector) *Agent {
	sp := SystemPrompt
	return &Agent{
		Connector:    connector,
		systemPrompt: &sp,
	}
}

// Question sends a question to the agent and returns the response.
// It queries the model using the provided question string and the system prompt.
// If an error occurs during the query, it returns an empty string and an error.
func (a *Agent) Question(s string) (string, error) {
	res, err := (*a.Connector).Query(&s, a.systemPrompt)

	if err != nil {
		return "", fmt.Errorf("failed to query model: %w", err)
	}
	return res, nil
}
