package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/stretchr/testify/assert"
)

func TestNewMistralConnector(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	modelID := "mistral-large-latest"

	connector := NewMistralConnector(&modelID)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.modelID != modelID {
		t.Errorf("Expected modelID to be %s, got %s", modelID, connector.modelID)
	}

	if connector.apiKey != "test-key" {
		t.Errorf("Expected apiKey to be test-key, got %s", connector.apiKey)
	}

	if connector.baseURL != DefaultMistralBaseURL {
		t.Errorf("Expected baseURL to be %s, got %s", DefaultMistralBaseURL, connector.baseURL)
	}
}

func TestNewMistralConnectorNoKey(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "")
	modelID := "mistral-large-latest"

	connector := NewMistralConnector(&modelID)

	assert.Nil(t, connector, "Expected connector to be nil when MISTRAL_API_KEY is not set")
}

func TestNewMistralConnectorNoModelID(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	var modelID *string

	connector := NewMistralConnector(modelID)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.modelID != DefaultMistralModel {
		t.Errorf("Expected modelID to be %s, got %s", DefaultMistralModel, connector.modelID)
	}
}

func TestNewMistralConnectorCustomBaseURL(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", "https://mistral.example.com")
	modelID := "mistral-large-latest"

	connector := NewMistralConnector(&modelID)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.baseURL != "https://mistral.example.com" {
		t.Errorf("Expected baseURL to be https://mistral.example.com, got %s", connector.baseURL)
	}
}

func TestNewMistralConnectorCustomBaseURLTrailingSlash(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", "https://mistral.example.com/")
	modelID := "mistral-large-latest"

	connector := NewMistralConnector(&modelID)

	if connector == nil {
		t.Fatal("Expected connector to be created, got nil")
	}

	if connector.baseURL != "https://mistral.example.com" {
		t.Errorf("Expected baseURL to have trailing slash trimmed, got %s", connector.baseURL)
	}
}

func TestConvertToolsToMistral(t *testing.T) {
	execTools := map[string]tools.Tool{
		"unix": tools.NewUnixTool(nil),
	}

	toolSpecs := convertToolsToMistral(execTools)

	if len(toolSpecs) != 1 {
		t.Fatalf("Expected 1 tool spec, got %d", len(toolSpecs))
	}

	toolSpec := toolSpecs[0]
	expectedTool := execTools["unix"]

	assert.Equal(t, "function", toolSpec.Type, "Expected tool type to be function")
	assert.Equal(t, expectedTool.Name(), toolSpec.Function.Name, "Expected tool name to be %s, got %s", expectedTool.Name(), toolSpec.Function.Name)
	assert.Equal(t, expectedTool.Description(), toolSpec.Function.Description, "Expected tool description to be %s, got %s", expectedTool.Description(), toolSpec.Function.Description)
	assert.NotNil(t, toolSpec.Function.Parameters, "Expected parameters to be non-nil")
}

func TestMistralBuildMessages(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	sysPrompt := "You are helpful."
	userPrompt := "Hello"

	params := &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
		Messages: []Message{
			{Role: "user", Content: "Previous message"},
			{Role: "assistant", Content: "Previous response"},
		},
	}

	messages := mc.buildMessages(params)

	assert.Equal(t, 4, len(messages))
	assert.Equal(t, "system", messages[0].Role)
	assert.Equal(t, sysPrompt, messages[0].Content)
	assert.Equal(t, "user", messages[1].Role)
	assert.Equal(t, "Previous message", messages[1].Content)
	assert.Equal(t, "assistant", messages[2].Role)
	assert.Equal(t, "Previous response", messages[2].Content)
	assert.Equal(t, "user", messages[3].Role)
	assert.Equal(t, userPrompt, messages[3].Content)
}

func TestMistralBuildMessagesEmptySystem(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	sysPrompt := ""
	userPrompt := "Hello"

	params := &QueryParams{
		SysPrompt:  &sysPrompt,
		UserPrompt: &userPrompt,
	}

	messages := mc.buildMessages(params)

	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, userPrompt, messages[0].Content)
}

