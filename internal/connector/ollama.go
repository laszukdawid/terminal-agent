package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/laszukdawid/terminal-agent/internal/tools"
	"github.com/laszukdawid/terminal-agent/internal/utils"
	"go.uber.org/zap"
)

const (
	OllamaProvider     = "ollama"
	DefaultOllamaModel = "llama3.2"

	DefaultOllamaHost = "http://localhost:11434" // Default Ollama endpoint
)

// OllamaRequest represents the request structure for Ollama API
type OllamaRequest struct {
	Model    string          `json:"model"`
	Prompt   string          `json:"prompt,omitempty"`
	Messages []OllamaMessage `json:"messages,omitempty"`
	Stream   bool            `json:"stream"`
	Tools    []OllamaTool    `json:"tools,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

// OllamaMessage represents a message in the chat format
type OllamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []OllamaToolCall `json:"tool_calls,omitempty"`
}

// OllamaTool represents a tool definition for Ollama
type OllamaTool struct {
	Type     string             `json:"type"`
	Function OllamaToolFunction `json:"function"`
}

// OllamaToolFunction represents the function definition within a tool
type OllamaToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// OllamaToolCall represents a tool call in the response
type OllamaToolCall struct {
	Function OllamaToolCallFunction `json:"function"`
}

// OllamaToolCallFunction represents the function call details
type OllamaToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// OllamaResponse represents the response structure from Ollama API
type OllamaResponse struct {
	Model              string         `json:"model"`
	CreatedAt          time.Time      `json:"created_at"`
	Response           string         `json:"response"`
	Done               bool           `json:"done"`
	Message            *OllamaMessage `json:"message,omitempty"`
	Context            []int          `json:"context,omitempty"`
	TotalDuration      int64          `json:"total_duration,omitempty"`
	LoadDuration       int64          `json:"load_duration,omitempty"`
	PromptEvalCount    int            `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64          `json:"prompt_eval_duration,omitempty"`
	EvalCount          int            `json:"eval_count,omitempty"`
	EvalDuration       int64          `json:"eval_duration,omitempty"`
}

// OllamaConnector implements the LLMConnector interface for Ollama
type OllamaConnector struct {
	baseURL    string
	modelID    string
	httpClient *http.Client
	logger     zap.Logger
}

// NewOllamaConnector creates a new OllamaConnector instance
func NewOllamaConnector(modelID *string) *OllamaConnector {
	logger := *utils.GetLogger()
	logger.Debug("NewOllamaConnector")

	if modelID == nil || *modelID == "" {
		model := DefaultOllamaModel
		modelID = &model
	}

	// Get baseURL from environment variable or use default
	baseURL := os.Getenv("OLLAMA_HOST")
	if baseURL == "" {
		baseURL = DefaultOllamaHost
	}

	connector := &OllamaConnector{
		baseURL: baseURL,
		modelID: *modelID,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		logger: logger,
	}

	return connector
}

// convertToolsToOllama converts the tools map to Ollama tool format
func convertToolsToOllama(execTools map[string]tools.Tool) []OllamaTool {
	var toolSpecs []OllamaTool
	for _, tool := range execTools {
		inputSchema := tool.InputSchema()

		toolSpec := OllamaTool{
			Type: "function",
			Function: OllamaToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  inputSchema,
			},
		}
		toolSpecs = append(toolSpecs, toolSpec)
	}
	return toolSpecs
}

// Query implements the Query method of the LLMConnector interface
func (oc *OllamaConnector) Query(ctx context.Context, params *QueryParams) (string, error) {
	oc.logger.Sugar().Debugw("Query", "model", oc.modelID)

	// Build the prompt from user and system prompts
	prompt := ""
	if params.SysPrompt != nil && *params.SysPrompt != "" {
		prompt = fmt.Sprintf("System: %s\n\n", *params.SysPrompt)
	}
	if params.UserPrompt != nil {
		prompt += *params.UserPrompt
	}

	// Create the request payload
	request := OllamaRequest{
		Model:  oc.modelID,
		Prompt: prompt,
		Stream: params.Stream, // Use the stream parameter from QueryParams
	}

	// Add options if needed
	if params.MaxTokens > 0 {
		request.Options = map[string]any{
			"num_predict": params.MaxTokens,
		}
	}

	// Marshal the request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/generate", oc.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := oc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and parse the response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(responseBody, &ollamaResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return ollamaResp.Response, nil
}

// QueryWithTool implements the QueryWithTool method of the LLMConnector interface
func (oc *OllamaConnector) QueryWithTool(ctx context.Context, params *QueryParams, tools map[string]tools.Tool) (LlmResponseWithTools, error) {
	oc.logger.Sugar().Debugw("Query with tool", "model", oc.modelID)
	response := LlmResponseWithTools{}

	// Convert tools to Ollama format
	ollamaTools := convertToolsToOllama(tools)

	// Create messages for chat format (required for tool calling)
	messages := []OllamaMessage{}
	if params.SysPrompt != nil && *params.SysPrompt != "" {
		messages = append(messages, OllamaMessage{
			Role:    "system",
			Content: *params.SysPrompt,
		})
	}
	if params.UserPrompt != nil {
		messages = append(messages, OllamaMessage{
			Role:    "user",
			Content: *params.UserPrompt,
		})
	}

	// Create the request payload
	request := OllamaRequest{
		Model:    oc.modelID,
		Messages: messages,
		Tools:    ollamaTools,
		Stream:   false,
	}

	// Add options if needed
	if params.MaxTokens > 0 {
		request.Options = map[string]any{
			"num_predict": params.MaxTokens,
		}
	}

	// Marshal the request to JSON
	requestBody, err := json.Marshal(request)
	if err != nil {
		return response, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request - use /api/chat endpoint for tool calling
	url := fmt.Sprintf("%s/api/chat", oc.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return response, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	oc.logger.Sugar().Debugw("Sending message to Ollama", "userPrompt", *params.UserPrompt, "tools", len(ollamaTools))

	// Send the request
	resp, err := oc.httpClient.Do(req)
	if err != nil {
		return response, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return response, fmt.Errorf("ollama API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and parse the response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return response, fmt.Errorf("failed to read response: %w", err)
	}

	var ollamaResp OllamaResponse
	if err := json.Unmarshal(responseBody, &ollamaResp); err != nil {
		return response, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	oc.logger.Sugar().Debugw("Received response from Ollama", "response", ollamaResp)

	// Handle the response
	if ollamaResp.Message != nil {
		response.Response = ollamaResp.Message.Content
	} else if ollamaResp.Response != "" {
		response.Response = ollamaResp.Response
	}

	// Check for tool calls
	if ollamaResp.Message != nil && len(ollamaResp.Message.ToolCalls) > 0 {
		// Process the first tool call (similar to other connectors)
		toolCall := ollamaResp.Message.ToolCalls[0]

		// Validate tool call
		if toolCall.Function.Name == "" {
			return response, fmt.Errorf("tool call name is empty")
		}
		if toolCall.Function.Arguments == nil {
			return response, fmt.Errorf("tool call arguments are empty")
		}

		response.ToolUse = true
		response.ToolName = toolCall.Function.Name
		response.ToolInput = toolCall.Function.Arguments
	}

	return response, nil
}
