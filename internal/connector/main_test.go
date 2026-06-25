package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnectorReturnsErrorForUnsupportedProvider(t *testing.T) {
	conn, err := NewConnector("nope", "model", nil)

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.EqualError(t, err, "unsupported provider: nope")
}

func TestNewConnectorUsesProviderDefaultModelWhenModelIDEmpty(t *testing.T) {
	conn, err := NewConnector(OpenaiProvider, "", nil)

	require.NoError(t, err)
	assert.IsType(t, &OpenAIConnector{}, conn)
}

func TestNewConnectorCreatesCodexConnector(t *testing.T) {
	conn, err := NewConnector(CodexProvider, "", nil)

	require.NoError(t, err)
	assert.IsType(t, &CodexConnector{}, conn)
	// An empty model must fall back to the codex default, not the OpenAI default.
	assert.Equal(t, DefaultCodexModel, conn.(*CodexConnector).modelID)
}

func TestNewConnectorCreatesMistralConnector(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	conn, err := NewConnector(MistralProvider, "mistral-large-latest", nil)

	require.NoError(t, err)
	assert.IsType(t, &MistralConnector{}, conn)
}

func TestNewConnectorReturnsMistralInitializationError(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "")

	conn, err := NewConnector(MistralProvider, "mistral-large-latest", nil)

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "failed to initialize mistral connector")
	assert.Contains(t, err.Error(), "MISTRAL_API_KEY")
}

func TestNewConnectorCreatesMiMoConnector(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "test-key")

	conn, err := NewConnector(MiMoProvider, "mimo-v2.5-pro", nil)

	require.NoError(t, err)
	assert.IsType(t, &MiMoConnector{}, conn)
	assert.Equal(t, "mimo-v2.5-pro", conn.(*MiMoConnector).modelID)
}