func TestMistralSSEParse(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	t.Run("normal streaming response", func(t *testing.T) {
		sseBody := `data: {"id":"1","choices":[{"delta":{"content":"Hello"}}]}

data: {"id":"2","choices":[{"delta":{"content":" world"}}]}

data: {"id":"3","choices":[{"delta":{"content":"!"}}]}

data: [DONE]
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(sseBody))
		}))
		defer server.Close()

		// Override baseURL temporarily - we need to test the streaming handler directly
		// by constructing a request to the test server
		req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", nil)
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer test-key")

		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		var chunks []string
		onStream := func(chunk string) error {
			chunks = append(chunks, chunk)
			return nil
		}

		result, err := mc.handleStreamingResponse(resp, onStream)
		assert.NoError(t, err)
		assert.Equal(t, "Hello world!", result)
		assert.Equal(t, []string{"Hello", " world", "!"}, chunks)
	})

	t.Run("stream with empty lines and other SSE fields", func(t *testing.T) {
		sseBody := `event: message
data: {"id":"1","choices":[{"delta":{"content":"A"}}]}

id: 1
data: {"id":"2","choices":[{"delta":{"content":"B"}}]}

data: [DONE]
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(sseBody))
		}))
		defer server.Close()

		req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", nil)
		assert.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		result, err := mc.handleStreamingResponse(resp, nil)
		assert.NoError(t, err)
		assert.Equal(t, "AB", result)
	})

	t.Run("stream with no DONE signal", func(t *testing.T) {
		sseBody := `data: {"id":"1","choices":[{"delta":{"content":"partial"}}]}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			w.Write([]byte(sseBody))
		}))
		defer server.Close()

		req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", nil)
		assert.NoError(t, err)

		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		result, err := mc.handleStreamingResponse(resp, nil)
		assert.NoError(t, err)
		assert.Equal(t, "partial", result)
	})
}

func TestMistralToolCallResponseParse(t *testing.T) {
	responseJSON := `{
		"id": "cmpl-123",
		"model": "mistral-large-latest",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Let me check that for you.",
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {
						"name": "unix",
						"arguments": "{\"command\": \"ls -la\", \"final\": false}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}]
	}`

	var mistralResp MistralResponse
	err := json.Unmarshal([]byte(responseJSON), &mistralResp)
	assert.NoError(t, err)

	assert.Equal(t, "cmpl-123", mistralResp.ID)
	assert.Equal(t, 1, len(mistralResp.Choices))

	choice := mistralResp.Choices[0]
	assert.Equal(t, "Let me check that for you.", choice.Message.Content)
	assert.Equal(t, 1, len(choice.Message.ToolCalls))

	toolCall := choice.Message.ToolCalls[0]
	assert.Equal(t, "call_abc", toolCall.ID)
	assert.Equal(t, "function", toolCall.Type)
	assert.Equal(t, "unix", toolCall.Function.Name)
	assert.Equal(t, `{"command": "ls -la", "final": false}`, toolCall.Function.Arguments)

	var args map[string]any
	err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
	assert.NoError(t, err)
	assert.Equal(t, "ls -la", args["command"])
	assert.Equal(t, false, args["final"])
}

func TestMistralToolCallResponseEmptyToolCalls(t *testing.T) {
	responseJSON := `{
		"id": "cmpl-123",
		"model": "mistral-large-latest",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello, how can I help?"
			},
			"finish_reason": "stop"
		}]
	}`

	var mistralResp MistralResponse
	err := json.Unmarshal([]byte(responseJSON), &mistralResp)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(mistralResp.Choices))
	assert.Equal(t, "Hello, how can I help?", mistralResp.Choices[0].Message.Content)
	assert.Nil(t, mistralResp.Choices[0].Message.ToolCalls)
}

func TestMistralErrorResponseParse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"object":"error","message":"Invalid API key","type":"auth_error","code":"invalid_api_key"}`))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "bad-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	mc.baseURL = server.URL

	req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", strings.NewReader("{}"))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer bad-key")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	parseErr := mc.parseErrorResponse(resp)
	assert.Error(t, parseErr)
	assert.Contains(t, parseErr.Error(), "Invalid API key")
}

func TestMistralErrorResponseNonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	mc.baseURL = server.URL

	req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", strings.NewReader("{}"))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	parseErr := mc.parseErrorResponse(resp)
	assert.Error(t, parseErr)
	assert.Contains(t, parseErr.Error(), "status 500")
}

