package connector

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportedProvidersIsAlphabeticalAndIncludesMiMo(t *testing.T) {
	got := SupportedProviders()

	sorted := slices.Clone(got)
	slices.Sort(sorted)
	assert.Equal(t, sorted, got, "SupportedProviders must be in alphabetical order")

	assert.Len(t, got, len(providerRegistry), "SupportedProviders must list every registry entry")
	assert.Contains(t, got, MiMoProvider)
}

func TestDefaultModelFor(t *testing.T) {
	assert.Equal(t, string(DefaultOpenAIModel), DefaultModelFor(OpenaiProvider))
	assert.Equal(t, DefaultCodexModel, DefaultModelFor(CodexProvider))
	assert.Equal(t, DefaultMiMoModel, DefaultModelFor(MiMoProvider))
	assert.Equal(t, DefaultMistralModel, DefaultModelFor(MistralProvider))
	assert.Equal(t, string(GLM47Flash), DefaultModelFor(BedrockProvider))
	assert.Equal(t, DefaultAnthropicModel, DefaultModelFor(AnthropicProvider))
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
			err := safeNewConnectorErr(t, name)
			if err != nil {
				assert.NotContains(t, strings.ToLower(err.Error()), "unsupported provider",
					"registry provider %q is not handled by NewConnector switch", name)
			}
		})
	}
}

// safeNewConnectorErr calls NewConnector and recovers from any panic (some
// constructors panic on missing config/keys, which still means the provider was
// recognized by the switch). Recovered panics are logged so a genuine
// constructor bug is visible rather than silently swallowed.
func safeNewConnectorErr(t *testing.T, provider string) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Logf("NewConnector(%q) panicked (provider recognized, treated as non-drift): %v", provider, r)
			err = nil
		}
	}()
	_, err = NewConnector(provider, "", nil)
	return err
}
