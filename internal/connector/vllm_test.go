package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVLLMQueryUsesConfiguredBaseURLWithoutAPIKey(t *testing.T) {
	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"meta-llama/Llama-3.1-8B-Instruct",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	t.Setenv("VLLM_BASE_URL", server.URL)
	t.Setenv("VLLM_API_KEY", "")
	connector := NewVLLMConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	result, err := connector.Query(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, "/chat/completions", gotPath)
	assert.Empty(t, gotAuth)
}

func TestVLLMQueryUsesAPIKeyWhenConfigured(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"meta-llama/Llama-3.1-8B-Instruct",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	t.Setenv("VLLM_BASE_URL", server.URL)
	t.Setenv("VLLM_API_KEY", "test-key")
	connector := NewVLLMConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	result, err := connector.Query(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, "Bearer test-key", gotAuth)
}
