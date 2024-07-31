package connector

func NewConnector(provider string, modelID string) *LLMConnector {

	var connector LLMConnector
	if provider == BedrockProvider {
		connector = NewBedrockConnector((*BedrockModelID)(&modelID))
	} else if provider == PerplexityProvider {
		ac := NewPerplexityConnector((*PerplexityModelId)(&modelID))
		connector = LLMConnector(ac)
	}

	return &connector
}
