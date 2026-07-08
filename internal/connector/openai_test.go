package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/require"
)

// emptyChoicesServer returns a valid 200 chat.completion with an empty choices
// array, mimicking a malformed/filtered provider response.
func emptyChoicesServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"gpt-4o-mini",
			"choices":[],
			"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}
		}`))
	}))
}

// TestOpenAIQueryReturnsErrorOnEmptyChoices guards BUG-9: an empty choices array
// must produce ErrNoProviderResponse rather than an index-out-of-range panic.
func TestOpenAIQueryReturnsErrorOnEmptyChoices(t *testing.T) {
	server := emptyChoicesServer(t)
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	conn := NewOpenAIConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	_, err := conn.Query(context.Background(), &QueryParams{SysPrompt: &sysPrompt, UserPrompt: &userPrompt})
	require.ErrorIs(t, err, ErrNoProviderResponse)
}

func TestOpenAIQueryWithToolReturnsErrorOnEmptyChoices(t *testing.T) {
	server := emptyChoicesServer(t)
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)

	conn := NewOpenAIConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	_, err := conn.QueryWithTool(context.Background(), &QueryParams{SysPrompt: &sysPrompt, UserPrompt: &userPrompt}, map[string]tools.Tool{})
	require.ErrorIs(t, err, ErrNoProviderResponse)
}
