package connector

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/config"
)

func NewConnector(provider string, modelID string, cfg config.Config) (LLMConnector, error) {
	var connector LLMConnector
	var err error
	switch provider {
	case BedrockProvider:
		connector, err = NewBedrockConnector((*BedrockModelID)(&modelID), cfg)
	case CodexProvider:
		connector = NewCodexConnector(&modelID)
	case OpenaiProvider:
		connector = NewOpenAIConnector(&modelID)
	case MiMoProvider:
		connector = NewMiMoConnector(&modelID)
	case AnthropicProvider:
		connector = NewAnthropicConnector(&modelID)
	case GoogleProvider:
		connector = NewGoogleConnector(&modelID)
	case LlamaProvider:
		connector = NewLlamaConnector(&modelID, cfg)
	case OllamaProvider:
		connector = NewOllamaConnector(&modelID)
	case MistralProvider:
		connector, err = NewMistralConnector(&modelID)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize %s connector: %w", provider, err)
	}

	if isNilConnector(connector) {
		return nil, fmt.Errorf("failed to initialize %s connector", provider)
	}

	return connector, nil
}

func RefreshProviderModelMetadataIfNeeded(ctx context.Context, provider string, modelID string, cfg config.Config, now time.Time, errWriter io.Writer) {
	switch provider {
	case BedrockProvider:
		refreshBedrockModelPriceIfNeeded(ctx, cfg, BedrockModelID(modelID), now, errWriter)
	}
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
