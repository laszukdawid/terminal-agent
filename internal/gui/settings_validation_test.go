package gui

import (
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/connector"
	"github.com/stretchr/testify/assert"
)

func TestValidateProviderModelAcceptsMiMo(t *testing.T) {
	assert.NoError(t, validateProviderModel(connector.MiMoProvider, "mimo-v2.5-pro"))
}

func TestProviderSetupHintMiMo(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "")
	assert.Contains(t, providerSetupHint(connector.MiMoProvider), "MIMO_API_KEY")
}

// The Google hint must name the env var the connector actually reads
// (GEMINI_API_KEY), not GOOGLE_API_KEY.
func TestProviderSetupHintGoogle(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	hint := providerSetupHint(connector.GoogleProvider)
	assert.Contains(t, hint, "GEMINI_API_KEY")
	assert.NotContains(t, hint, "GOOGLE_API_KEY")
}

func TestFilterProviders(t *testing.T) {
	all := connector.SupportedProviders()

	// "o" matches every provider whose name contains the letter o.
	assert.ElementsMatch(t,
		[]string{"anthropic", "bedrock", "google", "mimo", "ollama", "openai"},
		filterProviders(all, "o"),
	)
	// Case-insensitive.
	assert.ElementsMatch(t,
		filterProviders(all, "o"), filterProviders(all, "O"),
	)
	// Empty query returns the full list.
	assert.Equal(t, all, filterProviders(all, ""))
	// No match returns empty.
	assert.Empty(t, filterProviders(all, "zzz"))
}
