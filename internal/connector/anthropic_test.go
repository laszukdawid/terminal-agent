package connector

import (
	"context"
	"io"
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

func TestAnthropicMaxTokens(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int64
	}{
		{name: "zero floors to default", in: 0, want: defaultAnthropicMaxTokens},
		{name: "negative floors to default", in: -5, want: defaultAnthropicMaxTokens},
		{name: "positive passes through", in: 600, want: 600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := anthropicMaxTokens(tc.in); got != tc.want {
				t.Errorf("anthropicMaxTokens(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// anthropicMessageResponse is a minimal valid non-streaming message reply so the
// SDK decodes successfully and the test can inspect the request body.
const anthropicMessageResponse = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`

func TestAnthropicQuerySendsFlooredMaxTokens(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicMessageResponse))
	}))
	defer server.Close()

	ac := newTestAnthropicConnector(server.URL)
	sys, user := "sys", "hi"
	if _, err := ac.Query(context.Background(), &QueryParams{SysPrompt: &sys, UserPrompt: &user, MaxTokens: 0}); err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if !strings.Contains(gotBody, `"max_tokens":4096`) {
		t.Fatalf("request body = %s, want max_tokens 4096", gotBody)
	}
}

func TestAnthropicQueryWithToolSendsFlooredMaxTokens(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(anthropicMessageResponse))
	}))
	defer server.Close()

	ac := newTestAnthropicConnector(server.URL)
	sys, user := "sys", "hi"
	if _, err := ac.QueryWithTool(context.Background(), &QueryParams{SysPrompt: &sys, UserPrompt: &user, MaxTokens: 0}, nil); err != nil {
		t.Fatalf("QueryWithTool error: %v", err)
	}
	if !strings.Contains(gotBody, `"max_tokens":4096`) {
		t.Fatalf("request body = %s, want max_tokens 4096", gotBody)
	}
}
