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

func validateProviderModel(provider, model string) error {
	if provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}
	supported := connector.SupportedProviders()
	if !slices.Contains(supported, provider) {
		return fmt.Errorf("unsupported provider %q. Supported providers: %s", provider, strings.Join(supported, ", "))
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
			return "OpenAI requires OPENAI_API_KEY or 'agent auth login openai --api-key'."
		}
	case connector.CodexProvider:
		status, err := auth.NewManager().Status(provider)
		if err == nil && status.Configured {
			return ""
		}
		return "Codex requires 'agent auth login codex'."
	case connector.AnthropicProvider:
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			return "Anthropic requires ANTHROPIC_API_KEY to be set."
		}
	case connector.GoogleProvider:
		if os.Getenv("GEMINI_API_KEY") == "" {
			return "Google requires GEMINI_API_KEY to be set."
		}
	case connector.MistralProvider:
		if os.Getenv("MISTRAL_API_KEY") == "" {
			return "Mistral requires MISTRAL_API_KEY to be set."
		}
	case connector.MiMoProvider:
		if os.Getenv("MIMO_API_KEY") == "" {
			return "MiMo requires MIMO_API_KEY to be set."
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

type providerReadiness struct {
	Available bool
	Message   string
}

func providerReadinessStatus(provider string) providerReadiness {
	switch provider {
	case connector.OpenaiProvider:
		if os.Getenv("OPENAI_API_KEY") != "" {
			return providerReadiness{Available: true, Message: "Available: OPENAI_API_KEY is visible to the GUI process."}
		}
		status, err := auth.NewManager().Status(provider)
		if err == nil && status.Configured {
			return providerReadiness{Available: true, Message: "Available: stored OpenAI API key is configured."}
		}
		return providerReadiness{Message: "Unavailable: OPENAI_API_KEY is not visible to the GUI process, and no stored OpenAI API key is configured."}
	case connector.CodexProvider:
		status, err := auth.NewManager().Status(provider)
		if err == nil && status.Configured {
			return providerReadiness{Available: true, Message: "Available: stored Codex OAuth login is configured."}
		}
		return providerReadiness{Message: "Unavailable: no stored Codex OAuth login is configured."}
	case connector.AnthropicProvider:
		return envProviderReadiness("Anthropic", "ANTHROPIC_API_KEY")
	case connector.GoogleProvider:
		return envProviderReadiness("Google", "GEMINI_API_KEY")
	case connector.MistralProvider:
		return envProviderReadiness("Mistral", "MISTRAL_API_KEY")
	case connector.MiMoProvider:
		return envProviderReadiness("MiMo", "MIMO_API_KEY")
	case connector.LlamaProvider:
		if os.Getenv("YZMA_LIB") != "" {
			return providerReadiness{Available: true, Message: "Available: YZMA_LIB is visible to the GUI process."}
		}
		return providerReadiness{Message: "Unavailable: YZMA_LIB is not visible to the GUI process."}
	case connector.OllamaProvider:
		return providerReadiness{Available: true, Message: "Available: Ollama does not require an API key; OLLAMA_HOST is optional."}
	case connector.BedrockProvider:
		if os.Getenv("AWS_REGION") != "" {
			return providerReadiness{Available: true, Message: "Check AWS credentials: AWS_REGION is visible; Bedrock uses the AWS SDK credential chain."}
		}
		return providerReadiness{Available: true, Message: "Check AWS credentials: Bedrock uses the AWS SDK credential chain and defaults AWS_REGION when unset."}
	default:
		if provider == "" {
			return providerReadiness{}
		}
		return providerReadiness{Message: fmt.Sprintf("Unknown provider: %s", provider)}
	}
}

func envProviderReadiness(displayName, envVar string) providerReadiness {
	if os.Getenv(envVar) != "" {
		return providerReadiness{Available: true, Message: fmt.Sprintf("Available: %s is visible to the GUI process.", envVar)}
	}
	return providerReadiness{Message: fmt.Sprintf("Unavailable: %s is not visible to the GUI process. %s requires it.", envVar, displayName)}
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
		return fmt.Sprintf("%s Supported providers: %s.", message, strings.Join(connector.SupportedProviders(), ", "))
	case strings.Contains(lower, "api key"), strings.Contains(lower, "authenticate"), strings.Contains(lower, "authentication"):
		if hint != "" {
			return hint
		}
	case strings.Contains(lower, "oauth login has expired"), strings.Contains(lower, "agent auth login codex' again"):
		if provider == connector.CodexProvider {
			return "Stored Codex login has expired. Run 'agent auth login codex' again."
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

// filterProviders returns the providers whose name contains query
// (case-insensitive substring). An empty query returns all providers unchanged.
func filterProviders(all []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return all
	}
	out := make([]string, 0, len(all))
	for _, p := range all {
		if strings.Contains(strings.ToLower(p), query) {
			out = append(out, p)
		}
	}
	return out
}
