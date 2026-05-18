package connector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnectorReturnsErrorForUnsupportedProvider(t *testing.T) {
	conn, err := NewConnector("nope", "model")

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.EqualError(t, err, "unsupported provider: nope")
}

func TestNewConnectorUsesProviderDefaultModelWhenModelIDEmpty(t *testing.T) {
	conn, err := NewConnector(OpenaiProvider, "")

	require.NoError(t, err)
	assert.IsType(t, &OpenAIConnector{}, conn)
}
