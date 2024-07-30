package connector

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
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ClaudeRequest struct {
	Messages          []Message `json:"messages"`
	AntrhopicVersion  string    `json:"anthropic_version"`
	MaxTokensToSample int       `json:"max_tokens"`
	Temperature       float64   `json:"temperature,omitempty"`
	StopSequences     []string  `json:"stop_sequences,omitempty"`
}
