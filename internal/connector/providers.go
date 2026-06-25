package connector

// ProviderInfo is the single source of truth for a supported LLM provider.
// DefaultModel is the model used when the user does not specify one; it is empty
// for providers that require an explicit model (e.g. anthropic).
type ProviderInfo struct {
	Name         string
	DefaultModel string
}

// providerRegistry lists every provider the application supports. Keep it
// alphabetical by Name. Adding a provider also requires wiring its constructor
// into NewConnector (main.go); TestRegistryProvidersAreHandledByFactory guards
// against the two drifting apart.
var providerRegistry = []ProviderInfo{
	{Name: AnthropicProvider, DefaultModel: DefaultAnthropicModel},
	// BedrockModelID and ChatModel are named string types, hence the conversions.
	{Name: BedrockProvider, DefaultModel: string(GLM47Flash)},
	{Name: CodexProvider, DefaultModel: DefaultCodexModel},
	{Name: GoogleProvider, DefaultModel: Gemini31FlashLite},
	{Name: LlamaProvider, DefaultModel: DefaultLlamaModel},
	{Name: MiMoProvider, DefaultModel: DefaultMiMoModel},
	{Name: MistralProvider, DefaultModel: DefaultMistralModel},
	{Name: OllamaProvider, DefaultModel: DefaultOllamaModel},
	{Name: OpenaiProvider, DefaultModel: string(DefaultOpenAIModel)},
}

// SupportedProviders returns the provider names in stable alphabetical order.
func SupportedProviders() []string {
	names := make([]string, len(providerRegistry))
	for i, p := range providerRegistry {
		names[i] = p.Name
	}
	return names
}

// DefaultModelFor returns the default model for a provider, or "" when the
// provider has no fixed default or is unknown.
func DefaultModelFor(provider string) string {
	for _, p := range providerRegistry {
		if p.Name == provider {
			return p.DefaultModel
		}
	}
	return ""
}

// Providers returns a copy of the full registry.
func Providers() []ProviderInfo {
	out := make([]ProviderInfo, len(providerRegistry))
	copy(out, providerRegistry)
	return out
}
