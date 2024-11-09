package connector

import (
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

func NewConnector(provider string, modelID string) *LLMConnector {

	// Define the tools that the agent can use
	toolsMap := map[string]tools.Tool{
		"unix": tools.NewUnixTool(nil),
	}

	var connector LLMConnector
	switch provider {
	case BedrockProvider:
		connector = NewBedrockConnector((*BedrockModelID)(&modelID), toolsMap)
	case PerplexityProvider:
		ac := NewPerplexityConnector((*PerplexityModelId)(&modelID))
		connector = LLMConnector(ac)
	case OpenaiProvider:
		connector = NewOpenAIConnector(&modelID, toolsMap)
	case AnthropicProvider:
		connector = NewAnthropicConnector(&modelID, toolsMap)
	}

	return &connector
}
