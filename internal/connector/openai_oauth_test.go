package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/laszukdawid/terminal-agent/internal/auth"
	"github.com/laszukdawid/terminal-agent/internal/tools"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"go.uber.org/zap"
)

type stubTool struct{}

func (stubTool) RunSchema(input map[string]any) (string, error) { return "", nil }
func (stubTool) Run(input *string) (string, error)              { return "", nil }
func (stubTool) Name() string                                   { return "search_code" }
func (stubTool) Description() string                            { return "Search code" }
func (stubTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
}
func (stubTool) HelpText() string { return "" }

func TestBuildResponsesParamsIncludesHistoryAndPrompt(t *testing.T) {
	sysPrompt := "You are helpful"
	userPrompt := "latest question"
	params := buildResponsesParams("gpt-4o-mini", &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
		Messages: []Message{
			{Role: "user", Content: "old user"},
			{Role: "assistant", Content: "old answer"},
		},
		MaxTokens: 321,
	})

	if params.Instructions.Value != sysPrompt {
		t.Fatalf("instructions = %q, want %q", params.Instructions.Value, sysPrompt)
	}
	if params.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want %q", params.Model, "gpt-4o-mini")
	}
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal(params) error = %v", err)
	}
	if !bytes.Contains(body, []byte(`"store":false`)) {
		t.Fatalf("marshaled params = %s, want store=false", string(body))
	}
	if bytes.Contains(body, []byte(`"max_output_tokens":`)) {
		t.Fatalf("marshaled params = %s, want max_output_tokens omitted", string(body))
	}
	if len(params.Input.OfInputItemList) != 3 {
		t.Fatalf("input items = %d, want 3", len(params.Input.OfInputItemList))
	}
	if params.Input.OfInputItemList[0].OfMessage == nil || params.Input.OfInputItemList[0].OfMessage.Role != responses.EasyInputMessageRoleUser {
		t.Fatal("expected first input item to be a user message")
	}
	if params.Input.OfInputItemList[1].OfMessage == nil || params.Input.OfInputItemList[1].OfMessage.Role != responses.EasyInputMessageRoleAssistant {
		t.Fatal("expected second input item to be an assistant message")
	}
}

func TestConvertToolsToResponses(t *testing.T) {
	toolSpecs := convertToolsToResponses(map[string]tools.Tool{
		"search_code": stubTool{},
	})

	if len(toolSpecs) != 1 {
		t.Fatalf("tool specs = %d, want 1", len(toolSpecs))
	}
	if toolSpecs[0].OfFunction == nil {
		t.Fatal("expected function tool spec")
	}
	if toolSpecs[0].OfFunction.Name != "search_code" {
		t.Fatalf("tool name = %q, want %q", toolSpecs[0].OfFunction.Name, "search_code")
	}
}

func TestStreamOAuthResponseUsesCodexRequestContract(t *testing.T) {
	var capturedBody []byte
	var capturedHeader http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("io.ReadAll(r.Body) error = %v", err)
		}
		capturedHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"forced"}`))
	}))
	defer server.Close()

	client := openai.NewClient(
		option.WithAPIKey("token"),
		option.WithBaseURL(server.URL),
		option.WithHeader("ChatGPT-Account-ID", "account-1"),
		option.WithHeader("originator", auth.OpenAIOriginator()),
		option.WithHeader("OpenAI-Beta", auth.OpenAIResponsesBetaHeader),
	)

	oc := &OpenAIConnector{
		client:  &client,
		logger:  *zap.NewNop(),
		modelID: "gpt-5.4",
	}

	sysPrompt := "You are helpful"
	userPrompt := "what model is this"
	params := buildResponsesParams("gpt-5.4", &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	_, _, err := oc.streamOAuthResponse(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	}, params)
	if err == nil {
		t.Fatal("expected error from forced 400 response")
	}
	if !bytes.Contains(capturedBody, []byte(`"stream":true`)) {
		t.Fatalf("request body = %s, want stream=true", string(capturedBody))
	}
	if !bytes.Contains(capturedBody, []byte(`"store":false`)) {
		t.Fatalf("request body = %s, want store=false", string(capturedBody))
	}
	if got := capturedHeader.Get("ChatGPT-Account-ID"); got != "account-1" {
		t.Fatalf("ChatGPT-Account-ID = %q, want %q", got, "account-1")
	}
}
