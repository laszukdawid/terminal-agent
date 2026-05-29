package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.uber.org/zap"
)

func TestQueryAnthropicStreamReturnsAccumulateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
	}))
	defer server.Close()

	ac := newTestAnthropicConnector(server.URL)
	_, err := ac.queryAnthropicStreamWithCallback(context.Background(), testAnthropicMessageParams(), func(string) error { return nil })
	if err == nil {
		t.Fatal("expected accumulate error")
	}
	if !strings.Contains(err.Error(), "no content block") {
		t.Fatalf("error = %v, want no content block", err)
	}
}

func TestQueryAnthropicStreamReturnsStreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: not-json\n\n"))
	}))
	defer server.Close()

	ac := newTestAnthropicConnector(server.URL)
	_, err := ac.queryAnthropicStreamWithCallback(context.Background(), testAnthropicMessageParams(), func(string) error { return nil })
	if err == nil {
		t.Fatal("expected stream error")
	}
}

func newTestAnthropicConnector(baseURL string) *AnthropicConnector {
	client := anthropic.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey("test-key"),
	)
	return &AnthropicConnector{
		modelID: "claude-test",
		logger:  *zap.NewNop(),
		client:  client,
	}
}

func testAnthropicMessageParams() anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		MaxTokens: 1,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		},
		Model: "claude-test",
	}
}