func TestMistralQueryRetriesRateLimitThenSucceeds(t *testing.T) {
	previousRetries := mistralMaxRateLimitRetries
	previousBase := mistralRateLimitBackoffBase
	previousMax := mistralRateLimitBackoffMax
	mistralMaxRateLimitRetries = 2
	mistralRateLimitBackoffBase = time.Millisecond
	mistralRateLimitBackoffMax = 2 * time.Millisecond
	defer func() {
		mistralMaxRateLimitRetries = previousRetries
		mistralRateLimitBackoffBase = previousBase
		mistralRateLimitBackoffMax = previousMax
	}()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := requests.Add(1)
		if attempt == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"object":"error","message":"Rate limit exceeded","type":"rate_limit","code":"rate_limit_exceeded"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","model":"test","choices":[{"message":{"content":"formatted"}}]}`))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	userPrompt := "format this"
	result, err := mc.Query(t.Context(), &QueryParams{UserPrompt: &userPrompt})
	assert.NoError(t, err)
	assert.Equal(t, "formatted", result)
	assert.EqualValues(t, 2, requests.Load())
}

func TestMistralQueryReturnsRateLimitAfterRetriesExhausted(t *testing.T) {
	previousRetries := mistralMaxRateLimitRetries
	previousBase := mistralRateLimitBackoffBase
	previousMax := mistralRateLimitBackoffMax
	mistralMaxRateLimitRetries = 2
	mistralRateLimitBackoffBase = time.Millisecond
	mistralRateLimitBackoffMax = 2 * time.Millisecond
	defer func() {
		mistralMaxRateLimitRetries = previousRetries
		mistralRateLimitBackoffBase = previousBase
		mistralRateLimitBackoffMax = previousMax
	}()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"object":"error","message":"Rate limit exceeded","type":"rate_limit","code":"rate_limit_exceeded"}`))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	userPrompt := "format this"
	_, err := mc.Query(t.Context(), &QueryParams{UserPrompt: &userPrompt})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Rate limit exceeded")
	assert.EqualValues(t, 3, requests.Load())
}

func TestParseRetryAfterDelay(t *testing.T) {
	assert.Equal(t, 2*time.Second, parseRetryAfterDelay("2"))

	future := time.Now().Add(1500 * time.Millisecond).UTC().Format(http.TimeFormat)
	delay := parseRetryAfterDelay(future)
	assert.Greater(t, delay, time.Duration(0))
	assert.LessOrEqual(t, delay, 2*time.Second)

	assert.Equal(t, time.Duration(0), parseRetryAfterDelay("bogus"))
	assert.Equal(t, time.Duration(0), parseRetryAfterDelay("0"))
}

func TestMistralSupportsNativeToolCalling(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	assert.True(t, mc.SupportsNativeToolCalling())
}

func TestMistralQueryEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1","model":"test","choices":[]}`))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	mc.baseURL = server.URL

	req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", strings.NewReader("{}"))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	_, err = mc.parseCompletionResponse(resp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no response from Mistral")
}

func TestMistralDeltaToolCallsNotParsed(t *testing.T) {
	sseBody := `data: {"id":"1","choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"unix","arguments":"{\"com"}}]}}]}

data: {"id":"2","choices":[{"delta":{"tool_calls":[{"function":{"arguments":"mand\": \"ls\"}"}}]}}]}

data: [DONE]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseBody))
	}))
	defer server.Close()

	t.Setenv("MISTRAL_API_KEY", "test-key")
	t.Setenv("MISTRAL_BASE_URL", server.URL)
	modelID := "mistral-small-latest"
	mc := NewMistralConnector(&modelID)
	if mc == nil {
		t.Fatal("Expected connector to be created")
	}

	req, err := http.NewRequest("POST", server.URL+"/v1/chat/completions", nil)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-key")

	client := &http.Client{}
	resp, err := client.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	// Streaming handler ignores tool_calls in delta (only extracts content)
	result, err := mc.handleStreamingResponse(resp, nil)
	assert.NoError(t, err)
	// Tool call deltas have no "content" field, so result should be empty
	assert.Equal(t, "", result)
}
