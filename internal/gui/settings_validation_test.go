package gui

import (
	"context"
	"errors"
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

func TestProviderSetupHintOpenAIUsesAPIKeyOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	assert.Contains(t, providerSetupHint(connector.OpenaiProvider), "agent auth login openai --api-key")
}

func TestProviderSetupHintCodexUsesOAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	assert.Contains(t, providerSetupHint(connector.CodexProvider), "agent auth login codex")
}

func TestProviderReadinessMiMoMissing(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "")

	status := providerReadinessStatus(connector.MiMoProvider)

	assert.False(t, status.Available)
	assert.Contains(t, status.Message, "Unavailable")
	assert.Contains(t, status.Message, "MIMO_API_KEY")
	assert.Contains(t, status.Message, "GUI process")
}

func TestProviderReadinessMiMoAvailable(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "test-key")

	status := providerReadinessStatus(connector.MiMoProvider)

	assert.True(t, status.Available)
	assert.Contains(t, status.Message, "Available")
	assert.Contains(t, status.Message, "MIMO_API_KEY")
	assert.Contains(t, status.Message, "GUI process")
}

// The Google hint must name the env var the connector actually reads
// (GEMINI_API_KEY), not GOOGLE_API_KEY.
func TestProviderSetupHintGoogle(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	hint := providerSetupHint(connector.GoogleProvider)
	assert.Contains(t, hint, "GEMINI_API_KEY")
	assert.NotContains(t, hint, "GOOGLE_API_KEY")
}

func TestProviderReadinessGoogleUsesGeminiAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")

	status := providerReadinessStatus(connector.GoogleProvider)

	assert.False(t, status.Available)
	assert.Contains(t, status.Message, "GEMINI_API_KEY")
	assert.NotContains(t, status.Message, "GOOGLE_API_KEY")
}

func TestProviderReadinessOllamaAvailable(t *testing.T) {
	status := providerReadinessStatus(connector.OllamaProvider)

	assert.True(t, status.Available)
	assert.Contains(t, status.Message, "does not require an API key")
}

func TestFilterProviders(t *testing.T) {
	all := connector.SupportedProviders()

	// "o" matches every provider whose name contains the letter o.
	assert.ElementsMatch(t,
		[]string{"anthropic", "bedrock", "codex", "google", "mimo", "ollama", "openai"},
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

func TestRuntimeErrorMessageReturnsTrimmedProviderText(t *testing.T) {
	err := errors.New("  provider openai request failed: {\"error\":{\"code\":\"insufficient_quota\"}}  ")

	got := runtimeErrorMessage(err)
	want := "provider openai request failed: {\"error\":{\"code\":\"insufficient_quota\"}}"
	if got != want {
		t.Fatalf("runtimeErrorMessage() = %q, want %q", got, want)
	}
}

func TestRuntimeErrorMessageSuppressesCancellation(t *testing.T) {
	if got := runtimeErrorMessage(context.Canceled); got != "" {
		t.Fatalf("runtimeErrorMessage(context.Canceled) = %q, want empty", got)
	}
}
