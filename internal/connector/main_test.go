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

func TestNewConnectorCreatesMistralConnector(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	conn, err := NewConnector(MistralProvider, "mistral-large-latest", nil)

	require.NoError(t, err)
	assert.IsType(t, &MistralConnector{}, conn)
}
