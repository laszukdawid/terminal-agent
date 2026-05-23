package connector

import (
	"fmt"
	"reflect"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

func NewConnector(provider string, modelID string, cfg config.Config) (LLMConnector, error) {
	var connector LLMConnector
	switch provider {
	case BedrockProvider:
		connector = NewBedrockConnector((*BedrockModelID)(&modelID))
	case OpenaiProvider:
		connector = NewOpenAIConnector(&modelID)
	case AnthropicProvider:
		connector = NewAnthropicConnector(&modelID)
	case GoogleProvider:
		connector = NewGoogleConnector(&modelID)
	case LlamaProvider:
		connector = NewLlamaConnector(&modelID, cfg)
	case OllamaProvider:
		connector = NewOllamaConnector(&modelID)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	if isNilConnector(connector) {
		return nil, fmt.Errorf("failed to initialize %s connector", provider)
	}

	return connector, nil
}

func isNilConnector(connector LLMConnector) bool {
	if connector == nil {
		return true
	}

	value := reflect.ValueOf(connector)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return true
	}

	return false
}
