package connector

import (
	"github.com/laszukdawid/terminal-agent/internal/tools"
)

func NewConnector(provider string, modelID string) *LLMConnector {
	if modelID == "" {
		panic("modelID is empty")
	}

	unixTool := tools.NewUnixTool(nil)

	// Define the tools that the agent can use
	toolsMap := map[string]tools.Tool{
		unixTool.Name():                 unixTool,
		tools.NewWebsearchTool().Name(): tools.NewWebsearchTool(),
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
	case GoogleProvider:
		connector = NewGoogleConnector(&modelID)
	}

	return &connector
}
