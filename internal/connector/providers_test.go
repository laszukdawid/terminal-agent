package connector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedProvidersIsAlphabeticalAndIncludesMiMo(t *testing.T) {
	got := SupportedProviders()
	want := []string{
		AnthropicProvider, BedrockProvider, GoogleProvider, LlamaProvider,
		MiMoProvider, MistralProvider, OllamaProvider, OpenaiProvider,
	}
	assert.Equal(t, want, got)
}

func TestDefaultModelFor(t *testing.T) {
	assert.Equal(t, string(DefaultOpenAIModel), DefaultModelFor(OpenaiProvider))
	assert.Equal(t, DefaultMiMoModel, DefaultModelFor(MiMoProvider))
	assert.Equal(t, DefaultMistralModel, DefaultModelFor(MistralProvider))
	assert.Equal(t, string(ClaudeHaiku), DefaultModelFor(BedrockProvider))
	assert.Equal(t, "", DefaultModelFor(AnthropicProvider))
	assert.Equal(t, "", DefaultModelFor("nope"))
}

// Drift guard: every registry provider must be handled by the NewConnector
// switch (not fall through to the "unsupported provider" default). Constructors
// may panic or error on missing config/keys -- that still means the provider was
// recognized, so we recover and only fail on the unsupported-provider error.
func TestRegistryProvidersAreHandledByFactory(t *testing.T) {
	for _, name := range SupportedProviders() {
		name := name
		t.Run(name, func(t *testing.T) {
			err := safeNewConnectorErr(name)
			if err != nil {
				assert.NotContains(t, strings.ToLower(err.Error()), "unsupported provider",
					"registry provider %q is not handled by NewConnector switch", name)
			}
		})
	}
}

func safeNewConnectorErr(provider string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = nil // constructor ran => provider recognized; not a drift error
		}
	}()
	_, err = NewConnector(provider, "", nil)
	return err
}
