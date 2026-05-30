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

func TestStreamOAuthResponseFallsBackToStreamedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`event: response.created
data: {"type":"response.created","response":{"id":"resp_1","object":"response","created_at":1,"status":"in_progress","instructions":"You are helpful","model":"gpt-5.4","output":[],"parallel_tool_calls":true,"store":false,"tools":[],"usage":null},"sequence_number":0}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"msg_1","type":"message","status":"in_progress","content":[],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":1}

event: response.content_part.added
data: {"type":"response.content_part.added","content_index":0,"item_id":"msg_1","output_index":0,"part":{"type":"output_text","annotations":[],"logprobs":[],"text":""},"sequence_number":2}

event: response.output_text.delta
data: {"type":"response.output_text.delta","content_index":0,"delta":"Hello","item_id":"msg_1","logprobs":[],"obfuscation":"abc","output_index":0,"sequence_number":3}

event: response.output_text.delta
data: {"type":"response.output_text.delta","content_index":0,"delta":"!","item_id":"msg_1","logprobs":[],"obfuscation":"def","output_index":0,"sequence_number":4}

event: response.output_text.done
data: {"type":"response.output_text.done","content_index":0,"item_id":"msg_1","logprobs":[],"output_index":0,"sequence_number":5,"text":"Hello!"}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"msg_1","type":"message","status":"completed","content":[{"type":"output_text","annotations":[],"logprobs":[],"text":"Hello!"}],"phase":"final_answer","role":"assistant"},"output_index":0,"sequence_number":6}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1,"status":"completed","instructions":"You are helpful","model":"gpt-5.4","output":[],"parallel_tool_calls":true,"store":false,"tools":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}},"sequence_number":7}

`))
	}))
	defer server.Close()

	client := openai.NewClient(
		option.WithAPIKey("token"),
		option.WithBaseURL(server.URL),
	)
	oc := &OpenAIConnector{
		client:  &client,
		logger:  *zap.NewNop(),
		modelID: "gpt-5.4",
	}

	sysPrompt := "You are helpful"
	userPrompt := "hello"
	params := buildResponsesParams("gpt-5.4", &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	})

	_, text, err := oc.streamOAuthResponse(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	}, params)
	if err != nil {
		t.Fatalf("streamOAuthResponse() error = %v", err)
	}
	if text == nil || *text != "Hello!" {
		t.Fatalf("streamOAuthResponse() text = %v, want Hello!", text)
	}
}

func TestQueryWithToolOAuthCapturesStreamedFunctionCall(t *testing.T) {
	// Codex delivers function calls via output_item.done with an empty completed output array.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`event: response.created
data: {"type":"response.created","response":{"id":"resp_1","object":"response","created_at":1,"status":"in_progress","instructions":"You are helpful","model":"gpt-5.5","output":[],"parallel_tool_calls":true,"store":false,"tools":[],"usage":null},"sequence_number":0}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","status":"in_progress","arguments":"","call_id":"call_1","name":"search_code"},"output_index":0,"sequence_number":1}

event: response.function_call_arguments.delta
data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"query\":\"hi\"}","sequence_number":2}

event: response.function_call_arguments.done
data: {"type":"response.function_call_arguments.done","item_id":"fc_1","output_index":0,"arguments":"{\"query\":\"hi\"}","sequence_number":3}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","status":"completed","arguments":"{\"query\":\"hi\"}","call_id":"call_1","name":"search_code"},"output_index":0,"sequence_number":4}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_1","object":"response","created_at":1,"status":"completed","instructions":"You are helpful","model":"gpt-5.5","output":[],"parallel_tool_calls":true,"store":false,"tools":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}},"sequence_number":5}

`))
	}))
	defer server.Close()

	client := openai.NewClient(
		option.WithAPIKey("token"),
		option.WithBaseURL(server.URL),
	)
	oc := &OpenAIConnector{
		client:  &client,
		logger:  *zap.NewNop(),
		modelID: "gpt-5.5",
		auth:    auth.ResolvedAuth{Type: auth.CredentialTypeOAuth},
	}

	sysPrompt := "You are helpful"
	userPrompt := "find hi"
	resp, err := oc.queryWithToolOAuth(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	}, map[string]tools.Tool{"search_code": stubTool{}})
	if err != nil {
		t.Fatalf("queryWithToolOAuth() error = %v", err)
	}
	if !resp.ToolUse {
		t.Fatal("expected ToolUse=true for streamed function call")
	}
	if resp.ToolName != "search_code" {
		t.Fatalf("ToolName = %q, want %q", resp.ToolName, "search_code")
	}
	if got, _ := resp.ToolInput["query"].(string); got != "hi" {
		t.Fatalf("ToolInput[query] = %q, want %q", got, "hi")
	}
}

func TestOpenAIQueryWithToolReturnsInvalidArgumentError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-1",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o-mini",
			"choices":[{
				"index":0,
				"message":{
					"role":"assistant",
					"content":"",
					"tool_calls":[{
						"id":"call_1",
						"type":"function",
						"function":{"name":"search_code","arguments":"not-json"}
					}]
				},
				"finish_reason":"tool_calls"
			}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	defer server.Close()

	client := openai.NewClient(
		option.WithAPIKey("token"),
		option.WithBaseURL(server.URL),
	)
	oc := &OpenAIConnector{
		client:  &client,
		logger:  *zap.NewNop(),
		modelID: "gpt-4o-mini",
		auth:    auth.ResolvedAuth{Type: auth.CredentialTypeAPIKey},
	}
	sysPrompt := "You are helpful"
	userPrompt := "search"

	_, err := oc.QueryWithTool(context.Background(), &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	}, map[string]tools.Tool{"search_code": stubTool{}})
	if err == nil {
		t.Fatal("expected invalid argument error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("failed to parse tool call arguments")) {
		t.Fatalf("error = %v, want tool argument parse failure", err)
	}
}
