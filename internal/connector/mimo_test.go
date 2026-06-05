package connector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMiMoRetriesRateLimitedRequests(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"429","message":"Too many requests","type":"limitation"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"mimo-v2.5-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	connector := NewMiMoConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	result, err := connector.Query(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestMiMoHonorsRetryAfterMs(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			w.Header().Set("Retry-After-Ms", "25")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"code":"429","message":"Too many requests","type":"limitation"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"created":0,
			"model":"mimo-v2.5-pro",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	connector := NewMiMoConnector(nil)
	sysPrompt := "You are helpful."
	userPrompt := "hello"

	start := time.Now()
	result, err := connector.Query(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
	assert.GreaterOrEqual(t, time.Since(start), 25*time.Millisecond)
}

func TestMiMo429DiagnosticsPreserveResponseBody(t *testing.T) {
	logger := zap.NewNop()
	middleware := newMiMoDiagnosticsMiddleware(logger)
	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/chat/completions", nil)
	req.Header.Set("X-Stainless-Retry-Count", "1")
	body := `{"code":"429","message":"Too many requests","type":"limitation"}`

	resp, err := middleware(req, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{
				"Retry-After": []string{"0"},
			},
			Body: io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	require.NoError(t, err)
	restoredBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(restoredBody))
}

func TestMiMoDiagnosticsHelpers(t *testing.T) {
	header := http.Header{
		"X-Mimo-Request-Id": []string{"mimo-request-1"},
	}

	assert.Equal(t, "mimo-request-1", firstHeader(header, "X-Request-ID", "X-Mimo-Request-ID"))
	assert.Equal(t, "", firstHeader(header, "X-Request-ID"))
	assert.Equal(t, "short", truncateMiMoLogValue("short", 10))
	assert.Equal(t, "01234... [truncated]", truncateMiMoLogValue("0123456789", 5))

	var nested miMoErrorResponse
	require.NoError(t, json.Unmarshal([]byte(`{"error":{"code":"429","message":"Too many requests","type":"limitation"}}`), &nested))
	code, message, errorType := nested.fields()
	assert.Equal(t, "429", code)
	assert.Equal(t, "Too many requests", message)
	assert.Equal(t, "limitation", errorType)
}
