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

func (a *Agent) Question(s string) error {
	res, err := (*a.Connector).Query(&s, a.systemPrompt)

	if err != nil {
		return err
	}
	fmt.Println(res)
	return nil
}
