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
	// Messages carries prior conversation history. Every connector includes it on
	// the plain Query path (ask/chat). On the QueryWithTool path support is
	// inconsistent across connectors (most build the request from SysPrompt +
	// UserPrompt only, while a few also include Messages), so tool-calling callers
	// must not rely on it: carry any needed history or tool results in UserPrompt
	// instead. All current QueryWithTool callers do this — the task loop renders
	// the full step history into the prompt and the ask web-search loop threads
	// tool results into it.
	Messages  []Message
	Stream    bool
	MaxTokens int
	Device    string
	OnStream  func(string) error
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
}

type ToolCallingConnector interface {
	LLMConnector
	SupportsNativeToolCalling() bool
	// QueryWithTool runs a single tool-calling turn. Callers must include any
	// conversation history or prior tool results in params.UserPrompt: whether
	// params.Messages is honored on this path varies by connector (see the note
	// there), so it must not be relied upon for tool-calling turns. Caller
	// streaming callbacks (params.Stream / params.OnStream) are not honored.
	QueryWithTool(ctx context.Context, params *QueryParams, tools map[string]tools.Tool) (LlmResponseWithTools, error)
}
