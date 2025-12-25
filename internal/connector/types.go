package connector

import (
	"context"

	"github.com/laszukdawid/terminal-agent/internal/tools"
)

type LLMPrice struct {
	InputPrice  float64
	OutputPrice float64
	TotalPrice  float64
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PerplexityUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Delta struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Choice struct {
	Index        int     `json:"index"`
	FinishReason string  `json:"finish_reason"`
	Message      Message `json:"message"`
	Delta        Delta   `json:"delta"`
}

type PerplexityResponse struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Created int64           `json:"created"`
	Usage   PerplexityUsage `json:"usage"`
	Object  string          `json:"object"`
	Choices []Choice        `json:"choices"`
}

type ClaudeResponseContent struct {
	Responses []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
}

type ClaudeResponse struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	Role         string       `json:"role"`
	Model        string       `json:"model"`
	Content      []Content    `json:"content"`
	StopReason   string       `json:"stop_reason"`
	StopSequence *string      `json:"stop_sequence"` // Using *string to allow null
	Usage        BedrockUsage `json:"usage"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type BedrockUsage struct {
	InputTokens  int32 `json:"inputTokens"`
	OutputTokens int32 `json:"outputTokens"`
	TotalTokens  int32 `json:"totalTokens"`
}

type ClaudeRequest struct {
	System            string    `json:"system"`
	Messages          []Message `json:"messages"`
	AnthropicVersion  string    `json:"anthropic_version"`
	MaxTokensToSample int       `json:"max_tokens"`
	Temperature       float64   `json:"temperature,omitempty"`
	StopSequences     []string  `json:"stop_sequences,omitempty"`
}

type QueryParams struct {
	UserPrompt *string
	SysPrompt  *string
	Messages   []Message
	Stream     bool
	MaxTokens  int
}

type LlmResponseWithTools struct {
	Response     string
	ToolUse      bool
	ToolName     string
	ToolInput    map[string]any
	ToolResponse any
}

type LLMConnector interface {
	Query(ctx context.Context, params *QueryParams) (string, error)
	QueryWithTool(ctx context.Context, params *QueryParams, tools map[string]tools.Tool) (LlmResponseWithTools, error)
}
