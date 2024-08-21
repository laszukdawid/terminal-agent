package connector

import "github.com/laszukdawid/terminal-agent/internal/tools"

func NewConnector(provider string, modelID string) *LLMConnector {

	// Define the tools that the agent can use
	toolsMap := map[string]tools.Tool{
		"unix": tools.NewUnixTool(nil),
	}

	var connector LLMConnector
	if provider == BedrockProvider {
		connector = NewBedrockConnector((*BedrockModelID)(&modelID), toolsMap)
	} else if provider == PerplexityProvider {
		ac := NewPerplexityConnector((*PerplexityModelId)(&modelID))
		connector = LLMConnector(ac)
	}

	return &connector
}
