package gui

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/connector"
)

var supportedProviders = []string{
	connector.AnthropicProvider,
	connector.BedrockProvider,
	connector.GoogleProvider,
	connector.LlamaProvider,
	connector.OllamaProvider,
	connector.OpenaiProvider,
}

func validateProviderModel(provider, model string) error {
	if provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	if !slices.Contains(supportedProviders, provider) {
		return fmt.Errorf("unsupported provider %q. Supported providers: %s", provider, strings.Join(supportedProviders, ", "))
	}
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	return nil
}

func providerSetupHint(provider string) string {
	switch provider {
	case connector.OpenaiProvider:
		status, err := auth.NewManager().Status(provider)
		if err == nil && status.Configured {
			return ""
		}
		if os.Getenv("OPENAI_API_KEY") == "" {
			return "OpenAI requires OPENAI_API_KEY or 'agent auth login openai'."
		}
	case connector.AnthropicProvider:
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return "Anthropic requires ANTHROPIC_API_KEY to be set."
		}
	case connector.GoogleProvider:
		if os.Getenv("GOOGLE_API_KEY") == "" {
			return "Google requires GOOGLE_API_KEY to be set."
		}
	case connector.LlamaProvider:
		return "Llama uses local GGUF model aliases from config.json under llama_models."
	case connector.OllamaProvider:
		return "Ollama uses the local server configuration from OLLAMA_HOST when set."
	case connector.BedrockProvider:
		if os.Getenv("AWS_REGION") == "" {
			return "Bedrock uses AWS credentials and defaults AWS_REGION when unset."
		}
	}
	return ""
}

func explainRuntimeError(provider string, err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(strings.ToLower(err.Error()), "context canceled") || strings.Contains(strings.ToLower(err.Error()), "context cancelled") || err == context.Canceled {
		return ""
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return ""
	}

	hint := providerSetupHint(provider)
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "unsupported provider"):
		return fmt.Sprintf("%s Supported providers: %s.", message, strings.Join(supportedProviders, ", "))
	case strings.Contains(lower, "api key"), strings.Contains(lower, "authenticate"), strings.Contains(lower, "authentication"):
		if hint != "" {
			return hint
		}
	case strings.Contains(lower, "oauth login has expired"), strings.Contains(lower, "agent auth login openai' again"):
		if provider == connector.OpenaiProvider {
			return "Stored OpenAI login has expired. Run 'agent auth login openai' again or set OPENAI_API_KEY."
		}
	case strings.Contains(lower, "status code: 401"), strings.Contains(lower, "status code 401"):
		if hint != "" {
			return hint
		}
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "connection refused"):
		if provider == connector.OllamaProvider {
			return "Could not reach Ollama. Start Ollama locally or set OLLAMA_HOST to the correct server URL."
		}
	}

	return message
}
