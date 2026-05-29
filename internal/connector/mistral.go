package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	MistralProvider     = "mistral"
	DefaultMistralModel = "mistral-small-latest"

	DefaultMistralBaseURL = "https://api.mistral.ai"
)

var (
	mistralMaxRateLimitRetries  = 3
	mistralRateLimitBackoffBase = time.Second
	mistralRateLimitBackoffMax  = 8 * time.Second
)

type MistralRequest struct {
	Model             string           `json:"model"`
	Messages          []MistralMessage `json:"messages"`
	Stream            bool             `json:"stream,omitempty"`
	MaxTokens         *int             `json:"max_tokens,omitempty"`
	Tools             []MistralTool    `json:"tools,omitempty"`
	ToolChoice        string           `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool            `json:"parallel_tool_calls,omitempty"`
}

type MistralMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []MistralToolCall `json:"tool_calls,omitempty"`
}

type MistralTool struct {
	Type     string              `json:"type"`
	Function MistralToolFunction `json:"function"`
}

type MistralToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type MistralToolCall struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function MistralToolCallFunction `json:"function"`
}

type MistralToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type MistralResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Choices []MistralChoice `json:"choices"`
	Usage   *MistralUsage   `json:"usage,omitempty"`
}

type MistralChoice struct {
	Index        int            `json:"index"`
	Message      MistralMessage `json:"message,omitempty"`
	Delta        *MistralDelta  `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason,omitempty"`
}

type MistralDelta struct {
	Content   string            `json:"content,omitempty"`
	ToolCalls []MistralToolCall `json:"tool_calls,omitempty"`
}

type MistralUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type MistralErrorResponse struct {
	Object  string `json:"object"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type MistralConnector struct {
	apiKey     string
	baseURL    string
	modelID    string
	httpClient *http.Client
	logger     zap.Logger
}

func NewMistralConnector(modelID *string) (*MistralConnector, error) {
	logger := *utils.GetLogger()
	logger.Debug("NewMistralConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultMistralModel
		modelID = &model
	}

	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("MISTRAL_API_KEY is required to use Mistral models")
	}

	baseURL := os.Getenv("MISTRAL_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultMistralBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	connector := &MistralConnector{
		apiKey:  apiKey,
		baseURL: baseURL,
		modelID: *modelID,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		logger: logger,
	}

	return connector, nil
}

func convertToolsToMistral(execTools map[string]tools.Tool) []MistralTool {
	var toolSpecs []MistralTool
	for _, tool := range execTools {
		toolSpec := MistralTool{
			Type: "function",
			Function: MistralToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.InputSchema(),
			},
		}
		toolSpecs = append(toolSpecs, toolSpec)
	}
	return toolSpecs
}

func (mc *MistralConnector) buildMessages(qParams *QueryParams) []MistralMessage {
	messages := []MistralMessage{}

	if qParams.SysPrompt != nil && *qParams.SysPrompt != "" {
		messages = append(messages, MistralMessage{
			Role:    "system",
			Content: *qParams.SysPrompt,
		})
	}

	for _, msg := range qParams.Messages {
		messages = append(messages, MistralMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	if qParams.UserPrompt != nil && *qParams.UserPrompt != "" {
		messages = append(messages, MistralMessage{
			Role:    "user",
			Content: *qParams.UserPrompt,
		})
	}

	return messages
}

func (mc *MistralConnector) Query(ctx context.Context, params *QueryParams) (string, error) {
	mc.logger.Sugar().Debugw("Query", "model", mc.modelID)

	messages := mc.buildMessages(params)

	request := MistralRequest{
		Model:    mc.modelID,
		Messages: messages,
		Stream:   params.Stream,
	}

	if params.MaxTokens > 0 {
		maxTokens := params.MaxTokens
		request.MaxTokens = &maxTokens
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := mc.baseURL + "/v1/chat/completions"
	resp, err := mc.doRequestWithRateLimitRetry(ctx, url, requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", mc.parseErrorResponse(resp)
	}

	if params.Stream {
		return mc.handleStreamingResponse(resp, params.OnStream)
	}

	return mc.parseCompletionResponse(resp)
}

func (mc *MistralConnector) QueryWithTool(ctx context.Context, params *QueryParams, execTools map[string]tools.Tool) (LlmResponseWithTools, error) {
	mc.logger.Sugar().Debugw("Query with tool", "model", mc.modelID)
	response := LlmResponseWithTools{}

	messages := []MistralMessage{}

	if params.SysPrompt != nil && *params.SysPrompt != "" {
		messages = append(messages, MistralMessage{
			Role:    "system",
			Content: *params.SysPrompt,
		})
	}

	if params.UserPrompt != nil && *params.UserPrompt != "" {
		messages = append(messages, MistralMessage{
			Role:    "user",
			Content: *params.UserPrompt,
		})
	}

	mistralTools := convertToolsToMistral(execTools)
	parallelToolCalls := false

	request := MistralRequest{
		Model:             mc.modelID,
		Messages:          messages,
		Tools:             mistralTools,
		ToolChoice:        "auto",
		ParallelToolCalls: &parallelToolCalls,
	}

	if params.MaxTokens > 0 {
		maxTokens := params.MaxTokens
		request.MaxTokens = &maxTokens
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := mc.baseURL + "/v1/chat/completions"
	mc.logger.Sugar().Debugw("Sending message to Mistral", "userPrompt", *params.UserPrompt, "tools", len(mistralTools))

	resp, err := mc.doRequestWithRateLimitRetry(ctx, url, requestBody)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return response, mc.parseErrorResponse(resp)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response: %w", err)
	}

	var mistralResp MistralResponse
	if err := json.Unmarshal(responseBody, &mistralResp); err != nil {
		return response, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	mc.logger.Sugar().Debugw("Received response from Mistral", "response", mistralResp)

	if len(mistralResp.Choices) == 0 {
		return response, fmt.Errorf("no response from Mistral")
	}

	choice := mistralResp.Choices[0]
	if choice.Message.Content != "" {
		response.Response = choice.Message.Content
	}

	if len(choice.Message.ToolCalls) > 0 {
		toolCall := choice.Message.ToolCalls[0]

		if toolCall.Function.Name == "" {
			return response, fmt.Errorf("tool call name is empty")
		}
		if toolCall.Function.Arguments == "" {
			return response, fmt.Errorf("tool call arguments are empty")
		}

		var args map[string]any
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return response, fmt.Errorf("failed to unmarshal tool call arguments: %w", err)
		}

		response.ToolUse = true
		response.ToolName = toolCall.Function.Name
		response.ToolInput = args
	}

	return response, nil
}

func (mc *MistralConnector) SupportsNativeToolCalling() bool {
	return true
}

func (mc *MistralConnector) doRequestWithRateLimitRetry(ctx context.Context, url string, requestBody []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+mc.apiKey)

		resp, err := mc.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt >= mistralMaxRateLimitRetries {
			return resp, nil
		}

		delay := mistralRateLimitDelay(resp, attempt)
		mc.logger.Sugar().Warnw("Mistral rate limited, retrying request", "attempt", attempt+1, "maxRetries", mistralMaxRateLimitRetries, "delay", delay)
		resp.Body.Close()

		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func mistralRateLimitDelay(resp *http.Response, attempt int) time.Duration {
	if delay := parseRetryAfterDelay(resp.Header.Get("Retry-After")); delay > 0 {
		return delay
	}

	delay := mistralRateLimitBackoffBase << attempt
	if delay > mistralRateLimitBackoffMax {
		return mistralRateLimitBackoffMax
	}

	return delay
}

func parseRetryAfterDelay(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	retryTime, err := http.ParseTime(value)
	if err != nil {
		return 0
	}

	delay := time.Until(retryTime)
	if delay <= 0 {
		return 0
	}

	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (mc *MistralConnector) parseCompletionResponse(resp *http.Response) (string, error) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var mistralResp MistralResponse
	if err := json.Unmarshal(responseBody, &mistralResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(mistralResp.Choices) == 0 {
		return "", fmt.Errorf("no response from Mistral")
	}

	return mistralResp.Choices[0].Message.Content, nil
}

func (mc *MistralConnector) handleStreamingResponse(resp *http.Response, onStream func(string) error) (string, error) {
	var mdRenderer *MarkdownStreamRenderer
	if onStream == nil {
		renderer, err := NewMarkdownStreamRenderer()
		if err != nil {
			mc.logger.Warn("Failed to create markdown renderer, falling back to plain text", zap.Error(err))
		} else {
			mdRenderer = renderer
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	var fullResponse string

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk MistralResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta != nil {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				fullResponse += content

				if onStream != nil {
					if err := onStream(content); err != nil {
						return "", err
					}
				} else if mdRenderer != nil {
					mdRenderer.ProcessChunk(content)
				} else {
					fmt.Print(content)
				}
			}
		}
	}

	if mdRenderer != nil {
		mdRenderer.Flush()
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading stream: %w", err)
	}

	return fullResponse, nil
}

func (mc *MistralConnector) parseErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp MistralErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		return fmt.Errorf("mistral API error (status %d): %s", resp.StatusCode, errResp.Message)
	}

	bodyStr := string(body)
	if bodyStr == "" {
		bodyStr = http.StatusText(resp.StatusCode)
	}

	return fmt.Errorf("mistral API returned status %d: %s", resp.StatusCode, bodyStr)
}
