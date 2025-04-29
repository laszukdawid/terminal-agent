package connector

func NewConnector(provider string, modelID string) *LLMConnector {
	if modelID == "" {
		panic("modelID is empty")
	}

	var connector LLMConnector
	switch provider {
	// case BedrockProvider:
	// 	connector = NewBedrockConnector((*BedrockModelID)(&modelID))
	// case PerplexityProvider:
	// 	ac := NewPerplexityConnector((*PerplexityModelId)(&modelID))
	// 	connector = LLMConnector(ac)
	// case OpenaiProvider:
	// 	connector = NewOpenAIConnector(&modelID)
	case AnthropicProvider:
		connector = NewAnthropicConnector(&modelID)
	case GoogleProvider:
		connector = NewGoogleConnector(&modelID)
	}

	return &connector
}
